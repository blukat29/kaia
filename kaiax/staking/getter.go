package staking

import (
	"errors"

	moduletypes "github.com/kaiachain/kaia/kaiax/staking/types"
)

func (s *StakingModule) GetStakingInfo() (*moduletypes.StakingInfo, error) {
	return nil, errors.New("not implemented")
}
