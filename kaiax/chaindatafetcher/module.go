package chaindatafetcher

import (
	"github.com/kaiachain/kaia/blockchain/types"
	"github.com/kaiachain/kaia/blockchain/vm"
	chaindatafetchertypes "github.com/kaiachain/kaia/kaiax/chaindatafetcher/types"
	"github.com/kaiachain/kaia/log"
	"github.com/kaiachain/kaia/networks/rpc"
)

var (
	_ chaindatafetchertypes.ChainDataFetcherModule = (*ChainDataFetcher)(nil)

	logger = log.NewModuleLogger(log.ChainDataFetcher)
)

type ChainDataFetcher struct {
}

func NewChainDataFetcher() *ChainDataFetcher {
	return &ChainDataFetcher{}
}

func (c *ChainDataFetcher) Init(opts *chaindatafetchertypes.InitOpts) error {
	return nil
}

func (c *ChainDataFetcher) Initialized() bool {
	return true
}

func (c *ChainDataFetcher) Start() error {
	logger.Info("ChainDataFetcher started")
	return nil
}

func (c *ChainDataFetcher) Stop() error {
	logger.Info("ChainDataFetcher stopped")
	return nil
}

func (c *ChainDataFetcher) API() []rpc.API {
	return nil
}

func (c *ChainDataFetcher) PostInsertBlock(*types.Block) error {
	return nil
}

// PreRunTx
func (c *ChainDataFetcher) PreRunTx(*vm.EVM, *types.Transaction) (*types.Transaction, error) {
	return nil, nil
}

// PostRunTx
func (c *ChainDataFetcher) PostRunTx(*vm.EVM, *types.Transaction) error {
	return nil
}
