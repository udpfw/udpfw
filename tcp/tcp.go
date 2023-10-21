package tcp

import (
	"errors"
	"github.com/nats-io/nuid"
	"github.com/udpfw/dispatch/config"
	"github.com/udpfw/dispatch/pubsub"
	"go.uber.org/zap"
	"net"
	"sync"
	"time"
)

func New(ctx *config.Context, pubSub pubsub.PubSub) (*Server, error) {
	listener, err := net.Listen("tcp", ctx.BindAddress)
	if err != nil {
		return nil, err
	}

	return &Server{
		log:         zap.L().With(zap.String("facility", "TCP")),
		listener:    listener,
		clients:     map[string]*Client{},
		clientsLock: &sync.Mutex{},
		idGen:       nuid.New(),
		pubSub:      pubSub,
		wg:          &sync.WaitGroup{},
	}, nil
}

type Server struct {
	listener    net.Listener
	clients     map[string]*Client
	clientsLock *sync.Mutex
	log         *zap.Logger
	idGen       *nuid.NUID
	pubSub      pubsub.PubSub
	wg          *sync.WaitGroup
}

func (s *Server) CountConnected() int {
	s.clientsLock.Lock()
	defer s.clientsLock.Unlock()
	return len(s.clients)
}

func (s *Server) emitBroadcast(id string, data ClientMessage) {
	pkt := pubsub.MakePacket(id, data)
	if err := s.pubSub.Broadcast(pkt); err != nil {
		s.log.Error("CRITICAL: Failed emitting broadcast",
			zap.String("client", id),
			zap.ByteString("payload", data),
			zap.Error(err))
	}
}

func (s *Server) unregisterClient(id string) {
	s.clientsLock.Lock()
	delete(s.clients, id)
	s.clientsLock.Unlock()
	s.wg.Done()
	s.log.Debug("Deregistered client", zap.String("id", id))
}

func (s *Server) Run() error {
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
		broadcastFn := func(msg ClientMessage) {
			s.emitBroadcast(id, msg)
		}
		doneFn := func() {
			s.unregisterClient(id)
		}
		c := NewClient(id, conn, doneFn, broadcastFn)
		s.clientsLock.Lock()
		s.clients[id] = c
		s.wg.Add(1)
		s.clientsLock.Unlock()
		c.log.Debug("Registered new client", zap.String("id", id), zap.String("addr", conn.RemoteAddr().String()))
		go c.service()
	}
}

func (s *Server) Shutdown() {
	s.log.Info("Stopping listener...")
	if err := s.listener.Close(); err != nil {
		s.log.Error("Failed stopping listener", zap.Error(err))
	}

	s.log.Info("Dispatching shutdown packet to clients")
	s.clientsLock.Lock()
	for id, c := range s.clients {
		c.Write(NewClientMessage(ClientMessageBye, nil))
		s.log.Debug("Dispatched shutdown", zap.String("client", id))
	}
	s.clientsLock.Unlock()

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
