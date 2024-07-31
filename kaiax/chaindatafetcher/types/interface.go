package types

import (
	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/consensus"
	"github.com/kaiachain/kaia/kaiax"
	"github.com/kaiachain/kaia/node/cn/tracers"
)

type BlockChain interface {
	CurrentHeader() *types.Header
}

type InitOpts struct {
	// config   *ChainDataFetcherConfig
	Engine   consensus.Engine
	Chain    BlockChain
	DebugAPI *tracers.API
}

// go:generate mockgen ...
type ChainDataFetcherModule interface {
	kaiax.BaseModule
	kaiax.JsonRpcModule   // for chaindatafetcher_ namespace
	kaiax.ExecutionModule // for execution result exporting
	kaiax.TxProcessModule // for internal tx tracing
	Init(opts *InitOpts) error
}
