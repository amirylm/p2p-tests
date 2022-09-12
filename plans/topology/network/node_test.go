package network

import (
	"context"
	"fmt"
	spectypes "github.com/bloxapp/ssv-spec/types"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, logging.SetLogLevelRegex("p2p:*", "debug"))
	n := 10
	var msgCount int64
	var errCount int64
	msgHandler := func(name string) MsgHandler {
		return func(s string, message spectypes.SSVMessage) {
			atomic.AddInt64(&msgCount, 1)
		}
	}
	errHandler := func(name string) ErrHandler {
		return func(s string, err error) {
			atomic.AddInt64(&errCount, 1)
		}
	}
	var nodes []*Node
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("testnode-%d", i)
		node, err := NewNode(ctx, &NodeConfig{
			Name:        name,
			ListenAddrs: []string{"/ip4/0.0.0.0/tcp/0"},
			MsgHandler:  msgHandler(name),
			ErrHandler:  errHandler(name),
		})
		require.NoError(t, err)
		nodes = append(nodes, node)
	}

	<-time.After(time.Second * 1)

	tctx, tcancel := context.WithTimeout(ctx, time.Second*6)
	defer tcancel()
	for _, node := range nodes {
		for _, other := range nodes {
			if node.host.ID() == other.Host().ID() {
				continue
			}
			require.NoError(t, node.Host().Connect(tctx, *host.InfoFromHost(other.Host())))
		}
	}

	t.Log("all nodes are connected")

	<-time.After(time.Second * 2)

	for _, node := range nodes {
		go func(n *Node) {
			_ = n.Listen(ctx, "test")
		}(node)
	}

	t.Log("listening on topic: test")

	<-time.After(time.Second * 3)

	var wg sync.WaitGroup
	for i, node := range nodes {
		wg.Add(1)
		go func(n *Node, i int) {
			defer wg.Done()
			msg := spectypes.SSVMessage{
				MsgType: spectypes.SSVDecidedMsgType,
				MsgID:   spectypes.NewMsgID([]byte("1111"), spectypes.BNRoleAttester),
				Data:    []byte(fmt.Sprintf("some decided message %d", i)),
			}
			data, err := msg.Encode()
			require.NoError(t, err)
			require.NoError(t, n.TopicManager().Publish(ctx, "test", data))
		}(node, i)
	}

	wg.Wait()
	<-time.After(time.Second * 3)

	require.GreaterOrEqual(t, msgCount, int64(len(nodes)*(len(nodes)-2)))
	require.Equal(t, errCount, int64(0))
}
