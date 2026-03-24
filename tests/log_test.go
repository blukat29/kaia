package tests

import "github.com/kaiachain/kaia/log"

func init() {
	log.EnableLogForTest(log.LvlError, log.LvlWarn)
}
