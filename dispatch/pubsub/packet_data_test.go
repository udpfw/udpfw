package pubsub

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

var src = "0123456789ABCDEFGHIJKL"

func TestMakePacket(t *testing.T) {
	packet := MakePacket(src, "ns", []byte("payload"))
	assert.Equal(t, src, packet.Source())
	assert.Equal(t, 2, packet.NamespaceLength())
	assert.Equal(t, "ns", packet.Namespace())
	assert.Equal(t, 7, packet.PayloadLength())
	assert.Equal(t, []byte("payload"), packet.Payload())
}
