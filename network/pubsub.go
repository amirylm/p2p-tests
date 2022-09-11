package network

import (
	"context"
	"crypto/sha256"
	spectypes "github.com/bloxapp/ssv-spec/types"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"time"
)

// setupPubsub initialize the underlying pubsub router
func (n *Node) setupPubsub() error {
	psOpts := []pubsub.Option{
		pubsub.WithSeenMessagesTTL(32 * 12 * time.Second),
		pubsub.WithPeerOutboundQueueSize(512),
		pubsub.WithValidateQueueSize(512),
		pubsub.WithGossipSubParams(gossipSubParam()),
		pubsub.WithMessageIdFn(msgID),
	}
	// TODO: add scoring
	ps, err := pubsub.NewGossipSub(n.ctx, n.host, psOpts...)
	if err != nil {
		return errors.Wrap(err, "could not create pubsub router")
	}
	n.topics = newTopicManager(ps, nil, n.cfg.ValidationLatency)
	return nil
}

func (n *Node) Listen(pctx context.Context, t string) error {
	sub, exist, err := n.topics.Subscribe(t)
	if err != nil {
		return err
	}
	if exist {
		return nil
	}
	nodeLogger.With("name", n.cfg.Name, "peer", n.host.ID().String()).Debug("listening on topic ", t)
	ctx, cancel := context.WithCancel(pctx)
	defer cancel()

	for ctx.Err() == nil {
		pmsg, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// context is done
				return nil
			} else if err == pubsub.ErrSubscriptionCancelled || err == pubsub.ErrTopicClosed {
				// stop listening to topic
				return nil
			}
			n.errHandler(t, err)
			return err
		}
		if pmsg == nil {
			n.errHandler(t, errors.New("got empty message"))
			continue
		}
		msg, ok := pmsg.ValidatorData.(spectypes.SSVMessage)
		if !ok {
			n.errHandler(t, errors.New("could not covert message"))
			continue
		}
		n.msgHandler(t, msg)
	}
	return nil
}

func msgValidator(topicName string, latency time.Duration) msgValidatorFunc {
	return func(ctx context.Context, p peer.ID, pmsg *pubsub.Message) pubsub.ValidationResult {
		topic := pmsg.GetTopic()
		if topic != topicName {
			//reportValidationResult(validationResultTopic)
			return pubsub.ValidationReject
		}
		//metricsPubsubActiveMsgValidation.WithLabelValues(topic).Inc()
		//defer metricsPubsubActiveMsgValidation.WithLabelValues(topic).Dec()
		if len(pmsg.GetData()) == 0 {
			//reportValidationResult(validationResultNoData)
			return pubsub.ValidationReject
		}

		msg, err := decodeMsg(pmsg.GetData())
		if err != nil {
			//reportValidationResult(validationResultEncoding)
			return pubsub.ValidationReject
		}
		pmsg.ValidatorData = msg
		if latency > 0 {
			time.Sleep(latency)
		}
		return pubsub.ValidationAccept
	}
}

func gossipSubParam() pubsub.GossipSubParams {
	params := pubsub.DefaultGossipSubParams()
	params.D = 8
	params.Dlo = 6
	params.Dhi = 10
	params.Dlazy = 6
	params.HeartbeatInterval = 700 * time.Millisecond

	return params
}

func msgID(pmsg *pb.Message) string {
	msg := pmsg.GetData()
	if len(msg) == 0 {
		return ""
	}
	h := sha256.Sum256(msg)
	return string(h[20:])
}

type msgValidatorFunc = func(ctx context.Context, p peer.ID, msg *pubsub.Message) pubsub.ValidationResult

func decodeMsg(data []byte) (spectypes.SSVMessage, error) {
	var msg spectypes.SSVMessage
	err := msg.Decode(data)
	if err != nil {
		return msg, err
	}
	return msg, nil
}
