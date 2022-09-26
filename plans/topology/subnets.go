package main

import (
	"context"
	"encoding/hex"
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
		clustersCfg = ".clusters.yaml"
		subnetsCount = 128
		maxPeers = 25
		latency           = 5
		latencyMax        = 50
		validationLatency = 5
	)
	if runenv.IsParamSet("clusters") {
		clustersCfg = runenv.StringParam("clusters")
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

	cls, err := readClusters(clustersCfg)
	if err != nil {
		return errors.Wrap(err, "could not read clusters")
	}

	myOid := spectypes.OperatorID(initCtx.GlobalSeq)
	var myValidators []string
	valdatorsCount := 0
	for _, shares := range cls {
		sharesLoop:
		for _, s := range shares {
			valdatorsCount++
			for _, oid := range s.Operators {
				if myOid == oid {
					myValidators = append(myValidators, s.PK)
					runenv.R().Counter("my_validators_count").Inc(1)
					continue sharesLoop
				}
			}
		}
	}

	runenv.RecordMessage("started test instance; params: i=%d, myValidators=%d, validators=%d, groups=%d, max_peers=%d, t_latency=%d, t_latency_max=%d, validation_latency=%d",
		initCtx.GlobalSeq, len(myValidators), valdatorsCount, len(cls), maxPeers, latency, latencyMax, validationLatency)

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
			runenv.RecordFailure(errors.Wrapf(err, "could not connect to node %s", ai.ID.String()))
		}
	}

	// wait for all peers to signal that they're connected
	initCtx.SyncClient.MustSignalAndWait(ctx, "connected", runenv.TestInstanceCount)

	subnets := [128]int{}
	for _, valPk := range myValidators {
		sub := ValidatorSubnet(valPk, uint64(subnetsCount))
		subnets[sub]++
		go func() {
			if err := node.Listen(ctx, fmt.Sprintf("test.%d", sub)); err != nil {
				runenv.RecordFailure(errors.Wrap(err, "could not listen to test topic"))
			}
		}()
	}
	for i, s := range subnets {
		runenv.R().Counter(fmt.Sprintf("subnet_%d_validators", i)).Inc(int64(s))
	}
	//go func() {
	//	if err := node.Listen(ctx, "test.decided"); err != nil {
	//		runenv.RecordFailure(errors.Wrap(err, "could not listen to test topic"))
	//	}
	//}()

	// wait for all peers to signal that they're subscribed
	initCtx.SyncClient.MustSignalAndWait(ctx, "subscribed", runenv.TestInstanceCount)

	//<-time.After(time.Second * 2)

	rand.Seed(time.Now().UnixNano() + initCtx.GlobalSeq)

	var sendErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		id := node.Host().ID().String()
		for i := 0; i < 9; i++ {
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

			for _, valPkHex := range myValidators {
				valPk, err := hex.DecodeString(valPkHex)
				if err != nil {
					runenv.RecordFailure(errors.Wrap(err, "could not listen to test topic"))
					continue
				}
				msg := spectypes.SSVMessage{
					MsgType: spectypes.SSVDecidedMsgType,
					MsgID:   spectypes.NewMsgID(valPk, spectypes.BNRoleAttester),
					Data:    []byte(fmt.Sprintf("some decided message from %s i=%d,pk=%s", name, i, valPkHex)),
				}
				data, err := msg.Encode()
				if err != nil {
					sendErr = err
					continue
				}
				ts := time.Now()
				sub := ValidatorSubnet(valPkHex, uint64(subnetsCount))
				err = node.TopicManager().Publish(ctx, fmt.Sprintf("test.%d", sub), data)
				point := fmt.Sprintf("publish-result,subnet=%d,pk=%s,round=%d", sub, valPkHex, i)
				if err != nil {
					runenv.RecordMessage("could not publish message from peer %s with error: %s", id, err.Error())
					runenv.R().RecordPoint(point, -1.0)
				} else {
					runenv.RecordMessage("publish message on topic %s from peer %s", fmt.Sprintf("test.%d", sub), id)
					runenv.R().RecordPoint(point, float64(time.Now().Sub(ts).Milliseconds())/1000.0)
				}
			}
			doneState := tsync.State(fmt.Sprintf("done-%d", i))
			initCtx.SyncClient.MustSignalAndWait(ctx, doneState, runenv.TestInstanceCount)
		}
	}()

	wg.Wait()

	if sendErr != nil {
		return sendErr
	}

	<-time.After(time.Second * 2)


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
