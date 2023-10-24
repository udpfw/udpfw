package pubsub

import (
	"fmt"
	"github.com/udpfw/dispatch/config"
)

var (
	ClosedErr         = fmt.Errorf("pubsub closed")
	AlreadyRunningErr = fmt.Errorf("pubsub is already running")
	BadBroadcastErr   = fmt.Errorf("#Broadcast called on inactive pubsub")
	NoConfigErr       = fmt.Errorf("no configuration provided for initializing a pubsub abstraction")
)

type PubSub interface {
	Start() error
	Broadcast(PacketData) error
	ReadNext() (PacketData, error)
	Shutdown() error
}

func NewPubSub(ctx *config.Context) (PubSub, error) {
	switch v := ctx.PubSubService.(type) {
	case nil:
		return nil, NoConfigErr
	case *config.RedisConfig:
		return newRedisPubSub(v)
	case *config.NATSConfig:
		return newNatsPubSub(v)
	default:
		panic("Not implemented")
	}
}
