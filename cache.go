package cache2go

import "sync"

var (
	cache = make(map[string]*CacheTable)
	mutex sync.RWMutex
)

// 创建一个Cache
func Cache(table string) *CacheTable {
	mutex.RLock()
	t, ok := cache[table]
	mutex.RUnlock()

	if !ok {
		mutex.Lock()
		// 下面两行为什么要再确认一次呢?
		// 有个词叫Double check,是为了防止多个goroutine同时调用Cache()重复进行初始化
		t, ok = cache[table]
		if !ok {
			t = &CacheTable{
				name:  table,
				items: make(map[interface{}]*CacheItem),
			}
			cache[table] = t
		}
		mutex.Unlock()
	}
	return t
}
