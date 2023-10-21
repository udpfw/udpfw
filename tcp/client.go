package tcp

import (
	"bytes"
	"errors"
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
	writeQueue chan ClientMessage

	readySignal chan bool
	assembler   *messageAssembler
	broadcast   func(msg ClientMessage)
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
	}
}

func (c *Client) serviceReads(done func()) {
	defer done()
	wantsHello := true
	buffer := make([]byte, 0, 64)
	bufferCur := 0
	for {
		n, err := c.conn.Read(buffer[bufferCur:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !c.stopped.Load() {
					c.log.Debug("Client disconnected without BYE packet")
					c.drop()
					return
				}
				return // Exiting cleanly.
			} else {
				c.log.Error("Error reading", zap.Error(err))
				c.drop()
				return
			}
		}
		bufferCur += n
		if wantsHello && bufferCur >= helloSize {
			helloData := buffer[:helloSize]
			if !bytes.Equal(helloData, helloMessage) {
				c.log.Warn("Dropping client offering invalid handshake")
				c.drop()
				return
			}
			// Hello is good. Rearrange the buffer, and continue as it would.
			if bufferCur-helloSize > 0 {
				for i := 0; i < bufferCur; i++ {
					buffer[i] = buffer[helloSize+i]
				}
				n -= helloSize
			}
			wantsHello = false
			c.ready()
		}

		for i := 0; i < bufferCur; i++ {
			msg := c.assembler.feed(buffer[i])
			if msg != nil {
				c.handleMessage(msg)
				buffer = buffer[:0]
				bufferCur = 0
			}
		}
	}
}

func (c *Client) handleMessage(msg ClientMessage) {
	switch msg.Type() {
	case ClientMessagePkt:
		c.broadcast(msg)
	case ClientMessageBye:
		c.drop()
	case ClientMessageInvalid:
		c.log.Warn("Ignoring message with invalid type", zap.ByteString("payload", msg))
	}
}

func (c *Client) Write(msg ClientMessage) {
	c.writeQueue <- msg
}

func NewClient(id string, conn net.Conn, done func(), broadcast func(msg ClientMessage)) *Client {
	return &Client{
		id:          id,
		conn:        conn,
		log:         zap.L().With(zap.String("facility", "TCP"), zap.String("client", id)),
		done:        done,
		writeQueue:  make(chan ClientMessage, 64),
		readySignal: make(chan bool),
		assembler:   newMessageAssembler(),
		broadcast:   broadcast,
	}
}
