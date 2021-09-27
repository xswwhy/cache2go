package cache2go

import (
	"sync"
	"time"
)

type CacheItem struct {
	key  interface{}
	data interface{}

	lifeSpan    time.Duration
	createdOn   time.Time
	accessedOn  time.Time
	accessCount int64

	// item被删除时触发的回调函数
	aboutToExpire []func(key interface{})
	sync.RWMutex
}

func NewCacheItem(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	t := time.Now()
	return &CacheItem{
		key:         key,
		lifeSpan:    lifeSpan,
		createdOn:   t,
		accessedOn:  t,
		accessCount: 0,
		data:        data,
	}
}

// 更新accessedOn,达到延长到期时间的目的
func (item *CacheItem) KeepAlive() {
	item.Lock()
	defer item.Unlock()
	item.accessedOn = time.Now()
	item.accessCount++
}

// 获取item的生命周期
func (item *CacheItem) LifeSpan() time.Duration {
	return item.lifeSpan
}

// 获取item的访问时间
func (item *CacheItem) AccessedOn() time.Time {
	item.RWMutex.RLock()
	defer item.RWMutex.RUnlock()
	return item.accessedOn
}

// 获取item的创建时间
func (item *CacheItem) CreatedOn() time.Time {
	item.RWMutex.RLock()
	defer item.RWMutex.RUnlock()
	return item.createdOn
}

// 获取item的访问次数
func (item *CacheItem) AccessCount() int64 {
	item.RLock()
	defer item.RUnlock()
	return item.accessCount
}

func (item *CacheItem) Key() interface{} {
	return item.key
}

func (item *CacheItem) Data() interface{} {
	return item.data
}

// aboutToExpire 的增删改
func (item *CacheItem) SetAboutToExpireCallback(f func(interface{})) {
	if len(item.aboutToExpire) > 0 {
		item.RemoveAboutToExpireCallback()
	}
	item.RWMutex.Lock()
	defer item.RWMutex.Unlock()
	item.aboutToExpire = append(item.aboutToExpire, f)
}

func (item *CacheItem) AddAboutToExpireCallback(f func(interface{})) {
	item.RWMutex.Lock()
	item.RWMutex.Unlock()
	item.aboutToExpire = append(item.aboutToExpire, f)
}

func (item *CacheItem) RemoveAboutToExpireCallback() {
	item.RWMutex.Lock()
	defer item.RWMutex.Unlock()
	item.aboutToExpire = nil
}
