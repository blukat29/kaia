package types

import (
	"github.com/kaiachain/kaia/kaiax"
	"github.com/kaiachain/kaia/reward"
)

type StakingInfo = reward.StakingInfo // TODO-kaiax: Re-define optimized here.

type StakingModule interface {
	kaiax.BaseModule
	kaiax.ExecutionModule
	kaiax.UnwindableModule

	GetStakingInfo() (*StakingInfo, error)
}
