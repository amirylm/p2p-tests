package main

import (
	"context"
	"fmt"
	spectypes "github.com/bloxapp/ssv-spec/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
	"math/rand"
	"sync"
	"time"
)

func runSubnets(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	var (
		valdatorsCount = 250
		subnetsCount = 128
		maxPeers = 25
		latency           = 5
		latencyMax        = 50
		validationLatency = 5
	)
	if runenv.IsParamSet("validators") {
		valdatorsCount = runenv.IntParam("validators")
	}
	if runenv.IsParamSet("subnets") {
		subnetsCount      = runenv.IntParam("subnets")
	}
	if runenv.IsParamSet("max_peers") {
		maxPeers          = runenv.IntParam("max_peers")
	}
	if runenv.IsParamSet("t_latency") {
		latency          = runenv.IntParam("max_peers")
	}
	if runenv.IsParamSet("t_latency_max") {
		latencyMax          = runenv.IntParam("t_latency_max")
	}
	if runenv.IsParamSet("validation_latency") {
		validationLatency          = runenv.IntParam("validation_latency")
	}

	runenv.RecordMessage("started test instance; params: validators=%d, subnets=%d, max_peers=%d, t_latency=%d, t_latency_max=%d, validation_latency=%d",
		valdatorsCount, subnetsCount, maxPeers, latency, latencyMax, validationLatency)

	ctx, cancel := context.WithTimeout(context.Background(), 32*12*time.Second)
	defer cancel()

	// wait until all instances in this test run have signalled
	initCtx.MustWaitAllInstancesInitialized(ctx)

	ip := initCtx.NetClient.MustGetDataNetworkIP()

	msgHandler := func(t string, message spectypes.SSVMessage) {
		runenv.R().Counter(fmt.Sprintf("in_msg_%s", t)).Inc(1)
	}
	errHandler := func(t string, err error) {
		runenv.R().Counter(fmt.Sprintf("in_msg_err_%s", t)).Inc(1)
	}
	name := fmt.Sprintf("testnode-%d", initCtx.GlobalSeq)
	node, err := NewNode(ctx, &NodeConfig{
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
		_ = node.Close()
		//runenv.RecordFailure(errors.Wrap(err, "could not close node"))
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

	// wait for all peers to signal that they're connected
	initCtx.SyncClient.MustSignalAndWait(ctx, "connected", runenv.TestInstanceCount)

	go func() {
		if err := node.Listen(ctx, "test"); err != nil {
			runenv.RecordFailure(errors.Wrap(err, "could not listen to test topic"))
		}
	}()

	// wait for all peers to signal that they're subscribed
	initCtx.SyncClient.MustSignalAndWait(ctx, "subscribed", runenv.TestInstanceCount)

	rand.Seed(time.Now().UnixNano() + initCtx.GlobalSeq)

	var sendErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		id := node.Host().ID().String()
		for i := 0; i < 13; i++ {
			runenv.RecordMessage("⚡️  ITERATION ROUND %d", i)
			latency := time.Duration(rand.Int31n(int32(latencyMax))) * time.Millisecond
			runenv.RecordMessage("(round %d) my latency: %s", i, latency)

			initCtx.NetClient.MustConfigureNetwork(ctx, &network.Config{
				Network:        "default",
				Enable:         true,
				Default:        network.LinkShape{Latency: latency},
				CallbackState:  tsync.State(fmt.Sprintf("network-configured-%d", i)),
				CallbackTarget: runenv.TestInstanceCount,
			})

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
			point := fmt.Sprintf("publish-result,peer=%s,round=%d", id, i)
			if err != nil {
				runenv.RecordMessage("could not publish message from peer %s with error: %s", id, err.Error())
				runenv.R().RecordPoint(point, -1.0)
			} else {
				runenv.RecordMessage("publish message from peer %s", id)
				runenv.R().RecordPoint(point, float64(time.Now().Sub(ts).Milliseconds())/1000.0)
			}
			doneState := tsync.State(fmt.Sprintf("done-%d", i))
			initCtx.SyncClient.MustSignalAndWait(ctx, doneState, runenv.TestInstanceCount)
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
