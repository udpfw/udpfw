package tcp

import (
	"bytes"
	"errors"
	"github.com/udpfw/common"
	"go.uber.org/zap"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type Client struct {
	id         string
	conn       net.Conn
	log        *zap.Logger
	done       func()
	stopped    atomic.Bool
	writeQueue chan common.ClientMessage

	readySignal chan bool
	assembler   *common.MessageAssembler
	broadcast   func(msg common.ClientMessage)
	hostname    string
}

func (c *Client) service() {
	go func() {
		defer c.done()
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
	wantsHello := true
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
		bufferCur += n
		if wantsHello && bufferCur >= common.HelloSize {
			helloData := buffer[:common.HelloSize]
			if !bytes.Equal(helloData, common.HelloMessage) {
				c.log.Warn("Dropping client offering invalid handshake")
				c.drop()
				return
			}
			// Hello is good. Rearrange the buffer, and continue as it would.
			if bufferCur-common.HelloSize > 0 {
				for i := 0; i < bufferCur; i++ {
					buffer[i] = buffer[common.HelloSize+i]
				}
			}
			n -= common.HelloSize
			bufferCur = 0
			c.log.Info("Received valid handshake")
			wantsHello = false
			c.Write(common.NewClientMessage(common.ClientMessageAck, []byte(c.hostname)))
			c.ready()
			if n == 0 {
				continue
			}
		}

		for i := 0; i < n; i++ {
			msg := c.assembler.Feed(buffer[i])
			if msg != nil {
				c.log.Debug("Got message from client")
				c.handleMessage(msg)
			}
		}
		bufferCur = 0
	}
}

func (c *Client) handleMessage(msg common.ClientMessage) {
	switch msg.Type() {
	case common.ClientMessagePkt:
		c.log.Debug("Processing PKT message")
		c.broadcast(msg)
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

func NewClient(hostname, id string, conn net.Conn, done func(), broadcast func(msg common.ClientMessage)) *Client {
	return &Client{
		id:          id,
		hostname:    hostname,
		conn:        conn,
		log:         zap.L().With(zap.String("facility", "TCP"), zap.String("client", id)),
		done:        done,
		writeQueue:  make(chan common.ClientMessage, 64),
		readySignal: make(chan bool),
		assembler:   common.NewMessageAssembler(),
		broadcast:   broadcast,
	}
}
