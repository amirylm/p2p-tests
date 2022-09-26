package main

import (
	"context"
	"encoding/hex"
	"fmt"
	spectypes "github.com/bloxapp/ssv-spec/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/pkg/errors"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	tsync "github.com/testground/sdk-go/sync"
	"math/rand"
	"sort"
	"sync"
	"time"
)

func runGroups(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	var (
		clustersCfg = ".clusters.yaml"
		validatorsCount = 0
		maxPeers = 25
		latency           = 5
		latencyMax        = 50
		validationLatency = 5
	)
	if runenv.IsParamSet("clusters") {
		clustersCfg = runenv.StringParam("clusters")
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
	validatorGroups := make(map[string]string)
	myGroups := make(map[string]int)
	for _, shares := range cls {
	sharesLoop:
		for _, s := range shares {
			validatorsCount++
			for _, oid := range s.Operators {
				if myOid == oid {
					gid := groupID(s.Operators)
					myGroups[gid]++
					runenv.R().Counter(fmt.Sprintf("group_%s", gid)).Inc(1)
					myValidators = append(myValidators, s.PK)
					validatorGroups[s.PK] = gid
					runenv.R().Counter("my_validators_count").Inc(1)
					continue sharesLoop
				}
			}
		}
	}

	runenv.RecordMessage("started test instance; params: i=%d, myValidators=%d, myGroups=%d, validators=%d, groups=%d, max_peers=%d, t_latency=%d, t_latency_max=%d, validation_latency=%d",
		initCtx.GlobalSeq, len(myValidators), len(myGroups), validatorsCount, len(cls), maxPeers, latency, latencyMax, validationLatency)

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

	for g, _ := range myGroups {
		go func(gid string) {
			if err := node.Listen(ctx, fmt.Sprintf("test.%s", gid)); err != nil {
				runenv.RecordFailure(errors.Wrap(err, "could not listen to test topic"))
			}
		}(g)
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
		for i := 0; i < 1; i++ {
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
					runenv.RecordFailure(errors.Wrap(err, "could not parse message"))
					continue
				}
				ts := time.Now()
				gid := validatorGroups[valPkHex]
				topicName := fmt.Sprintf("test.%s", gid)
				err = node.TopicManager().Publish(ctx, topicName, data)
				point := fmt.Sprintf("publish-result,group=%s,pk=%s,round=%d", gid, valPkHex, i)
				if err != nil {
					runenv.RecordFailure(errors.Wrapf(err, "could not publish message on topic %s from peer %s", topicName, id))
					runenv.R().RecordPoint(point, -1.0)
				} else {
					runenv.RecordMessage("publish message on topic %s from peer %s", topicName, id)
					runenv.R().RecordPoint(point, float64(time.Now().Sub(ts).Milliseconds())/1000.0)
				}
			}
			doneState := tsync.State(fmt.Sprintf("done-%d", i))
			initCtx.SyncClient.MustSignalAndWait(ctx, doneState, runenv.TestInstanceCount)
			<-time.After(time.Second * 1)
		}
	}()

	wg.Wait()


	if sendErr != nil {
		return sendErr
	}

	<-time.After(time.Second * 2)

	return nil
}

func groupID(g group) string {
	ints := make([]int, 4)
	for i, oid := range g {
		ints[i] = int(oid)
	}
	sort.Ints(ints)
	return fmt.Sprintf("%d_%d_%d_%d", ints[0], ints[1], ints[2], ints[3])
}
