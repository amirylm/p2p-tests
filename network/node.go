package network

import (
	"context"
	crand "crypto/rand"
	"fmt"
	spectypes "github.com/bloxapp/ssv-spec/types"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	libp2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"sync"
	"time"
)

var nodeLogger = logging.Logger("p2p:node")

type MsgHandler func(string, spectypes.SSVMessage)
type ErrHandler func(string, error)

type NodeConfig struct {
	Name        string
	ListenAddrs []string

	MsgHandler MsgHandler
	ErrHandler ErrHandler
	MaxPeers   int

	ValidationLatency time.Duration
}

type Node struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg *NodeConfig
	// components
	host   host.Host
	topics TopicManager
	// handlers
	msgHandler MsgHandler
	errHandler ErrHandler
}

// NewNode creates a new Node instance with the underlying p2p components
func NewNode(pctx context.Context, cfg *NodeConfig) (*Node, error) {
	logger := nodeLogger.With("name", cfg.Name)
	logger.Debug("creating libp2p host")
	sk, _, err := crypto.GenerateSecp256k1Key(crand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate private key")
	}
	h, err := libp2p.New(
		libp2p.Identity(sk),
		libp2p.ListenAddrStrings(cfg.ListenAddrs...),
		libp2p.UserAgent(fmt.Sprintf("test/topology/%s", cfg.Name)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not create libp2p host")
	}
	ctx, cancel := context.WithCancel(pctx)
	n := Node{
		ctx:        ctx,
		cancel:     cancel,
		cfg:        cfg,
		host:       h,
		msgHandler: cfg.MsgHandler,
		errHandler: cfg.ErrHandler,
	}
	logger = logger.With("peer", h.ID().String())
	if err := n.setupPubsub(); err != nil {
		logger.Warn("could not create pubsub router: ", err)
		if closeErr := n.Close(); closeErr != nil {
			return nil, errors.Wrap(closeErr, err.Error())
		}
		return nil, err
	}

	ntf, gc := notiffee(h.Network(), cfg.MaxPeers)
	go func() {
		for ctx.Err() == nil {
			time.Sleep(time.Minute)
			gc()
		}
	}()
	h.Network().Notify(ntf)

	logger.Info("node is ready")

	return &n, nil
}

func (n *Node) Host() host.Host {
	return n.host
}

func (n *Node) TopicManager() TopicManager {
	return n.topics
}

func (n *Node) Close() error {
	n.cancel()
	return n.host.Close()
}

func notiffee(net libp2pnetwork.Network, maxPeers int) (*libp2pnetwork.NotifyBundle, func()) {
	if maxPeers == 0 {
		maxPeers = 25
	}
	connected := 0
	connectedCache := map[peer.ID]bool{}
	l := &sync.RWMutex{}

	gc := func() {
		l.Lock()
		defer l.Unlock()

		toRemove := make([]peer.ID, 0)
		for pid := range connectedCache {
			switch net.Connectedness(pid) {
			case libp2pnetwork.CannotConnect, libp2pnetwork.NotConnected:
				toRemove = append(toRemove, pid)
			default:
			}
		}
		for _, pid := range toRemove {
			delete(connectedCache, pid)
		}
	}

	return &libp2pnetwork.NotifyBundle{
		ConnectedF: func(n libp2pnetwork.Network, c libp2pnetwork.Conn) {
			l.Lock()
			defer l.Unlock()

			pid := c.RemotePeer()
			if _, ok := connectedCache[pid]; !ok {
				if connected > maxPeers {
					if err := n.ClosePeer(pid); err != nil {
						nodeLogger.Warnf("could not disconnect from peer %s: %s", pid.String(), err.Error())
					}
					return
				}
				connected++
				connectedCache[pid] = true
				//metricConnections.WithLabelValues(selfID).Inc()
				nodeLogger.Debugf("new connected peer %s", pid.String())
			}
		},
		DisconnectedF: func(n libp2pnetwork.Network, c libp2pnetwork.Conn) {
			l.Lock()
			defer l.Unlock()

			pid := c.RemotePeer()
			if n.Connectedness(pid) == libp2pnetwork.Connected {
				return
			}
			if _, ok := connectedCache[pid]; ok {
				connected--
				delete(connectedCache, pid)
				//metricConnections.WithLabelValues(selfID).Dec()
				nodeLogger.Debugf("disconnected peer %s", pid.String())
			}
		},
	}, gc
}
