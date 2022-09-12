package main

import (
	"context"
	"fmt"
	"github.com/amir-blox/p2p-tests/network"
	spectypes "github.com/bloxapp/ssv-spec/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
	"math/rand"
	"sync"
	"time"
)

func runSubnets(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	var (
		valdatorsCount    = runenv.IntParam("validators")
		subnetsCount      = runenv.IntParam("subnets")
		maxPeers          = runenv.IntParam("max_peers")
		latency           = runenv.IntParam("t_latency")
		latencyMax        = runenv.IntParam("t_latency_max")
		validationLatency = runenv.IntParam("validation_latency")
	)

	runenv.RecordMessage("started test instance; params: validators=%d, subnets=%d, t_latency=%d, t_latency_max=%d, validation_latency=%d",
		valdatorsCount, subnetsCount, latency, latencyMax, validationLatency)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// wait until all instances in this test run have signalled
	initCtx.MustWaitAllInstancesInitialized(ctx)

	ip := initCtx.NetClient.MustGetDataNetworkIP()

	msgHandler := func(s string, message spectypes.SSVMessage) {
		runenv.R().Counter(fmt.Sprintf("in_msg_%s", s)).Inc(1)
	}
	errHandler := func(s string, err error) {
		runenv.R().Counter(fmt.Sprintf("in_msg_err_%s", s)).Inc(1)
	}
	name := fmt.Sprintf("testnode-%d", initCtx.GlobalSeq)
	node, err := network.NewNode(ctx, &network.NodeConfig{
		Name:              name,
		ListenAddrs:       []string{fmt.Sprintf("/ip4/%s/tcp/0", ip)},
		MaxPeers:          maxPeers,
		MsgHandler:        msgHandler,
		ErrHandler:        errHandler,
		ValidationLatency: time.Millisecond * time.Duration(validationLatency),
	})
	if err != nil {
		return fmt.Errorf("failed to instantiate node: %w", err)
	}
	defer func() {
		err := node.Close()
		runenv.RecordFailure(errors.Wrap(err, "could not close node"))
	}()

	runenv.RecordMessage("node is ready, listening on: %v", node.Host().Addrs())

	hostId := node.Host().ID()
	selfAI := host.InfoFromHost(node.Host())

	peers, err := getAllPeers(ctx, runenv, initCtx, *selfAI)
	if err != nil {
		return errors.Wrap(err, "could not get peers")
	}

	for _, ai := range peers {
		if ai.ID == hostId {
			continue
		}
		runenv.RecordMessage("dialing peer: %s", ai.ID)
		if err := node.Host().Connect(ctx, ai); err != nil {
			return err
		}
	}

	<-time.After(time.Second * 2)

	go func() {
		if err := node.Listen(ctx, "test"); err != nil {
			runenv.RecordFailure(errors.Wrap(err, "could not listen to test topic"))
		}
	}()

	<-time.After(time.Second * 10)

	rand.Seed(time.Now().UnixNano() + initCtx.GlobalSeq)

	var sendErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		id := node.Host().ID().String()
		for i := 0; i < 17; i++ {
			runenv.RecordMessage("⚡️  ITERATION ROUND %d", i)
			latency := time.Duration(rand.Int31n(int32(latencyMax))) * time.Millisecond
			runenv.RecordMessage("(round %d) my latency: %s", i, latency)

			msg := spectypes.SSVMessage{
				MsgType: spectypes.SSVDecidedMsgType,
				MsgID:   spectypes.NewMsgID([]byte("1111"), spectypes.BNRoleAttester),
				Data:    []byte(fmt.Sprintf("some decided message from %s i=%d", name, i)),
			}
			data, err := msg.Encode()
			if err != nil {
				sendErr = err
				continue
			}
			ts := time.Now()
			err = node.TopicManager().Publish(ctx, "test", data)
			point := fmt.Sprintf("publish-result,round=%d,peer=%s", i, id)
			if err != nil {
				runenv.RecordMessage("could not publish message from peer %s with error: %s", id, err.Error())
				runenv.R().RecordPoint(point, -1.0)
			} else {
				runenv.RecordMessage("publish message from peer %s", id)
				// record a result point; these points will be batch-inserted
				// into InfluxDB when the test concludes.
				runenv.R().RecordPoint(point, float64(time.Now().Sub(ts).Milliseconds())/1000.0)
			}
			<-time.After(1 * time.Second)
		}
		<-time.After(time.Second * 5)
	}()

	wg.Wait()

	if sendErr != nil {
		return sendErr
	}

	return nil
}

func getAllPeers(ctx context.Context, runenv *runtime.RunEnv, initCtx *run.InitContext, ai peer.AddrInfo) ([]peer.AddrInfo, error) {
	// the peers topic where all instances will advertise their AddrInfo.
	peersTopic := tsync.NewTopic("peers", new(peer.AddrInfo))
	// initialize a slice to store the AddrInfos of all other peers in the run.
	peers := make([]peer.AddrInfo, 0, runenv.TestInstanceCount)
	// Publish our own.
	initCtx.SyncClient.MustPublish(ctx, peersTopic, ai)

	peersCh := make(chan peer.AddrInfo)
	sctx, scancel := context.WithCancel(ctx)
	defer scancel()
	sub := initCtx.SyncClient.MustSubscribe(sctx, peersTopic, peersCh)

	// Receive the expected number of AddrInfos.
	for len(peers) < cap(peers) {
		select {
		case ai := <-peersCh:
			peers = append(peers, ai)
		case err := <-sub.Done():
			return peers, err
		}
	}

	return peers, nil
}
