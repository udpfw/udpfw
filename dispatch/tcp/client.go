package tcp

import (
	"errors"
	"github.com/udpfw/common"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type Client struct {
	id          string
	conn        net.Conn
	log         *zap.Logger
	stopped     atomic.Bool
	writeQueue  chan common.ClientMessage
	readySignal chan bool
	assembler   *common.MessageAssembler
	wantsHello  bool
	ns          *string
	server      *Server
}

func (c *Client) service() {
	go func() {
		defer c.server.SignalDone(c)
		wg := sync.WaitGroup{}
		wg.Add(2)
		go c.serviceWrites(wg.Done)
		go c.serviceReads(wg.Done)
		wg.Wait()
	}()
}

func (c *Client) ready() { close(c.readySignal) }

func (c *Client) drop() {
	if c.stopped.Swap(true) {
		return // Already stopped, or in the process of stopping.
	}
	_ = c.conn.Close()
}

func (c *Client) serviceWrites(done func()) {
	defer done()
	<-c.readySignal

	// In case we were dropped before the ready signal...
	if c.stopped.Load() {
		return
	}

	for msg := range c.writeQueue {
		if msg == nil {
			break
		}

		toWrite := len(msg)
		written := 0
		for written < toWrite {
			n, err := c.conn.Write(msg[written:])
			if err != nil {
				if errors.Is(err, io.EOF) {
					if !c.stopped.Load() {
						c.log.Debug("Client disconnected without BYE packet")
						c.drop()
						return
					}
					return // Exiting cleanly.
				}
				c.log.Error("Error writing", zap.Error(err))
				c.drop()
				return
			}
			written += n
		}
		c.log.Debug("Wrote payload to client", zap.Int("size", toWrite))
	}
}

func (c *Client) serviceReads(done func()) {
	defer done()
	buffer := make([]byte, 128)
	bufferCur := 0
	for {
		n, err := c.conn.Read(buffer[bufferCur:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !c.stopped.Load() {
					c.log.Debug("Client disconnected without BYE packet")
					c.drop()
				}
			} else {
				c.log.Error("Error reading", zap.Error(err))
				c.drop()
			}
			return
		}

		for i := 0; i < n; i++ {
			msg := c.assembler.Feed(buffer[i])
			if msg != nil {
				c.log.Debug("Got message from client")
				c.handleMessage(msg)
				break
			}

			if c.wantsHello && c.assembler.BufferSize() > 2 && c.assembler.ExpectedType() != common.ClientMessageHello {
				c.log.Info("Dropping client emitting invalid first message")
				c.drop()
				return
			}

		}
		bufferCur = 0
	}
}

func (c *Client) handleMessage(msg common.ClientMessage) {
	if c.wantsHello && msg.Type() != common.ClientMessageHello {
		c.log.Info("Dropping client attempting exchange without handshake")
		c.drop()
		return
	}

	switch msg.Type() {
	case common.ClientMessageHello:
		c.log.Debug("Received valid handshake")
		var ns string
		if msg.PayloadSize() > 0 {
			ns = string(msg.Payload())
			c.log.Debug("Registered interest in namespace", zap.String("namespace", ns))
		} else {
			ns = "$$global"
			c.log.Debug("Client is running on global namespace")
		}
		c.ns = &ns
		c.wantsHello = false
		c.server.AssocNamespace(c, ns)
		c.Write(common.NewClientMessage(common.ClientMessageAck, []byte(c.server.hostname)))
		c.ready()

	case common.ClientMessagePing:
		c.log.Debug("Processing PING message")
		c.Write(common.NewClientMessage(common.ClientMessagePong, nil))

	case common.ClientMessagePkt:
		c.log.Debug("Processing PKT message")
		c.server.RequestBroadcast(c, msg)

	case common.ClientMessageBye:
		c.log.Debug("Processing BYE message")
		c.drop()

	case common.ClientMessageInvalid:
		c.log.Warn("Ignoring message with invalid type", zap.ByteString("payload", msg))
	}
}

func (c *Client) Write(msg common.ClientMessage) {
	c.writeQueue <- msg
}

func NewClient(s *Server, id string, conn net.Conn) *Client {
	return &Client{
		id:          id,
		conn:        conn,
		log:         zap.L().With(zap.String("facility", "TCP"), zap.String("client", id)),
		writeQueue:  make(chan common.ClientMessage, 64),
		readySignal: make(chan bool),
		assembler:   common.NewMessageAssembler(),
		server:      s,
	}
}
