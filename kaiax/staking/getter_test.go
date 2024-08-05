package staking

import (
	"math/big"
	"testing"

	"github.com/kaiachain/kaia/accounts/abi/bind/backends"
	"github.com/kaiachain/kaia/blockchain"
	"github.com/kaiachain/kaia/blockchain/system"
	"github.com/kaiachain/kaia/common"
	"github.com/kaiachain/kaia/log"
	"github.com/kaiachain/kaia/params"
	"github.com/kaiachain/kaia/storage/database"
	"github.com/stretchr/testify/assert"
)

func TestGetStakingInfo_PostKaia(t *testing.T) {
	log.EnableLogForTest(log.LvlCrit, log.LvlWarn)

	// Create a simulated backend
	config := params.TestChainConfig.Copy()
	config.IstanbulCompatibleBlock = common.Big0
	config.LondonCompatibleBlock = common.Big0
	config.EthTxTypeCompatibleBlock = common.Big0
	config.MagmaCompatibleBlock = common.Big0
	config.KoreCompatibleBlock = common.Big0
	config.ShanghaiCompatibleBlock = common.Big0
	config.CancunCompatibleBlock = common.Big0
	config.KaiaCompatibleBlock = common.Big0
	config.SetDefaults()

	var (
		db    = database.NewMemoryDBManager()
		alloc = blockchain.GenesisAlloc{
			system.AddressBookAddr: {
				Code:    system.AddressBookMockTwoCNCode,
				Balance: big.NewInt(0),
			},
		}
		backend = backends.NewSimulatedBackendWithDatabase(db, alloc, config)
	)

	// Force using the MultiCallMock
	originCode := system.MultiCallCode
	system.MultiCallCode = system.MultiCallMockCode
	defer func() {
		system.MultiCallCode = originCode
		backend.Close()
	}()

	// Test GetStakingInfo()
	mStaking := NewStakingModule()
	mStaking.Init(&InitOpts{
		ChainKv:     db.GetMiscDB(),
		ChainConfig: config,
		Chain:       backend.BlockChain(),
	})

	// Addresses taken from AddressBookMock.sol:AddressBookMockTwoCN
	// Staking amounts taken from MultiCallContractMock.sol
	expected := &StakingInfo{
		SourceBlockNum: 0,
		NodeIds: []common.Address{
			common.HexToAddress("0x0000000000000000000000000000000000000F00"),
			common.HexToAddress("0x0000000000000000000000000000000000000F03")},
		StakingContracts: []common.Address{
			common.HexToAddress("0x0000000000000000000000000000000000000F01"),
			common.HexToAddress("0x0000000000000000000000000000000000000f04")},
		RewardAddrs: []common.Address{
			common.HexToAddress("0x0000000000000000000000000000000000000f02"),
			common.HexToAddress("0x0000000000000000000000000000000000000f05")},
		KIFAddr:        common.HexToAddress("0x0000000000000000000000000000000000000F06"),
		KEFAddr:        common.HexToAddress("0x0000000000000000000000000000000000000f07"),
		StakingAmounts: []uint64{5_000_000, 20_000_000},
	}
	si, err := mStaking.GetStakingInfo(0)
	assert.Nil(t, err)
	assert.Equal(t, expected, si)
}
