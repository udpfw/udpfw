package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"testing"
)

func TestMakeCLIFlags(t *testing.T) {
	_, err := MakeCLIFlags()
	require.NoError(t, err)
}

func TestMakeFlagsFrom(t *testing.T) {
	assertUsage := func(f *cli.StringFlag, name string) { assert.Equal(t, name+" usage", f.Usage) }
	assertRequired := func(f *cli.StringFlag, required bool) { assert.Equal(t, required, f.Required) }
	assertName := func(f *cli.StringFlag, name string) { assert.Equal(t, name, f.Name) }
	assertEnvs := func(f *cli.StringFlag, name string) {
		assert.Equal(t, []string{"UDPFW_DISPATCH_" + name, "DISPATCH_" + name}, f.EnvVars)
	}
	assertTakesFile := func(f *cli.StringFlag, flag bool) { assert.Equal(t, flag, f.TakesFile) }
	assertCategory := func(f *cli.StringFlag, name string) { assert.Equal(t, name, f.Category) }
	assertValue := func(f *cli.StringFlag, value string) { assert.Equal(t, value, f.Value) }

	type str struct {
		Required string    `name:"bind" usage:"bind usage" env:"bInD"`
		Optional *string   `name:"foo" usage:"foo usage" env:"FOO"`
		File     *FilePath `name:"bar" usage:"bar usage" env:"BAR"`
		Value    string    `name:"baz" usage:"baz usage" env:"BAZ" value:"test"`
		Category string    `name:"fuz" usage:"fuz usage" env:"FUZ" category:"bla"`
	}

	flags, err := makeCLIFlagsFrom[str]()
	require.NoError(t, err)

	required := flags[0].(*cli.StringFlag)
	assertUsage(required, "bind")
	assertRequired(required, true)
	assertName(required, "bind")
	assertEnvs(required, "BIND")
	assertTakesFile(required, false)
	assertCategory(required, "")
	assertValue(required, "")

	optional := flags[1].(*cli.StringFlag)
	assertUsage(optional, "foo")
	assertRequired(optional, false)
	assertName(optional, "foo")
	assertEnvs(optional, "FOO")
	assertTakesFile(optional, false)
	assertCategory(optional, "")
	assertValue(optional, "")

	file := flags[2].(*cli.StringFlag)
	assertUsage(file, "bar")
	assertRequired(file, false)
	assertName(file, "bar")
	assertEnvs(file, "BAR")
	assertTakesFile(file, true)
	assertCategory(file, "")
	assertValue(file, "")

	value := flags[3].(*cli.StringFlag)
	assertUsage(value, "baz")
	assertRequired(value, true)
	assertName(value, "baz")
	assertEnvs(value, "BAZ")
	assertTakesFile(value, false)
	assertCategory(value, "")
	assertValue(value, "test")

	category := flags[4].(*cli.StringFlag)
	assertUsage(category, "fuz")
	assertRequired(category, true)
	assertName(category, "fuz")
	assertEnvs(category, "FUZ")
	assertTakesFile(category, false)
	assertCategory(category, "bla")
	assertValue(category, "")
}

func TestContextFrom(t *testing.T) {
	input := []string{
		"",
		"--bind", "bind",
		"--nats-url", "nats_url",
		"--nats-user-credentials", "/foo/bar",
	}
	flags, err := MakeCLIFlags()
	require.NoError(t, err)
	var opts AllOptions

	app := cli.App{
		Flags: flags,
		Action: func(context *cli.Context) error {
			opts, err = OptionsFrom(context)
			return nil
		},
	}
	require.NoError(t, app.Run(input))

	assert.Equal(t, "bind", opts.Bind)
	assert.Equal(t, "nats_url", *opts.NatsURL)
	assert.Equal(t, FilePath("/foo/bar"), *opts.NatsUserCredentials)

	assert.Equal(t, "udpfw-dispatch-exchange", *opts.NatsSubscriptionSubject)
	assert.Equal(t, "udpfw-dispatch-exchange", *opts.RedisPubsubChannel)
}
