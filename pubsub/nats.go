package pubsub

import (
	"github.com/nats-io/nats.go"
	"github.com/udpfw/dispatch/config"
	"go.uber.org/zap"
	"sync/atomic"
)

func newNatsPubSub(options *config.NATSConfig) (PubSub, error) {
	nc, err := nats.Connect(options.URL, options.ConnectionOptions...)
	if err != nil {
		return nil, err
	}

	client := &natsPubSub{
		conn:    nc,
		subject: options.Subject,
		running: &atomic.Bool{},
		log:     zap.L().With(zap.String("facility", "NatsPubSub")),
	}
	return client, nil
}

type natsPubSub struct {
	conn         *nats.Conn
	subscription *nats.Subscription
	msgChan      chan *nats.Msg
	subject      string
	running      *atomic.Bool
	log          *zap.Logger
}

func (n *natsPubSub) Start() error {
	if n.running.Swap(true) {
		return AlreadyRunningErr
	}

	n.log.Info("Registering interest in subject", zap.String("subject", n.subject))

	msgChannel := make(chan *nats.Msg, 256)
	sub, err := n.conn.ChanSubscribe(n.subject, msgChannel)
	if err != nil {
		n.conn.Close()
		return err
	}
	n.log.Debug("Registered interest. Writes and reads are being serviced in background.")

	n.msgChan = msgChannel
	n.subscription = sub
	return nil
}

func (n *natsPubSub) Broadcast(data PacketData) error {
	if !n.running.Load() {
		return BadBroadcastErr
	}

	if err := n.conn.Publish(n.subject, data); err != nil {
		n.log.Error("CRITICAL: Failed publishing object",
			zap.ByteString("data", data),
			zap.Error(err))
		return err
	}

	return nil
}

func (n *natsPubSub) ReadNext() (PacketData, error) {
	if !n.running.Load() {
		return nil, ClosedErr
	}

	msg := <-n.msgChan
	if msg == nil {
		// Subscriber has stopped. Bail.
		return nil, ClosedErr
	}

	return msg.Data, nil
}

func (n *natsPubSub) Shutdown() error {
	defer n.conn.Close()

	if !n.running.Swap(false) {
		return nil
	}

	if err := n.subscription.Drain(); err != nil {
		return err
	}

	return nil
}
