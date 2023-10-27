package common

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMessageAssembler(t *testing.T) {
	t.Run("Hello package, no namespace", func(t *testing.T) {
		data := []byte{
			0x00, 0x01, 0x00, 0x21, 0x55, 0x44, 0x50, 0x46,
			0x57, 0x00, 0x00, 0x00,
		}
		asm := NewMessageAssembler()
		var res ClientMessage
		for _, v := range data {
			res = asm.Feed(v)
		}
		require.NotNil(t, res)
		assert.Equal(t, ClientMessageHello, res.Type())
		assert.Equal(t, 0, res.PayloadSize())
		assert.Nil(t, res.Payload())

	})

	t.Run("Hello package, with namespace", func(t *testing.T) {
		data := []byte{
			0x00, 0x01, 0x00, 0x21, 0x55, 0x44, 0x50, 0x46,
			0x57, 0x00, 0x00, 0x06, 0x66, 0x6F, 0x6F, 0x62,
			0x61, 0x72,
		}
		asm := NewMessageAssembler()
		var res ClientMessage
		for _, v := range data {
			res = asm.Feed(v)
		}
		require.NotNil(t, res)
		assert.Equal(t, ClientMessageHello, res.Type())
		assert.Equal(t, 6, res.PayloadSize())
		assert.Equal(t, []byte("foobar"), res.Payload())
	})

	t.Run("Zero-len message", func(t *testing.T) {
		data := []byte{
			0x00, 0x02, 0x00, 0x00,
		}
		asm := NewMessageAssembler()
		var res ClientMessage
		for _, v := range data {
			res = asm.Feed(v)
		}
		require.NotNil(t, res)
		assert.Equal(t, ClientMessageAck, res.Type())
		assert.Equal(t, 0, res.PayloadSize())
		assert.Nil(t, res.Payload())
	})
}

func TestNewClientMessage(t *testing.T) {
	data := NewClientMessage(ClientMessageHello, []byte("foobar"))
	asm := NewMessageAssembler()
	var res ClientMessage
	for _, v := range data {
		res = asm.Feed(v)
	}
	require.NotNil(t, res)
	assert.Equal(t, ClientMessageHello, res.Type())
	assert.Equal(t, 6, res.PayloadSize())
	assert.Equal(t, []byte("foobar"), res.Payload())
}
