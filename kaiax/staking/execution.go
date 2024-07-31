package staking

import "github.com/kaiachain/kaia/blockchain/types"

// PostInsertBlock is called after a block is inserted into the blockchain.
// Calculates the staking info for the next block, then caches and persists it.
func (s *StakingModule) PostInsertBlock(*types.Block) error {
	return nil
}
