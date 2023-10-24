package config

import (
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"testing"
)

type OptionFn func() []string

func makeOptions(t *testing.T, opts ...OptionFn) *AllOptions {
	args := []string{""}
	for _, fn := range opts {
		args = append(args, fn()...)
	}
	var ctx *cli.Context
	flags, err := MakeCLIFlags()
	require.NoError(t, err)
	app := cli.App{
		Flags: flags,
		Action: func(context *cli.Context) error {
			ctx = context
			return nil
		},
	}
	err = app.Run(args)
	require.NoError(t, err)
	appOpts, err := OptionsFrom(ctx)
	require.NoError(t, err)
	return &appOpts
}

func WithBind(v string) OptionFn    { return func() []string { return []string{"--bind", v} } }
func WithAnyBind() OptionFn         { return WithBind("foo") }
func WithDebug() OptionFn           { return func() []string { return []string{"--debug"} } }
func WithNatsURL(v string) OptionFn { return func() []string { return []string{"--nats-url", v} } }
func WithAnyNatsURL() OptionFn      { return WithNatsURL("foo") }
func WithNatsSubscriptionSubject(v string) OptionFn {
	return func() []string { return []string{"--nats-subscription-subject", v} }
}
func WithAnyNatsSubscriptionSubject() OptionFn { return WithNatsSubscriptionSubject("foo") }
func WithNatsUserCredentials(v string) OptionFn {
	return func() []string { return []string{"--nats-user-credentials", v} }
}
func WithAnyNatsUserCredentials() OptionFn { return WithNatsUserCredentials("foo") }
func WithNatsUserCredentialsNKey(v string) OptionFn {
	return func() []string { return []string{"--nats-user-credentials-nkey", v} }
}
func WithAnyNatsUserCredentialsNKey() OptionFn { return WithNatsUserCredentialsNKey("foo") }
func WithNatsNkeySeed(v string) OptionFn {
	return func() []string { return []string{"--nats-nkey-from-seed", v} }
}
func WithAnyNatsNkeySeed() OptionFn { return WithNatsNkeySeed("foo") }
func WithNatsRootCA(v string) OptionFn {
	return func() []string { return []string{"--nats-root-ca", v} }
}
func WithAnyNatsRootCA() OptionFn { return WithNatsRootCA("foo") }
func WithNatsClientCertificate(v string) OptionFn {
	return func() []string { return []string{"--nats-client-certificate", v} }
}
func WithAnyNatsClientCertificate() OptionFn { return WithNatsClientCertificate("foo") }
func WithNatsClientKey(v string) OptionFn {
	return func() []string { return []string{"--nats-client-key", v} }
}
func WithAnyNatsClientKey() OptionFn { return WithNatsClientKey("foo") }
func WithRedisURL(v string) OptionFn { return func() []string { return []string{"--redis-url", v} } }
func WithAnyRedisURL() OptionFn      { return WithRedisURL("foo") }
func WithRedisPubsubChannel(v string) OptionFn {
	return func() []string { return []string{"--redis-pubsub-channel", v} }
}
func WithAnyRedisPubsubChannel() OptionFn { return WithRedisPubsubChannel("foo") }
