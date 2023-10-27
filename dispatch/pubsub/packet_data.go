package pubsub

import "encoding/binary"

const sourceLength = 22

type PacketData []byte

func (p PacketData) Source() string { return string(p[0:sourceLength]) }

func (p PacketData) NamespaceLength() int {
	return int(binary.BigEndian.Uint16(p[sourceLength:]))
}

func (p PacketData) Namespace() string {
	return string(p[sourceLength+2 : sourceLength+2+p.NamespaceLength()])
}

func (p PacketData) PayloadLength() int {
	return int(binary.BigEndian.Uint16(p[sourceLength+2+p.NamespaceLength():]))
}

func (p PacketData) Payload() []byte {
	offset := sourceLength + 2 + p.NamespaceLength() + 2
	return p[offset : offset+p.PayloadLength()]
}

func (p PacketData) Deconstruct() (source string, namespace string, payload []byte) {
	return p.Source(), p.Namespace(), p.Payload()
}

func MakePacket(source string, ns string, payload []byte) PacketData {
	sourceLen := len(source)
	if sourceLen != sourceLength {
		panic("Source must have 22 bytes")
	}

	nsLen := len(ns)
	payloadLen := len(payload)

	packet := make([]byte, sourceLen+nsLen+payloadLen+4)
	cursor := 0
	copy(packet, source)
	cursor += sourceLen

	binary.BigEndian.PutUint16(packet[cursor:], uint16(nsLen))
	cursor += 2

	copy(packet[cursor:], ns)
	cursor += nsLen

	binary.BigEndian.PutUint16(packet[cursor:], uint16(payloadLen))
	cursor += 2

	copy(packet[cursor:], payload)
	return packet
}
