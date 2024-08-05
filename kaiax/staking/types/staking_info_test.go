package types

import (
	"testing"

	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/params"
	"gotest.tools/assert"
)

type stakingInfoTC struct {
	stakingInfo          *StakingInfo
	expectedConsolidated []consolidatedNode
	expectedGini         float64
}

var stakingInfoTCs = generateStakingInfoTestCases()

func generateStakingInfoTestCases() []stakingInfoTC {
	var (
		n1 = common.HexToAddress("0x8aD8F547fa00f58A8c4fb3B671Ee5f1A75bA028a")
		n2 = common.HexToAddress("0xB2AAda7943919e82143324296987f6091F3FDC9e")
		n3 = common.HexToAddress("0xD95c70710f07A3DaF7ae11cFBa10c789da3564D0")
		n4 = common.HexToAddress("0xC704765db1d21C2Ea6F7359dcB8FD5233DeD16b5")

		s1 = common.HexToAddress("0x4dd324F9821485caE941640B32c3Bcf1fA6E93E6")
		s2 = common.HexToAddress("0x0d5Df5086B5f86f748dFaed5779c3f862C075B1f")
		s3 = common.HexToAddress("0xD3Ff05f00491571E86A3cc8b0c320aA76D7413A5")
		s4 = common.HexToAddress("0x11EF8e61d10365c2ECAe0E95b5fFa9ed4D68d64f")

		r1 = common.HexToAddress("0x241c793A9AD555f52f6C3a83afe6178408796ab2")
		r2 = common.HexToAddress("0x79b427Fb77077A9716E08D049B0e8f36Abfc8E2E")
		r3 = common.HexToAddress("0x62E47d858bf8513fc401886B94E33e7DCec2Bfb7")
		r4 = common.HexToAddress("0xf275f9f4c0d375F9E3E50370f93b504A1e45dB09")

		kef = common.HexToAddress("0x136807B12327a8AfF9831F09617dA1B9D398cda2")
		kif = common.HexToAddress("0x46bA8F7538CD0749e572b2631F9FB4Ce3653AFB8")

		a0 uint64 = 0
		aL uint64 = 1000000  // less than minstaking
		aM uint64 = 2000000  // exactly minstaking (params.DefaultMinimumStake)
		a1 uint64 = 10000000 // above minstaking. Using 1,2,4,8 to uniquely spot errors
		a2 uint64 = 20000000
		a3 uint64 = 40000000
		a4 uint64 = 80000000
	)
	if aM != params.DefaultMinimumStake.Uint64() {
		panic("broken test assumption")
	}

	return []stakingInfoTC{
		{ // Empty
			stakingInfo:          &StakingInfo{},
			expectedConsolidated: []consolidatedNode{},
			expectedGini:         EmptyGini,
		},
		{ // 1 entry
			stakingInfo: &StakingInfo{
				SourceBlockNum:   86400,
				NodeIds:          []common.Address{n1},
				StakingContracts: []common.Address{s1},
				RewardAddrs:      []common.Address{r1},
				KEFAddr:          kef,
				KIFAddr:          kif,
				StakingAmounts:   []uint64{a1},
			},
			expectedConsolidated: []consolidatedNode{
				{[]common.Address{n1}, []common.Address{s1}, r1, a1},
			},
			expectedGini: 0.0,
		},
		{ // Unrelated 4 nodes
			stakingInfo: &StakingInfo{
				SourceBlockNum:   2 * 86400,
				NodeIds:          []common.Address{n1, n2, n3, n4},
				StakingContracts: []common.Address{s1, s2, s3, s4},
				RewardAddrs:      []common.Address{r1, r2, r3, r4},
				KEFAddr:          kef,
				KIFAddr:          kif,
				StakingAmounts:   []uint64{a1, a2, a3, a4},
			},
			expectedConsolidated: []consolidatedNode{
				{[]common.Address{n1}, []common.Address{s1}, r1, a1},
				{[]common.Address{n2}, []common.Address{s2}, r2, a2},
				{[]common.Address{n3}, []common.Address{s3}, r3, a3},
				{[]common.Address{n4}, []common.Address{s4}, r4, a4},
			},
			expectedGini: 0.38,
		},
		{ // 4 nodes consolidated to 2 nodes
			stakingInfo: &StakingInfo{
				SourceBlockNum:   3 * 86400,
				NodeIds:          []common.Address{n1, n2, n3, n4},
				StakingContracts: []common.Address{s1, s2, s3, s4},
				RewardAddrs:      []common.Address{r1, r2, r1, r2}, // r1 and r2 used twice each
				KEFAddr:          kef,
				KIFAddr:          kif,
				StakingAmounts:   []uint64{a1, a2, a3, a4},
			},
			expectedConsolidated: []consolidatedNode{
				{[]common.Address{n1, n3}, []common.Address{s1, s3}, r1, a1 + a3},
				{[]common.Address{n2, n4}, []common.Address{s2, s4}, r2, a2 + a4},
			},
			expectedGini: 0.17,
		},
		{ // 4 nodes with some below minstaking
			stakingInfo: &StakingInfo{
				SourceBlockNum:   4 * 86400,
				NodeIds:          []common.Address{n1, n2, n3, n4},
				StakingContracts: []common.Address{s1, s2, s3, s4},
				RewardAddrs:      []common.Address{r1, r2, r3, r4},
				KEFAddr:          kef,
				KIFAddr:          kif,
				StakingAmounts:   []uint64{a2, aM, aL, a0}, // aL and a0 should be ignored in Gini calculation
			},
			expectedConsolidated: []consolidatedNode{
				{[]common.Address{n1}, []common.Address{s1}, r1, a2},
				{[]common.Address{n2}, []common.Address{s2}, r2, aM},
				{[]common.Address{n3}, []common.Address{s3}, r3, aL},
				{[]common.Address{n4}, []common.Address{s4}, r4, a0},
			},
			expectedGini: 0.41,
		},
	}
}

func TestStakingInfo(t *testing.T) {
	for _, tc := range stakingInfoTCs {
		assert.DeepEqual(t, tc.stakingInfo.ConsolidatedNodes(), tc.expectedConsolidated)
		assert.Equal(t, tc.stakingInfo.Gini(params.DefaultMinimumStake.Uint64()), tc.expectedGini)
	}
}

func TestComputeGini(t *testing.T) {
	assert.Equal(t, EmptyGini, computeGini([]float64{}))
	assert.Equal(t, 0.0, computeGini([]float64{1, 1, 1}))
	assert.Equal(t, 0.8, computeGini([]float64{0, 8, 0, 0, 0}))
	assert.Equal(t, 0.27, computeGini([]float64{5, 4, 1, 2, 3}))
}
