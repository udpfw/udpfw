package services

import (
	"errors"
	"fmt"
	"github.com/udpfw/common"
	"go.uber.org/zap"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type DispatchStatus string

var DrainingErr = fmt.Errorf("cannot write: drain in progress")

const (
	StatusConnecting    DispatchStatus = "connecting"
	StatusConnected     DispatchStatus = "connected"
	StatusDisconnecting DispatchStatus = "disconnecting"
	StatusDisconnected  DispatchStatus = "disconnected"
	StatusReconnecting  DispatchStatus = "reconnecting"
	StatusSwitching     DispatchStatus = "switching"
)

func NewDispatch(address string, handler *LoopHandler) *Dispatch {
	return &Dispatch{
		log:        zap.L().With(zap.String("facility", "dispatch")),
		address:    address,
		writeQueue: make(chan []byte, 4096),

		enqueued:   &atomic.Int32{},
		status:     &atomic.Value{},
		lastError:  &atomic.Value{},
		serverHost: &atomic.Value{},
		draining:   &atomic.Bool{},
		writerDone: make(chan bool),

		OnConnect:    make(chan struct{}),
		OnDisconnect: make(chan struct{}),
		OnPacket:     make(chan []byte, 4096),
		stop:         &atomic.Bool{},
		conn:         nil,

		suspended:  false,
		writerLock: &sync.Mutex{},
		readLock:   &sync.Mutex{},
	}
}

type Dispatch struct {
	log        *zap.Logger
	address    string
	writeQueue chan []byte

	enqueued   *atomic.Int32
	status     *atomic.Value
	lastError  *atomic.Value
	serverHost *atomic.Value
	draining   *atomic.Bool
	writerDone chan bool

	OnConnect    chan struct{}
	OnDisconnect chan struct{}
	OnPacket     chan []byte
	stop         *atomic.Bool
	conn         *dispatchConnection

	suspended  bool
	writerLock *sync.Mutex
	readLock   *sync.Mutex
}

type DispatchError struct {
	Error error
	Time  time.Time
}

func (d *Dispatch) Status() DispatchStatus    { return d.status.Load().(DispatchStatus) }
func (d *Dispatch) ServerHost() *string       { return d.serverHost.Load().(*string) }
func (d *Dispatch) LastError() *DispatchError { return d.lastError.Load().(*DispatchError) }

func (d *Dispatch) setStatus(val DispatchStatus) {
	d.status.Store(val)
	d.log.Debug("Status transitioned", zap.String("status", string(val)))
}

func (d *Dispatch) setError(err error) {
	d.lastError.Store(&DispatchError{
		Error: err,
		Time:  time.Now().UTC(),
	})
}

func (d *Dispatch) synchronize(lock sync.Locker) {
	defer lock.Unlock()
	lock.Lock()
}

func (d *Dispatch) makeConnection() {
	d.setStatus(StatusConnecting)
	var disp *dispatchConnection
	for {
		conn, err := net.Dial("tcp", d.address)
		if err == nil {
			disp, err = newDispatchConnection(d, conn)
			if err == nil {
				d.conn = disp
				d.serverHost.Store(&d.conn.ServerHost)
				d.log.Info("Now connected", zap.String("host", d.conn.ServerHost))
				if d.suspended {
					d.resume()
				}
				d.setStatus(StatusConnected)
				break
			}
		}

		timeout := 2 * time.Second
		d.log.Error("Connection attempt failed", zap.Duration("cooldown", timeout), zap.Error(err))
		d.setError(err)
		time.Sleep(timeout)
	}
}

func (d *Dispatch) Run() {
	d.makeConnection()
	d.service()
	d.setStatus(StatusDisconnected)
}

func (d *Dispatch) Write(data []byte) error {
	if d.draining.Load() {
		return DrainingErr
	}
	d.writeQueue <- data
	return nil
}

func (d *Dispatch) service() {
	stopped := make(chan bool, 2)
	go func() {
		d.serviceWrites()
		stopped <- true
	}()

	go func() {
		d.serviceReads()
		stopped <- true
	}()

	<-stopped
}

func (d *Dispatch) drain() {
	if d.draining.Swap(true) {
		return // Already draining
	}
	d.log.Info("Drain started")
	close(d.writeQueue)
	<-d.writerDone
	d.log.Info("Drain completed")
}

func (d *Dispatch) Shutdown() {
	if d.stop.Swap(true) {
		return
	}
	d.log.Info("Shutdown called")
	d.setStatus(StatusDisconnecting)

	d.drain()
	if err := d.conn.Write(common.NewClientMessage(common.ClientMessageBye, nil)); err != nil {
		d.log.Error("Failed emitting BYE packet", zap.Error(err))
	}
	if err := d.conn.Shutdown(); err != nil && !errors.Is(err, net.ErrClosed) {
		d.log.Error("Underlying connection failed to close. Check for leaked resources.", zap.Error(err))
	}
	close(d.OnPacket)
}

func (d *Dispatch) serviceReads() {
	for !d.stop.Load() {
		d.synchronize(d.readLock)
		pkt := d.conn.Read()
		if pkt == nil {
			continue
		}
		d.OnPacket <- pkt.Payload()
	}
}

func (d *Dispatch) serviceWrites() {
	defer close(d.writerDone)
	for !d.stop.Load() {
		toWrite, ok := <-d.writeQueue
		if !ok {
			return
		}
		for {
			d.synchronize(d.readLock)
			err := d.conn.Write(common.NewClientMessage(common.ClientMessagePkt, toWrite))
			if err != nil {
				d.log.Debug("Failed writing current packet due to broken connection. Will retry after synchronization is complete.")
				// Connection is broken, try again after resynchronize
				continue
			}
			break
		}
	}
}

func (d *Dispatch) handlePacket(pkt common.ClientMessage) {
	switch pkt.Type() {
	case common.ClientMessagePkt:
		d.OnPacket <- pkt.Payload()
	case common.ClientMessageBye:
		d.log.Info("Received disconnection request from dispatcher. Reconnecting...")
		d.reboot()
	default:
		d.log.Warn("Received unknown packet from dispatcher", zap.ByteString("data", pkt))
	}
}

func (d *Dispatch) suspend() {
	d.log.Debug("Suspending reads and writes")
	d.writerLock.Lock()
	d.readLock.Lock()
	d.suspended = true
}

func (d *Dispatch) resume() {
	d.log.Debug("Resuming reads and writes")
	d.writerLock.Unlock()
	d.readLock.Unlock()
	d.suspended = false
}

func (d *Dispatch) reboot() {
	d.log.Debug("Now switching dispatch server")
	d.setStatus(StatusSwitching)
	d.suspend()
	if err := d.conn.Write(common.NewClientMessage(common.ClientMessageBye, nil)); err != nil {
		d.log.Warn("Failed emitting BYE packet", zap.Error(err))
	}
	if err := d.conn.Shutdown(); err != nil {
		d.log.Error("Failed closing previous underlying connection. Check for leaked resources.", zap.Error(err))
	}
	d.makeConnection()
}

func (d *Dispatch) notifyBroken(conn *dispatchConnection) {
	if d.conn != conn || d.suspended {
		return
	}

	d.log.Info("Received broken connection notification from underlying connection. Will attempt to reconnect.")
	d.reboot()
}
