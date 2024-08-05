package types

import (
	"math"
	"sort"

	"github.com/kaiachain/kaia/common"
)

var EmptyGini float64 = -1.0 // The gini coefficient of an empty set.

// Refined staking information suitable for proposer selection.
// Sometimes a node would register multiple NodeIds in AddressBook,
// in which each entry has different StakingAddr and same RewardAddr.
// We treat those entries with common RewardAddr as one node.
//
// For example,
//
//	NodeAddrs      = [N1, N2, N3]
//	StakingAddrs   = [S1, S2, S3]
//	RewardAddrs    = [R1, R1, R3]
//	StakingAmounts = [A1, A2, A3]
//
// can be consolidated into
//
//	CN1 = {[N1,N2], [S1,S2], R1, A1+A2}
//	CN3 = {[N3],    [S3],    R3, A3}
type consolidatedNode struct {
	NodeIds          []common.Address
	StakingContracts []common.Address
	RewardAddr       common.Address // The common RewardAddr
	StakingAmount    uint64         // Sum of the staking amounts
}

type StakingInfo struct {
	SourceBlockNum uint64 `json:"blockNum"` // The source block number where the staking info is measured.

	// The AddressBook triplets
	NodeIds          []common.Address `json:"councilNodeAddrs"`
	StakingContracts []common.Address `json:"councilStakingAddrs"`
	RewardAddrs      []common.Address `json:"councilRewardAddrs"`

	// Treasury fund addresses
	KEFAddr common.Address `json:"kefAddr"` // KEF contract address
	KIFAddr common.Address `json:"kifAddr"` // KIF contract address

	// Staking amounts
	StakingAmounts []uint64 `json:"councilStakingAmounts"` // Staking amounts of each staking contracts, in KAIA, rounded down.

	// Computed fields
	consolidatedNodes  *[]consolidatedNode
	cachedGini         *float64
	cachedGiniMinStake uint64 // The minimum staking amount used to compute Gini coefficient.
}

func (si *StakingInfo) ConsolidatedNodes() []consolidatedNode {
	if si.consolidatedNodes == nil {
		si.consolidatedNodes = si.consolidateNodes()
	}
	return *si.consolidatedNodes
}

func (si *StakingInfo) consolidateNodes() *[]consolidatedNode {
	// because Go map is not ordered, rList keeps track of the occurrence order of RewardAddrs.
	// to later arrange the consolidatedNodes.
	cmap := make(map[common.Address]*consolidatedNode)
	rList := make([]common.Address, 0, len(si.RewardAddrs))

	for i := range si.NodeIds {
		r := si.RewardAddrs[i]
		if cn, ok := cmap[r]; ok {
			cn.NodeIds = append(cn.NodeIds, si.NodeIds[i])
			cn.StakingContracts = append(cn.StakingContracts, si.StakingContracts[i])
			cn.StakingAmount += si.StakingAmounts[i]
		} else {
			cmap[r] = &consolidatedNode{
				NodeIds:          []common.Address{si.NodeIds[i]},
				StakingContracts: []common.Address{si.StakingContracts[i]},
				RewardAddr:       r,
				StakingAmount:    si.StakingAmounts[i],
			}
			rList = append(rList, r)
		}
	}

	carr := make([]consolidatedNode, 0, len(cmap))
	for _, r := range rList {
		carr = append(carr, *cmap[r])
	}
	return &carr
}

// Returns the Gini coefficient among the staking amounts that are greater than or equal to minStake.
// The amounts are first consolidated by RewardAddr, filtered by minStake, and then summarized to Gini.
func (si *StakingInfo) Gini(minStake uint64) float64 {
	// Cache hits only if the same minStake is used.
	if si.cachedGini == nil || si.cachedGiniMinStake != minStake {
		g := si.computeGini(minStake)
		si.cachedGini = &g
		si.cachedGiniMinStake = minStake
	}
	return *si.cachedGini
}

func (si *StakingInfo) computeGini(minStake uint64) float64 {
	cnodes := si.ConsolidatedNodes()
	amounts := make(sort.Float64Slice, 0, len(cnodes))

	for _, cnode := range cnodes {
		if cnode.StakingAmount >= minStake {
			amounts = append(amounts, float64(cnode.StakingAmount))
		}
	}

	return computeGini(amounts)
}

func computeGini(amounts sort.Float64Slice) float64 {
	if len(amounts) == 0 {
		return EmptyGini
	}

	// A nlog(n) Gini coefficient algorithm. Faster than naive O(n^2) algorithm.
	sort.Sort(amounts)

	sumOfAbsoluteDifferences := float64(0)
	subSum := float64(0)

	for i, x := range amounts {
		temp := x*float64(i) - subSum
		sumOfAbsoluteDifferences = sumOfAbsoluteDifferences + temp
		subSum = subSum + x
	}

	result := sumOfAbsoluteDifferences / subSum / float64(len(amounts))
	result = math.Round(result*100) / 100
	return result
}
