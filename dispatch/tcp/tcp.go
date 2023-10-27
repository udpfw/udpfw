package tcp

import (
	"errors"
	"github.com/nats-io/nuid"
	"github.com/udpfw/common"
	"github.com/udpfw/dispatch/config"
	"github.com/udpfw/dispatch/pubsub"
	"go.uber.org/zap"
	"net"
	"os"
	"sync"
	"time"
)

func New(ctx *config.Context, pubSub pubsub.PubSub) (*Server, error) {
	listener, err := net.Listen("tcp", ctx.BindAddress)
	if err != nil {
		return nil, err
	}

	log := zap.L().With(zap.String("facility", "TCP"))

	hostname, err := os.Hostname()
	if err != nil {
		log.Warn("Failed obtaining hostname", zap.Error(err))
		hostname = "<unknown>"
	}

	return &Server{
		hostname:   hostname,
		log:        log,
		listener:   listener,
		clients:    &ClientMap{},
		namespaces: &NSMap{},
		idGen:      nuid.New(),
		pubSub:     pubSub,
		wg:         &sync.WaitGroup{},
	}, nil
}

type Server struct {
	listener   net.Listener
	clients    *ClientMap
	log        *zap.Logger
	idGen      *nuid.NUID
	pubSub     pubsub.PubSub
	wg         *sync.WaitGroup
	hostname   string
	namespaces *NSMap
}

func (s *Server) CountConnected() int {
	return s.clients.Len()
}

func (s *Server) emitBroadcast(id string, ns string, data common.ClientMessage) {
	pkt := pubsub.MakePacket(id, ns, data)
	if err := s.pubSub.Broadcast(pkt); err != nil {
		s.log.Error("CRITICAL: Failed emitting broadcast",
			zap.String("client", id),
			zap.ByteString("payload", data),
			zap.Error(err))
	}
}

func (s *Server) unregisterClient(id string) {
	s.clients.Delete(id)
	s.wg.Done()
	s.log.Debug("Deregistered client", zap.String("id", id))
}

func (s *Server) dispatchPubSubMessage(msg pubsub.PacketData) {
	src, ns, data := msg.Deconstruct()
	for _, cli := range s.namespaces.Get(ns) {
		if cli.id != src {
			cli.Write(data)
		}
	}
}

func (s *Server) Run() error {
	go func() {
		for {
			msg, err := s.pubSub.ReadNext()
			if errors.Is(err, pubsub.ClosedErr) {
				s.log.Info("Pubsub closed. Will stop iterating packets.")
				return
			}
			s.dispatchPubSubMessage(msg)
		}
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			s.log.Error("Failed accepting client", zap.Error(err))
			return err
		}

		id := s.idGen.Next()
		c := NewClient(s, id, conn)
		s.clients.RegisterClient(id, c)
		s.wg.Add(1)
		c.log.Debug("Registered new client",
			zap.String("id", id),
			zap.String("addr", conn.RemoteAddr().String()))
		go c.service()
	}
}

func (s *Server) RequestBroadcast(client *Client, msg common.ClientMessage) {
	s.emitBroadcast(client.id, *client.ns, msg)
}

func (s *Server) SignalDone(client *Client) {
	s.unregisterClient(client.id)
	if client.ns != nil {
		s.namespaces.Delete(*client.ns, client)
	}
}

func (s *Server) AssocNamespace(client *Client, ns string) {
	s.namespaces.Add(ns, client)
}

func (s *Server) Shutdown() {
	s.log.Info("Stopping listener...")
	if err := s.listener.Close(); err != nil {
		s.log.Error("Failed stopping listener", zap.Error(err))
	}

	s.log.Info("Dispatching shutdown packet to clients")
	s.clients.Range(func(id string, c *Client) bool {
		c.Write(common.NewClientMessage(common.ClientMessageBye, nil))
		s.log.Debug("Dispatched shutdown", zap.String("client", id))
		return true
	})

	s.log.Info("Waiting for clients to drain...")
	tick := time.NewTicker(3 * time.Second)
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				left := s.CountConnected()
				s.log.Info("Still waiting clients drainage", zap.Int("clients_left", left))
			}
		}
	}()
	s.wg.Wait()
	close(done)
}
