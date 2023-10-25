package common

import "encoding/binary"

var HelloMessage = []byte("!UDPFW\x00")
var HelloSize = len(HelloMessage)

type ClientMessageType byte

const (
	ClientMessageInvalid ClientMessageType = iota
	ClientMessageAck
	ClientMessagePing
	ClientMessagePkt
	ClientMessageBye
)

type ClientMessage []byte

const MinClientMessageLen = 3 // type (1) + size (2)

func (c ClientMessage) Type() ClientMessageType {
	if len(c) < MinClientMessageLen {
		return ClientMessageInvalid
	}

	switch c[0] {
	case byte(ClientMessageAck):
		return ClientMessageAck
	case byte(ClientMessagePing):
		return ClientMessagePing
	case byte(ClientMessagePkt):
		return ClientMessagePkt
	case byte(ClientMessageBye):
		return ClientMessageBye
	default:
		return ClientMessageInvalid
	}
}

func (c ClientMessage) PayloadSize() uint16 {
	if len(c) < MinClientMessageLen { // type + uint16
		return 0 // Invalid packet!
	}
	return binary.BigEndian.Uint16(c[1:3])
}

func (c ClientMessage) Payload() []byte {
	size := c.PayloadSize()
	if size == 0 {
		return nil
	}
	return c[MinClientMessageLen : MinClientMessageLen+size]
}

func (c ClientMessage) Deconstruct() (ClientMessageType, []byte) {
	t := c.Type()
	if t == ClientMessageInvalid {
		return t, nil
	}
	return t, c.Payload()
}

func NewClientMessage(kind ClientMessageType, payload []byte) ClientMessage {
	buf := make([]byte, MinClientMessageLen+len(payload))
	buf[0] = byte(kind)
	binary.BigEndian.PutUint16(buf[1:], uint16(len(payload)))
	if len(payload) > 0 {
		copy(buf[MinClientMessageLen:], payload)
	}
	return buf
}

func NewMessageAssembler() *MessageAssembler {
	return &MessageAssembler{
		buf: make([]byte, 0, 128),
	}
}

type AssemblerState int

const (
	stateType AssemblerState = iota
	stateSize
	statePayload
)

type MessageAssembler struct {
	buf   ClientMessage
	size  uint16
	state AssemblerState
}

func (m *MessageAssembler) assemble() ClientMessage {
	bLen := len(m.buf)
	buf := make([]byte, bLen)
	copy(buf, m.buf[:bLen])

	m.size = 0
	m.state = stateType
	m.buf = m.buf[:0]

	return buf
}

func (m *MessageAssembler) Feed(b byte) ClientMessage {
	m.buf = append(m.buf, b)
	bufLen := uint16(len(m.buf))
	switch m.state {
	case stateType:
		m.state = stateSize
	case stateSize:
		if bufLen == 3 {
			m.size = m.buf.PayloadSize()
			if m.size == 0 {
				return m.assemble()
			}
			m.state = statePayload
		}
	case statePayload:
		if bufLen-3 == m.size {
			return m.assemble()
		}
	}
	return nil
}
