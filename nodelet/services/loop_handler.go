package services

import (
	"hash"
	"hash/fnv"
	"sync"
	"time"
)

func NewLoopHandler() *LoopHandler {
	return &LoopHandler{
		messages:  make(map[string]time.Time),
		messageMu: sync.Mutex{},
		hashFn:    fnv.New64a(),
		stopCh:    make(chan bool),
		ticker:    nil,
	}
}

type LoopHandler struct {
	messages  map[string]time.Time
	messageMu sync.Mutex
	hashFn    hash.Hash64
	stopCh    chan bool
	ticker    *time.Ticker
}

func (l *LoopHandler) Stop() { close(l.stopCh) }

func (l *LoopHandler) Start() {
	l.ticker = time.NewTicker(10 * time.Second)
	go func() {
	loop:
		for {
			select {
			case <-l.ticker.C:
				l.messageMu.Lock()
				for h, t := range l.messages {
					if time.Now().Sub(t) > 2*time.Second {
						delete(l.messages, h)
					}
				}
				l.messageMu.Unlock()
			case <-l.stopCh:
				break loop
			}
		}
		l.ticker.Stop()
	}()
}

func (l *LoopHandler) hashPacket(pkt []byte) string {
	defer l.hashFn.Reset()
	_, _ = l.hashFn.Write(pkt)
	return string(l.hashFn.Sum(nil))
}

func (l *LoopHandler) RegisterPacket(pkt []byte) {
	digest := l.hashPacket(pkt)
	l.messageMu.Lock()
	defer l.messageMu.Unlock()
	l.messages[digest] = time.Now()
}

func (l *LoopHandler) ShouldDropPacket(pkt []byte) bool {
	digest := l.hashPacket(pkt)
	l.messageMu.Lock()
	defer l.messageMu.Unlock()

	t, ok := l.messages[digest]
	return ok && time.Now().Sub(t) < 2*time.Second
}
