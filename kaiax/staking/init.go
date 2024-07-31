package staking

import (
	"github.com/kaiachain/kaia/accounts/abi/bind/backends"
	moduletypes "github.com/kaiachain/kaia/kaiax/staking/types"
	"github.com/kaiachain/kaia/log"
	"github.com/kaiachain/kaia/params"
	"github.com/kaiachain/kaia/storage/database"
)

var (
	_ (moduletypes.StakingModule) = (*StakingModule)(nil)

	logger = log.NewModuleLogger(log.KaiaXStaking)
)

type gov interface {
	EffectiveParams(num uint64) (*params.GovParamSet, error)
}

type chain interface {
	backends.BlockChainForCaller
}

type InitOpts struct {
	ChainKv     database.Database
	ChainConfig *params.ChainConfig
	Chain       chain
	Gov         gov
}

type StakingModule struct {
	ChainKv     database.Database
	ChainConfig *params.ChainConfig
	Gov         gov
	Chain       chain
}

func NewStakingModule() *StakingModule {
	return &StakingModule{}
}

func (s *StakingModule) Init(opts *InitOpts) error {
	return nil
}

func (s *StakingModule) Start() error {
	logger.Info("StakingModule started")
	return nil
}

func (s *StakingModule) Stop() {
	logger.Info("StakingModule stopped")
}
