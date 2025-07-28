package managers

import (
	"gameserver/common/models"

	"sync"

	"github.com/google/uuid"
)

type UserManager struct {
}

var (
	userManager     *UserManager
	userManagerOnce sync.Once
)

func GetUserManager() *UserManager {
	userManagerOnce.Do(func() {
		userManager = &UserManager{}
	})
	return userManager
}

// 缓存实现需要开发者自行覆盖
var memCache = make(map[string]models.User)

// todollw 设置缓存
func (m *UserManager) SetUserCache() {
	// cacheData, exist := memCache[token]
	// if !exist {
	// 	newToken := m.GenToken(context)
	// } else {
	// 	memCache[token] = cacheData
	// }
}

func (m *UserManager) GetCache(accountId string) (models.User, bool) {
	data, exist := memCache[accountId]
	return data, exist
}

func (m *UserManager) GenToken() string {
	return uuid.New().String()
}
