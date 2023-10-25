package main

import (
	"fmt"
	"github.com/udpfw/nodelet/log"
	"github.com/udpfw/nodelet/services"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"os"
)

func main() {
	app := cli.App{
		Name: "udpfw",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "iface",
				Usage:    "The interface name from which UDP packets will be captured",
				EnvVars:  []string{"UDPFW_NODELET_IFACE", "NODELET_IFACE"},
				Required: true,
			},
			&cli.StringFlag{
				Name:    "dispatch-address",
				Usage:   "IP and port to the Dispatch service running on the local cluster",
				EnvVars: []string{"UDPFW_DISPATCH_ADDRESS", "NODELET_DISPATCH_ADDRESS"},
				Value:   "udpfw-dispatch.svc.cluster.local",
			},
			&cli.BoolFlag{
				Name:    "debug",
				Usage:   "Enables debug logging",
				EnvVars: []string{"UDPFW_NODELET_DEBUG", "NODELET_DEBUG"},
			},
		},
		Action: func(ctx *cli.Context) error {
			fmt.Println("Initialize logging...")
			if err := log.InitializeLogging(ctx.Bool("debug")); err != nil {
				panic(err)
			}

			logger := zap.L()

			iface := ctx.String("iface")
			logger.Info("Initialize packet handler...", zap.String("iface", iface))
			handler, err := services.NewPacketHandler(iface)
			if err != nil {
				logger.Fatal("Failed initializing packet handler", zap.Error(err))
			}
			go func() {
				if err := handler.Start(); err != nil {
					logger.Error("Error during handler operation", zap.Error(err))
					handler.Shutdown()
				}
			}()
			defer handler.Shutdown()

			logger.Info("Packet handler initialization complete")

			addrs := ctx.String("dispatch-address")
			logger.Info("Initialize Dispatch connector", zap.String("address", addrs))
			dispatch := services.NewDispatch(addrs)

			emitterDone := make(chan bool)
			go func() {
				defer close(emitterDone)
				for {
					pkt := handler.NextPacket()
					if err := dispatch.Write(pkt); err != nil {
						logger.Error("Failed enqueueing packet", zap.ByteString("data", pkt))
					}
				}
			}()

			injectorDone := make(chan bool)
			go func() {
				defer close(injectorDone)
				for pkt := range dispatch.OnPacket {
					if err := handler.Inject(pkt); err != nil {
						// TODO: Log
					}
				}
			}()
			logger.Info("Dispatch connector initialization complete")
			go dispatch.Run()

			select {
			case <-emitterDone:
			case <-injectorDone:
			}

			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
