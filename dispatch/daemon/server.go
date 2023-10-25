package daemon

import (
	"errors"
	"fmt"
	"github.com/udpfw/dispatch/config"
	"github.com/udpfw/dispatch/pubsub"
	"github.com/udpfw/dispatch/tcp"
	"go.uber.org/zap"
	"os"
	"sync/atomic"
)

func New(ctx *config.Context) *Daemon {
	return &Daemon{
		bind:     ctx.BindAddress,
		ctx:      ctx,
		stopping: &atomic.Bool{},
		stopped:  make(chan bool),
	}
}

type Daemon struct {
	bind     string
	psConfig any
	ctx      *config.Context
	ps       pubsub.PubSub
	tcp      *tcp.Server
	log      *zap.Logger
	stopping *atomic.Bool
	stopped  chan bool
}

func (s *Daemon) ArmShutdown(sigChan chan os.Signal) {
	<-sigChan
	s.log.Info("Received shutdown signal. Notifying clients and draining queues...")
	s.Shutdown()
}

func (s *Daemon) Shutdown() {
	if s.stopping.Swap(true) {
		return
	}

	s.tcp.Shutdown()
	s.log.Info("TCP shutdown completed. Now draining queues...")
	if err := s.ps.Shutdown(); err != nil {
		s.log.Error("CRITICAL: PubSub shutdown failed", zap.Error(err))
	}
	s.log.Info("Drain complete")
	s.log.Info("Bye!")
	close(s.stopped)
}

func (s *Daemon) Run() error {
	fmt.Println("Initialize log...")
	if err := config.InitializeLogging(s.ctx); err != nil {
		return err
	}
	log := zap.L().With(zap.String("facility", "Daemon"))
	s.log = log
	log.Debug("Debug logging is enabled")

	log.Info("Initialize PubSub...")
	ps, err := pubsub.NewPubSub(s.ctx)
	if errors.Is(err, pubsub.NoConfigErr) {
		return fmt.Errorf("no pubsub configuration provided. Provide either --redis-url or --nats-url")
	} else if err != nil {
		return err
	}
	s.ps = ps
	if err = ps.Start(); err != nil {
		log.Error("Failed starting pubsub", zap.Error(err))
		return err
	}

	log.Info("Initialize TCP Server...")
	srv, err := tcp.New(s.ctx, ps)
	if err != nil {
		log.Error("Failed initializing TCP server", zap.Error(err))
		if err := ps.Shutdown(); err != nil {
			log.Error("Failed shutting-down PubSub after previous failure", zap.Error(err))
		}
		return fmt.Errorf("failed initializing TCP server: %w", err)
	}
	s.tcp = srv

	log.Info("Now listening", zap.String("address", s.ctx.BindAddress))

	if err = srv.Run(); err != nil {
		log.Error("TCP Server emitted failure during listen", zap.Error(err))
	}
	s.Shutdown()
	<-s.stopped
	return nil
}
