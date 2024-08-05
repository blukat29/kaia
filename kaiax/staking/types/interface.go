package types

import (
	"github.com/kaiachain/kaia/kaiax"
)

type StakingModule interface {
	kaiax.BaseModule
	kaiax.ExecutionModule
	kaiax.UnwindableModule

	GetStakingInfo(num uint64) (*StakingInfo, error)
}
