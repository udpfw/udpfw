package services

import (
	"errors"
	"fmt"
	"github.com/udpfw/common"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var ConnectionBrokenErr = fmt.Errorf("connection is broken. Try again")

type dummyDispatcher struct{}

func (d *dummyDispatcher) notifyBroken(connection *dispatchConnection) {}
func (d *dummyDispatcher) targetNamespace() []byte                     { return nil }

var dummyDispatch Dispatcher = &dummyDispatcher{}

func newDispatchConnection(parent *Dispatch, conn net.Conn) (*dispatchConnection, error) {
	running := &atomic.Bool{}
	running.Store(true)
	ackLock := &sync.Mutex{}
	ackCond := sync.NewCond(ackLock)

	d := &dispatchConnection{
		conn: conn,
		asm:  common.NewMessageAssembler(),

		running:       running,
		disconnecting: &atomic.Bool{},

		receivedAck: false,
		ackLock:     ackLock,
		ackCond:     ackCond,

		ch:   make(chan common.ClientMessage, 100),
		done: make(chan bool),
	}

	d.storeParent(parent)

	if err := d.handshake(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			// TODO: LOG
		}
		return nil, err
	}

	return d, nil
}

type dispatchConnection struct {
	conn      net.Conn
	asm       *common.MessageAssembler
	parentPtr unsafe.Pointer

	running       *atomic.Bool
	disconnecting *atomic.Bool
	broken        *atomic.Bool

	receivedAck bool
	ackLock     *sync.Mutex
	ackCond     *sync.Cond
	ackError    error
	ServerHost  string

	ch   chan common.ClientMessage
	done chan bool
}

func (d *dispatchConnection) shouldRelayConnectionError() bool {
	return !d.disconnecting.Load() && !d.broken.Load()
}

func (d *dispatchConnection) storeParent(parent Dispatcher) {
	atomic.StorePointer(&d.parentPtr, unsafe.Pointer(&parent))
}

func (d *dispatchConnection) parent() Dispatcher {
	return *((*Dispatcher)(atomic.LoadPointer(&d.parentPtr)))
}

func (d *dispatchConnection) serviceReads() {
	defer close(d.ch)
	buffer := make([]byte, 4096)
	for d.running.Load() {
		n, err := d.conn.Read(buffer)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				d.notifyBroken()
			}
			return
		}

		for _, b := range buffer[:n] {
			var pkt common.ClientMessage
			if pkt = d.asm.Feed(b); pkt == nil {
				continue
			}
			if !d.receivedAck {
				if pkt.Type() != common.ClientMessageAck {
					d.ackError = fmt.Errorf("server responded with invalid ack")
				} else {
					d.ServerHost = string(pkt.Payload())
				}

				d.ackLock.Lock()
				d.ackCond.Signal()
				d.ackLock.Unlock()
				d.receivedAck = true

				if d.ackError == nil {
					continue
				}
				return
			}
			d.ch <- pkt
		}
	}
}

func (d *dispatchConnection) handshake() error {
	go d.serviceReads()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	okChan := make(chan bool)
	go func() {
		d.ackLock.Lock()
		defer d.ackLock.Unlock()
		d.ackCond.Wait()
		okChan <- true
	}()

	handshake := common.NewClientMessage(common.ClientMessageHello,
		d.parent().targetNamespace())
	if err := d.Write(handshake); err != nil {
		return err
	}

	select {
	case <-timer.C:
		return fmt.Errorf("server did not respond to handshake in time")
	case <-okChan:
		return nil
	}

}

func (d *dispatchConnection) Write(pkt common.ClientMessage) error {
	toWrite := len(pkt)
	written := 0
	for written < toWrite {
		n, err := d.conn.Write(pkt[written:])
		if err != nil {
			// TODO: LOG
			d.notifyBroken()
			return ConnectionBrokenErr
		}
		written += n
	}

	return nil
}

func (d *dispatchConnection) Read() common.ClientMessage { return <-d.ch }

func (d *dispatchConnection) Shutdown() error {
	d.running.Swap(false)
	err := d.conn.Close()
	d.storeParent(dummyDispatch)
	close(d.done)
	return err
}

func (d *dispatchConnection) wait() { <-d.done }

func (d *dispatchConnection) notifyBroken() {
	if parent := d.parent(); parent != nil {
		parent.(Dispatcher).notifyBroken(d)
	}
}
