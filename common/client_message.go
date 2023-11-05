package common

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

/*
Hello 0x00 0x01 0x00 "!UDPFW" 0x00 [size u16 be] [payload]

Ack   0x00 0x02

Ping  0x00 0x03

Pong  0x00 0x04

Pkt   0x00 0x05 [size u16 be] [payload]

Bye   0x00 0x06
*/

var HelloMagic = []byte("\x00!UDPFW\x00")
var HelloSize = len(HelloMagic)

type ClientMessageType byte

const (
	ClientMessageInvalid ClientMessageType = iota
	ClientMessageHello
	ClientMessageAck
	ClientMessagePing
	ClientMessagePong
	ClientMessagePkt
	ClientMessageBye
)

var sizeOffset = map[ClientMessageType]int{
	ClientMessageHello: 10,
	ClientMessageAck:   0,
	ClientMessagePing:  0,
	ClientMessagePong:  0,
	ClientMessagePkt:   2,
	ClientMessageBye:   0,
}

type ClientMessage []byte

func (c ClientMessage) Type() ClientMessageType {
	if len(c) < 2 {
		return ClientMessageInvalid
	}

	switch c[1] {
	case 0x01:
		return ClientMessageHello
	case 0x02:
		return ClientMessageAck
	case 0x03:
		return ClientMessagePing
	case 0x04:
		return ClientMessagePong
	case 0x05:
		return ClientMessagePkt
	case 0x06:
		return ClientMessageBye
	default:
		return ClientMessageInvalid
	}
}

var kindToString = map[ClientMessageType]string{
	ClientMessageInvalid: "INVALID",
	ClientMessageHello:   "HELLO",
	ClientMessageAck:     "ACK",
	ClientMessagePing:    "PING",
	ClientMessagePong:    "PONG",
	ClientMessagePkt:     "PKT",
	ClientMessageBye:     "BYE",
}

func (c ClientMessage) PayloadSize() int {
	offset, ok := sizeOffset[c.Type()]
	if !ok || offset == 0 {
		return 0
	}

	return int(binary.BigEndian.Uint16(c[offset : offset+2]))
}

func (c ClientMessage) String() string {
	return fmt.Sprintf("ClientMessage{Type: %s, Size: %d, Payload: %#v}",
		kindToString[c.Type()], c.PayloadSize(), c.Payload())
}

func (c ClientMessage) Payload() []byte {
	if c.PayloadSize() == 0 {
		return nil
	}
	offset, ok := sizeOffset[c.Type()]
	if !ok {
		return nil
	}

	return c[offset+2 : offset+2+c.PayloadSize()]
}

func (c ClientMessage) Deconstruct() (ClientMessageType, []byte) {
	t := c.Type()
	if t == ClientMessageInvalid {
		return t, nil
	}
	return t, c.Payload()
}

func NewClientMessage(kind ClientMessageType, payload []byte) ClientMessage {
	buf := make([]byte, 0, HelloSize+4) // At least 12 bytes may be immediately used
	buf = append(buf, 0, byte(kind))
	if kind == ClientMessageHello {
		buf = append(buf, HelloMagic...)
	}
	buf = append(buf, 0, 0)

	binary.BigEndian.PutUint16(buf[len(buf)-2:], uint16(len(payload)))

	if len(payload) > 0 {
		buf = append(buf, payload...)
	}
	return buf
}

func NewMessageAssembler() *MessageAssembler {
	return &MessageAssembler{
		buf:  make([]byte, 0, 128),
		size: 2,
	}
}

type AssemblerState int

const (
	stateZero AssemblerState = iota
	stateType
	stateHelloMagic
	stateSize
	statePayload
)

type MessageAssembler struct {
	buf    ClientMessage
	size   int
	state  AssemblerState
	bufLen int
}

func (m *MessageAssembler) assemble() ClientMessage {
	bLen := len(m.buf)
	buf := make([]byte, bLen)
	copy(buf, m.buf[:bLen])

	m.reset()

	return buf
}

func (m *MessageAssembler) reset() {
	m.size = 2 // Stores the size for reading the size itself
	m.state = stateZero
	m.buf = m.buf[:0]
	m.bufLen = 0
}

func (m *MessageAssembler) BufferSize() int { return m.bufLen }
func (m *MessageAssembler) ExpectedType() ClientMessageType {
	if m.bufLen < 2 {
		return ClientMessageInvalid
	}
	return m.buf.Type()
}

func (m *MessageAssembler) Feed(b byte) ClientMessage {
	m.buf = append(m.buf, b)
	m.bufLen = len(m.buf)
	switch m.state {
	case stateZero:
		if b != 0x00 {
			m.reset()
			break
		}
		m.state = stateType
	case stateType:
		if m.buf[1] == byte(ClientMessageHello) {
			m.state = stateHelloMagic
			break
		}
		m.state = stateSize
	case stateHelloMagic:
		if m.bufLen == 2+HelloSize {
			if !bytes.Equal(m.buf[2:], HelloMagic) {
				m.reset()
				break
			}
			m.state = stateSize
		}
	case stateSize:
		m.size -= 1
		if m.size == 0 {
			m.size = m.buf.PayloadSize()
			if m.size == 0 {
				return m.assemble()
			}
			m.state = statePayload
		}
	case statePayload:
		m.size -= 1
		if m.size == 0 {
			return m.assemble()
		}
	}
	return nil
}
