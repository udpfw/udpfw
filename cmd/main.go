package main

import (
	"github.com/udpfw/dispatch/config"
	"github.com/udpfw/dispatch/daemon"
	"os"
)
import "github.com/urfave/cli/v2"

func main() {
	opts, err := config.MakeCLIFlags()
	if err != nil {
		panic(err)
	}
	app := cli.App{
		Name:            "udpfw-dispatch",
		HelpName:        "udpfw-dispatch",
		Usage:           "UDPfw dispatch daemon",
		HideHelpCommand: true,
		Flags:           opts,
		Action:          daemon.Boot,
		Authors: []*cli.Author{
			{Name: "Vito Sartori", Email: "hey@vito.io"},
		},
		Copyright: "Copyright (C) The udpfw authors",
	}

	// TODO: Handle this
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
