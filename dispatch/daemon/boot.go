package daemon

import (
	"github.com/udpfw/dispatch/config"
	"github.com/urfave/cli/v2"
	"os"
	"os/signal"
)

func Boot(cliCtx *cli.Context) error {
	opts, err := config.OptionsFrom(cliCtx)
	if err != nil {
		return cli.Exit(err, 1)
	}
	ctx, err := opts.IntoContext()
	if err != nil {
		return cli.Exit(err, 1)
	}

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt)
	srv := New(ctx)
	go srv.ArmShutdown(sigChan)
	return srv.Run()
}
