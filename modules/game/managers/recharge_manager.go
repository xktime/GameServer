package managers

import (
	"sync"
)

type RechargeManager struct {
}

var (
	rechargeManager *RechargeManager
	rechargeOnce    sync.Once
)

func GetRechargeManager() *RechargeManager {
	rechargeOnce.Do(func() {
		rechargeManager = &RechargeManager{}
	})
	return rechargeManager
}
