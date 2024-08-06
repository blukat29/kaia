package types

import (
	"github.com/kaiachain/kaia/kaiax"
)

type StakingModule interface {
	kaiax.BaseModule
	kaiax.UnwindableModule

	GetStakingInfo(num uint64) (*StakingInfo, error)
}
