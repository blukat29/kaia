package staking

import (
	"fmt"
	"math/big"

	"github.com/kaiachain/kaia/accounts/abi/bind"
	"github.com/kaiachain/kaia/blockchain/state"
	"github.com/kaiachain/kaia/blockchain/system"
	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/common"
	staking_types "github.com/kaiachain/kaia/kaiax/staking/types"
	"github.com/kaiachain/kaia/params"
)

const ( // Numeric Type IDs used by AddressBook.getAllAddress(), and in turn by MultiCall contract.
	CN_NODE_ID_TYPE         = 0
	CN_STAKING_ADDRESS_TYPE = 1
	CN_REWARD_ADDRESS_TYPE  = 2
	POC_CONTRACT_TYPE       = 3 // The AddressBook's pocContractAddress field repurposed as KGF, KFF, KIF
	KIR_CONTRACT_TYPE       = 4 // The AddressBook's kirContractAddress field repurposed as KIR, KCF, KEF
)

// Return the validator staking status used for processing the given block number.
func (s *StakingModule) GetStakingInfo(num uint64) (*StakingInfo, error) {
	if s.isKaia(num) {
		return s.getPostKaiaCached(num)
	} else {
		return s.getPreKaiaCached(num)
	}
}

func (s *StakingModule) getPostKaiaCached(num uint64) (*StakingInfo, error) {
	sourceNum := staking_types.SourceBlockNum(num, s.stakingInterval, true)

	// TODO: memory cache
	si, err := s.getUncached(sourceNum)
	if err != nil {
		return nil, err
	}

	return si, nil
}

func (s *StakingModule) getPreKaiaCached(num uint64) (*StakingInfo, error) {
	sourceNum := staking_types.SourceBlockNum(num, s.stakingInterval, false)

	// TODO: memory and db caches
	si, err := s.getUncached(sourceNum)
	if err != nil {
		return nil, err
	}

	return si, nil
}

// Read the staking status from the blockchain state.
func (s *StakingModule) getUncached(num uint64) (*StakingInfo, error) {
	header := s.Chain.GetHeaderByNumber(num)
	if header == nil {
		return nil, fmt.Errorf("failed to get header for block number %d", num)
	}
	statedb, err := s.Chain.StateAt(header.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to get state for block number %d: %v", num, err)
	}

	return s.getFromStateMultiCall(header, statedb)
}

// A more efficient way using the MultiCall contract that fetches addresses and balances in one EVM call.
// One caveat is that MultiCall contract only works after Cancun fork which has the PUSH0 opcode.
// TODO: compile MultiCall without PUSH0 and use it from genesis block.
func (s *StakingModule) getFromStateMultiCall(header *types.Header, statedb *state.StateDB) (*StakingInfo, error) {
	num := header.Number.Uint64()

	// Bail out if AddressBook is not installed.
	// This is a common case for private nets.
	if statedb.GetCode(system.AddressBookAddr) == nil {
		logger.Trace("returning empty staking info because AddressBook is not installed", "sourceNum", num)
		return &StakingInfo{SourceBlockNum: num}, nil
	}

	// Now we're safe to call the  MultiCall contract.
	contract, err := system.NewMultiCallContractCaller(statedb, s.Chain, header)
	if err != nil {
		return nil, errCannotCallABook(err)
	}
	res, err := contract.MultiCallStakingInfo(&bind.CallOpts{BlockNumber: header.Number})
	if err != nil {
		return nil, errCannotCallABook(err)
	}

	return parseCallResult(num, res.TypeList, res.AddressList, res.StakingAmounts)
}

func parseCallResult(num uint64, types []uint8, addrs []common.Address, amounts []*big.Int) (*StakingInfo, error) {
	// Sanity check.
	if len(types) == 0 && len(addrs) == 0 {
		// This is an expected behavior when the AddressBook contract is not activated yet.
		logger.Trace("returning empty staking info because AddressBook is not activated", "sourceNum", num)
		return &StakingInfo{SourceBlockNum: num}, nil
	}
	if len(types) != len(addrs) {
		logger.Error("length of type list and address list differ", "sourceNum", num, "typeLen", len(types), "addrLen", len(addrs))
		return nil, errInvalidABook
	}

	// Collect the results to StakingInfo fields.
	var (
		nodeIds          []common.Address
		stakingContracts []common.Address
		rewardAddrs      []common.Address
		kefAddr          common.Address
		kifAddr          common.Address

		stakingAmounts = make([]uint64, len(amounts))
	)
	for i, ty := range types {
		switch ty {
		case CN_NODE_ID_TYPE:
			nodeIds = append(nodeIds, addrs[i])
		case CN_STAKING_ADDRESS_TYPE:
			stakingContracts = append(stakingContracts, addrs[i])
		case CN_REWARD_ADDRESS_TYPE:
			rewardAddrs = append(rewardAddrs, addrs[i])
		// Caution: not to confuse (POC, KIR) order
		case POC_CONTRACT_TYPE:
			kifAddr = addrs[i]
		case KIR_CONTRACT_TYPE:
			kefAddr = addrs[i]
		default:
			logger.Error("unknown type", "sourceNum", num, "type", ty)
			return nil, errInvalidABook
		}
	}
	for i, a := range amounts {
		stakingAmounts[i] = big.NewInt(0).Div(a, big.NewInt(params.KAIA)).Uint64()
	}

	// Sanity check
	if len(nodeIds) != len(stakingContracts) || len(nodeIds) != len(rewardAddrs) || len(nodeIds) != len(amounts) ||
		common.EmptyAddress(kefAddr) || common.EmptyAddress(kifAddr) {
		// This is an expected behavior when the AddressBook contract is not activated yet.
		logger.Trace("returning empty staking info because AddressBook is not activated", "sourceNum", num)
		return &StakingInfo{SourceBlockNum: num}, nil
	}

	return &StakingInfo{
		SourceBlockNum:   num,
		NodeIds:          nodeIds,
		StakingContracts: stakingContracts,
		RewardAddrs:      rewardAddrs,
		KEFAddr:          kefAddr,
		KIFAddr:          kifAddr,
		StakingAmounts:   stakingAmounts,
	}, nil
}
