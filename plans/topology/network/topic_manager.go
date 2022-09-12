package network

import (
	"context"
	logging "github.com/ipfs/go-log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"sync"
	"time"
)

var tmLogger = logging.Logger("p2p:topics")

// ScoreParamsFactory allows to configure scoring params per topic
type ScoreParamsFactory func(s string) *pubsub.TopicScoreParams

type TopicManager interface {
	Pubsub() *pubsub.PubSub

	ListPeers(t string) []peer.ID
	Publish(ctx context.Context, t string, data []byte) error
	Join(t string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error)
	Leave(t string) (*pubsub.Topic, error)
	Subscribe(t string, opts ...pubsub.SubOpt) (*pubsub.Subscription, bool, error)
}

type topicManager struct {
	ps *pubsub.PubSub

	mutex  *sync.RWMutex
	topics map[string]*pubsub.Topic
	subs   map[string]*pubsub.Subscription

	scoreParamsFactory ScoreParamsFactory

	validationLatency time.Duration
}

func newTopicManager(ps *pubsub.PubSub, scoreParams ScoreParamsFactory, validationLatency time.Duration) TopicManager {
	if scoreParams == nil {
		scoreParams = func(s string) *pubsub.TopicScoreParams {
			return nil
		}
	}
	return &topicManager{
		ps:                 ps,
		mutex:              &sync.RWMutex{},
		topics:             make(map[string]*pubsub.Topic),
		subs:               make(map[string]*pubsub.Subscription),
		scoreParamsFactory: scoreParams,
		validationLatency:  validationLatency,
	}
}

func (tm *topicManager) Pubsub() *pubsub.PubSub {
	return tm.ps
}

func (tm *topicManager) ListPeers(t string) []peer.ID {
	tm.mutex.RLock()
	topic, ok := tm.topics[t]
	tm.mutex.RUnlock()

	if !ok {
		return nil
	}

	return topic.ListPeers()
}

func (tm *topicManager) Publish(ctx context.Context, t string, data []byte) error {
	tm.mutex.RLock()
	topic, ok := tm.topics[t]
	tm.mutex.RUnlock()

	if !ok {
		return errors.Errorf("topic (%s) not found", t)
	}

	return topic.Publish(ctx, data)
}

func (tm *topicManager) Join(t string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	topic, ok := tm.topics[t]
	if ok {
		return topic, nil
	}
	tmLogger.Debug("joining topic ", t)
	return tm.join(t, opts...)
}

func (tm *topicManager) Leave(t string) (*pubsub.Topic, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tmLogger.Debug("leaving topic ", t)
	// first, cancel active subscriptions so we can close the topic
	sub, ok := tm.subs[t]
	if ok {
		delete(tm.subs, t)
		sub.Cancel()
	}
	topic, ok := tm.topics[t]
	if ok {
		delete(tm.topics, t)
		return topic, topic.Close()
	}
	return nil, nil
}

func (tm *topicManager) Subscribe(t string, opts ...pubsub.SubOpt) (*pubsub.Subscription, bool, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	sub, ok := tm.subs[t]
	if ok {
		return sub, true, nil
	}

	topic, ok := tm.topics[t]
	if !ok {
		var err error
		topic, err = tm.join(t)
		if err != nil {
			return nil, false, err
		}
	}

	sub, err := topic.Subscribe(opts...)
	if err != nil {
		return nil, false, err
	}
	tm.subs[t] = sub

	return sub, false, nil
}

func (tm *topicManager) join(t string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error) {
	topic, err := tm.ps.Join(t, opts...)
	if err != nil {
		return nil, err
	}
	// setup scoring and msg validator
	if p := tm.scoreParamsFactory(t); p != nil {
		if err := topic.SetScoreParams(p); err != nil {
			return topic, errors.Wrapf(err, "could not set topic score params for topic %s", t)
		}
	}
	_ = tm.ps.UnregisterTopicValidator(t)
	if err := tm.ps.RegisterTopicValidator(t, msgValidator(t, tm.validationLatency)); err != nil {
		return topic, errors.Wrapf(err, "could not set msg validator for topic %s", t)
	}
	tm.topics[t] = topic
	return topic, err
}
