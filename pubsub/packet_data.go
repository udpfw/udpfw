package pubsub

type PacketData []byte

func (p PacketData) Source() string { return string(p[0:22]) }

func (p PacketData) Payload() []byte { return p[22:] }

func (p PacketData) Deconstruct() (string, []byte) { return p.Source(), p.Payload() }

func MakePacket(source string, payload []byte) PacketData {
	return append([]byte(source), payload...)
}
