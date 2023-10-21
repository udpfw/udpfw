package pubsub

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"github.com/udpfw/dispatch/config"
	"go.uber.org/zap"
	"sync"
	"sync/atomic"
	"time"
)

func newRedisPubSub(options *config.RedisConfig) (PubSub, error) {
	conn := redis.NewClient(options.Options)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &redisPubSub{
		log:     zap.L().With(zap.String("facility", "RedisPubSub")),
		conn:    conn,
		channel: options.Channel,
		running: &atomic.Bool{},
	}, nil
}

type redisPubSub struct {
	conn       *redis.Client
	channel    string
	running    *atomic.Bool
	msgChan    <-chan *redis.Message
	pubSub     *redis.PubSub
	sendQueue  chan []byte
	writerDone *sync.WaitGroup
	log        *zap.Logger
}

func (r *redisPubSub) Start() error {
	if r.running.Swap(true) {
		return AlreadyRunningErr
	}

	r.log.Info("Subscribing to channel", zap.String("name", r.channel))

	pubsub := r.conn.Subscribe(context.Background(), r.channel)
	iface, err := pubsub.Receive(context.Background())
	if err != nil {
		return err
	}
	if _, ok := iface.(*redis.Subscription); !ok {
		_ = pubsub.Close() // Ignoring error since we are going down.
		return fmt.Errorf("failed starting subscription, expected subscription confirmation, but found %#v instead", iface)
	}
	r.log.Debug("Subscribed to channel and received ack. Now servicing writes...")

	r.pubSub = pubsub
	r.msgChan = pubsub.Channel()
	r.sendQueue = make(chan []byte, 256)
	r.writerDone = &sync.WaitGroup{}
	r.writerDone.Add(1)
	go r.serviceWrites()
	return nil
}

func (r *redisPubSub) Broadcast(data PacketData) error {
	if !r.running.Load() {
		return BadBroadcastErr
	}
	r.sendQueue <- data
	return nil
}

func (r *redisPubSub) ReadNext() (PacketData, error) {
	if !r.running.Load() {
		return nil, ClosedErr
	}

	msg := <-r.msgChan
	if msg == nil {
		return nil, ClosedErr
	}

	return []byte(msg.Payload), nil

}

func (r *redisPubSub) Shutdown() error {
	if !r.running.Swap(false) {
		return nil
	}
	r.log.Info("Shutdown called")
	r.log.Debug("Closing internal send queue")
	close(r.sendQueue)
	r.log.Debug("Draining internal send queue")
	r.writerDone.Wait()
	r.log.Debug("Closing PubSub subscription")
	if err := r.pubSub.Close(); err != nil {
		return err
	}

	return r.conn.Close()
}

func (r *redisPubSub) serviceWrites() {
	for data := range r.sendQueue {
		if err := r.conn.Publish(context.Background(), r.channel, data).Err(); err != nil {
			r.log.Error("CRITICAL: Failed publishing object",
				zap.ByteString("data", data),
				zap.Error(err))
		}
	}
	r.log.Debug("Send queue drained. Stop servicing writes.")
	r.writerDone.Done()
}
