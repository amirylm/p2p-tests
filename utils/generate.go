package utils

import (
	"fmt"
	"github.com/bloxapp/ssv-spec/types"
	"github.com/herumi/bls-eth-go-binary/bls"
	"math/rand"
	"sort"
	"time"
)

//var (
//	curveOrder = new(big.Int)
//)

func init() {
	_ = bls.Init(bls.BLS12_381)
	_ = bls.SetETHmode(bls.EthModeDraft07)
	//curveOrder, _ = curveOrder.SetString(bls.GetCurveOrder(), 10)
	rand.Seed(time.Now().UnixNano())
}

// Generate generates groups of operators and validators
func Generate(ops, vals, maxGroups int) Groups {
	operators := GenerateOperatorIDs(ops)
	g := GenerateGroups(vals, operators, maxGroups)
	return g
}

func GenerateOperatorIDs(n int) []types.OperatorID {
	oids := make([]types.OperatorID, n)
	for i := 0; i < n; i++ {
		oids[i] = types.OperatorID(i + 1)
	}
	return oids
}

type Share struct {
	PK        string
	Operators Group
}

type Group [4]types.OperatorID

func (g Group) ID() string {
	ints := make([]int, 4)
	for i, oid := range g {
		ints[i] = int(oid)
	}
	sort.Ints(ints)
	return fmt.Sprintf("%d_%d_%d_%d", ints[0], ints[1], ints[2], ints[3])
}

func (g *Group) Append(oid types.OperatorID) bool {
	if oid == types.OperatorID(0) {
		return false
	}
	for i, o := range g {
		if o == oid {
			return false
		}
		if o == types.OperatorID(0) {
			g[i] = oid
			return true
		}
	}
	return false
}

func RandGroup(operators int) Group {
	g := Group{}
	n := 4
	for i := 0; i < n; i++ {
		o := types.OperatorID(rand.Intn(operators) + 1)
		for !g.Append(o) {
			o = types.OperatorID(rand.Intn(operators) + 1)
		}
	}
	return g
}

type Groups map[string][]Share

func GenerateGroups(n int, operators []types.OperatorID, maxGroups int) Groups {
	groups := make(Groups)
	nOperators := len(operators)
	for i := 0; i < n; i++ {
		g := RandGroup(nOperators)
		gid := g.ID()
		shares, ok := groups[gid]
		if !ok {
			if maxGroups > 0 && len(groups) >= maxGroups {
				// avoid creating another group if we reached to max groups
				i--
				continue
			}
			shares = make([]Share, 0)
		}
		groups[gid] = append(shares, GenerateShare(g))
	}
	return groups
}

func GenerateShare(group Group) Share {
	sk := &bls.SecretKey{}
	sk.SetByCSPRNG()
	return Share{
		PK:        sk.GetPublicKey().SerializeToHexStr(),
		Operators: group,
	}
}
