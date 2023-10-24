package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func getOptsError(t *testing.T, opts ...OptionFn) error {
	_, err := makeOptions(t, opts...).IntoContext()
	return err
}

func getOpts(t *testing.T, opts ...OptionFn) *Context {
	o, err := makeOptions(t, opts...).IntoContext()
	require.NoError(t, err)
	return o
}

func TestAllOptions_IntoContext(t *testing.T) {
	t.Run("with both urls", func(t *testing.T) {
		err := getOptsError(t, WithAnyBind(), WithAnyNatsURL(), WithAnyRedisURL())
		assert.ErrorContains(t, err, "define either")
	})

	t.Run("with user credentials nkey, no user credentials", func(t *testing.T) {
		err := getOptsError(t, WithAnyBind(), WithAnyNatsURL(), WithAnyNatsUserCredentialsNKey())
		assert.ErrorContains(t, err, "must be used with --nats-user-credentials")
	})

	t.Run("with only nats url", func(t *testing.T) {
		o := getOpts(t, WithAnyBind(), WithNatsURL("test"))
		assert.Equal(t, "test", o.PubSubService.(*NATSConfig).URL)
	})
}
