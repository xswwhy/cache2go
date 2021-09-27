package cache2go

import (
	"log"
	"sort"
	"sync"
	"time"
)

type CacheTable struct {
	sync.RWMutex

	// table 表名
	name string
	// 以map的方式存储所有的item
	items map[interface{}]*CacheItem

	// 定时器,配合cleanupInterval触发expirationCheck函数达到缓存到期清理的作用
	cleanupTimer *time.Timer
	// 触发下一次 到期检查(expirationCheck函数) 的时间间隔
	cleanupInterval time.Duration

	logger *log.Logger

	// 访问不存在的item时,触发的回调函数
	loadData func(key interface{}, args ...interface{}) *CacheItem
	// 添加item时,触发的回调函数
	addedItem []func(item *CacheItem)
	// 删除数据时,触发的回调函数
	aboutToDeleteItem []func(item *CacheItem)
}

// 查看table缓存了多少item
func (table *CacheTable) Count() int {
	table.RLock()
	defer table.RUnlock()
	return len(table.items)
}

// 为table中每一个item执行一次trans操作(这是个耗时操作,而且会长时间持有写锁,尽量避免使用)
func (table *CacheTable) Foreach(trans func(key interface{}, item *CacheItem)) {
	table.Lock()
	defer table.Unlock()
	for k, v := range table.items {
		trans(k, v)
	}
}

// 设置loadData
func (table *CacheTable) SetDataLoader(f func(interface{}, ...interface{}) *CacheItem) {
	table.RWMutex.Lock()
	defer table.RWMutex.Unlock()
	table.loadData = f
}

// addedItem的增删改
func (table *CacheTable) SetAddedItemCallback(f func(item *CacheItem)) {
	if len(table.addedItem) > 0 {
		table.RemoveAddedItemCallbacks()
	}
	table.RWMutex.Lock()
	defer table.RWMutex.Unlock()
	table.addedItem = append(table.addedItem, f)
}

func (table *CacheTable) AddAddedItemCallback(f func(item *CacheItem)) {
	table.RWMutex.Lock()
	defer table.RWMutex.Unlock()
	table.addedItem = append(table.addedItem, f)
}

func (table *CacheTable) RemoveAddedItemCallbacks() {
	table.RWMutex.Lock()
	defer table.RWMutex.Unlock()
	table.addedItem = nil
}

// aboutToDeleteItem的增删改
func (table *CacheTable) SetAboutToDeleteItemCallback(f func(*CacheItem)) {
	if len(table.aboutToDeleteItem) > 0 {
		table.RemoveAboutToDeleteItemCallback()
	}
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

func (table *CacheTable) AddAboutToDeleteItemCallback(f func(*CacheItem)) {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = append(table.aboutToDeleteItem, f)
}

func (table *CacheTable) RemoveAboutToDeleteItemCallback() {
	table.Lock()
	defer table.Unlock()
	table.aboutToDeleteItem = nil
}

// 设置log的处理方式
func (table *CacheTable) SetLogger(logger *log.Logger) {
	table.RWMutex.Lock()
	defer table.RWMutex.Unlock()
	table.logger = logger
}

// 由定时器触发的到期时间检查,外界不能直接调用
// 遍历所有item,检查到期时间,删除到期的item
// 更新 cleanupInterval
func (table *CacheTable) expirationCheck() {
	table.RWMutex.Lock()
	if table.cleanupTimer != nil {
		table.cleanupTimer.Stop()
	}
	if table.cleanupInterval > 0 {
		table.log("Expiration check triggered after", table.cleanupInterval, "for table", table.name)
	} else {
		table.log("Expiration check installed for table", table.name)
	}

	now := time.Now()
	smallestDuration := 0 * time.Second // 记录所有未到期的item中 最快要到期的时间间隔
	for key, item := range table.items {
		item.RWMutex.RLock()
		lifeSpan := item.lifeSpan
		accessedOn := item.accessedOn
		item.RWMutex.RUnlock() // 读完数据后及时释放读锁

		if lifeSpan == 0 { // lifeSpan为0的没有过期时间,不参与过期检查
			continue
		}
		if now.Sub(accessedOn) > lifeSpan { // 过期了的item
			table.deleteInternal(key)
		} else {
			if smallestDuration == 0 || lifeSpan-now.Sub(accessedOn) < smallestDuration {
				smallestDuration = lifeSpan - now.Sub(accessedOn)
			}
		}
	}

	// 设置下次触发 到期检查 的时间及回调函数(expirationCheck函数)
	table.cleanupInterval = smallestDuration
	if smallestDuration > 0 {
		table.cleanupTimer = time.AfterFunc(smallestDuration, func() {
			go table.expirationCheck()
		})
	}
	table.RWMutex.Unlock()
}

// 供内部使用 table中添加item
func (table *CacheTable) addInternal(item *CacheItem) {
	table.log("Adding item with key", item.key, "and lifespan of", item.lifeSpan, "to table", table.name)
	table.items[item.key] = item

	// 先把要访问的数据拿出来,尽快释放写锁
	expDur := table.cleanupInterval
	addedItem := table.addedItem
	table.RWMutex.Unlock()

	// 调用 table.addedItem中的回调
	if addedItem != nil {
		for _, callback := range addedItem {
			callback(item)
		}
	}

	// 检查新加的item是否会触发 到期检查
	if (item.lifeSpan > 0) && (expDur == 0 || item.lifeSpan < expDur) {
		table.expirationCheck()
	}
}

// 供外界使用 table中添加item
func (table *CacheTable) Add(key interface{}, lifeSpan time.Duration, data interface{}) *CacheItem {
	item := NewCacheItem(key, lifeSpan, data)
	table.RWMutex.Lock()
	table.addInternal(item)
	return item
}

// 供内部使用 table中删除item
func (table *CacheTable) deleteInternal(key interface{}) (*CacheItem, error) {
	r, ok := table.items[key]
	if !ok {
		return nil, ErrKeyNotFound
	}
	aboutToDeletItem := table.aboutToDeleteItem
	table.RWMutex.Unlock()
	// 触发table中删除item的回调
	if aboutToDeletItem != nil {
		for _, callback := range aboutToDeletItem {
			callback(r)
		}
	}
	r.RWMutex.RLock()
	// 触发item被删除的回调
	if r.aboutToExpire != nil {
		for _, callback := range r.aboutToExpire {
			callback(key)
		}
	}
	r.RWMutex.RUnlock()

	table.RWMutex.Lock() // deleteInternal函数外table.RWMutex先lock在unlock ,函数里面先unlock在lock,主要是为了减少持有锁的时间
	table.log("Deleting item with key", key, "created on", r.createdOn, "and hit", r.accessCount, "times from table", table.name)
	delete(table.items, key)
	return r, nil
}

// 供外界使用 table中删除item
func (table *CacheTable) Delete(key interface{}) (*CacheItem, error) {
	table.Lock()
	defer table.Unlock()
	return table.deleteInternal(key)
}

// 判断该item是否在table中
func (table *CacheTable) Exists(key interface{}) bool {
	table.RWMutex.RLock()
	defer table.RWMutex.RUnlock()
	_, ok := table.items[key]
	return ok
}

// 缓存item了返回false  没有缓存就缓存一下返回true
func (table *CacheTable) NotFoundAdd(key interface{}, lifeSpan time.Duration, data interface{}) bool {
	table.RWMutex.Lock()
	if _, ok := table.items[key]; ok {
		table.RWMutex.Unlock()
		return false
	}
	item := NewCacheItem(key, lifeSpan, data)
	table.addInternal(item)
	return true
}

// 查询缓存key
func (table *CacheTable) Value(key interface{}, args ...interface{}) (*CacheItem, error) {
	table.RWMutex.RLock()
	r, ok := table.items[key]
	loadData := table.loadData
	table.RWMutex.RUnlock()
	if ok {
		// 更新时间,返回查询结果
		r.KeepAlive()
		return r, nil
	}

	// 没有找到的情况
	if loadData != nil {
		item := loadData(key, args...)
		if item != nil {
			table.Add(item.key, item.lifeSpan, item.data)
			return item, nil
		}
		return nil, ErrKeyNotFoundOrLoadable
	}
	return nil, ErrKeyNotFound
}

// 清除所有item
func (table *CacheTable) Flush() {
	table.RWMutex.Lock()
	defer table.RWMutex.Unlock()

	table.log("Flushing table", table.name)
	table.items = make(map[interface{}]*CacheItem)
	table.cleanupInterval = 0
	if table.cleanupTimer != nil {
		table.cleanupTimer.Stop()
	}
}

// 为了排序而定义的结构
type CacheItemPair struct {
	Key         interface{}
	AccessCount int64
}
type CacheItemList []CacheItemPair

// 这三个函数为了实现sort.Sort()中参数的接口
func (p CacheItemList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p CacheItemList) Len() int      { return len(p) }
// 这个控制排序大到小还是小到大    p[i].AccessCount > p[j].AccessCount 从大到小排   p[i].AccessCount < p[j].AccessCount 从小到大排
func (p CacheItemList) Less(i, j int) bool { return p[i].AccessCount > p[j].AccessCount }

// 从大到小取 count 个
func (table *CacheTable) MostAccessed(count int64) []*CacheItem {
	table.RWMutex.RLock()
	defer table.RWMutex.RUnlock()
	p := make(CacheItemList, len(table.items))
	i := 0
	for k, v := range table.items {
		p[i] = CacheItemPair{Key: k, AccessCount: v.accessCount}
		i++
	}
	sort.Sort(p)
	var r []*CacheItem
	c := int64(0)
	for _, v := range p {
		if c >= count {
			break
		}
		if item, ok := table.items[v.Key]; ok {
			r = append(r, item)
		}
		c++
	}
	return r
}

func (table *CacheTable) log(v ...interface{}) {
	if table.logger == nil {
		return
	}
	table.logger.Println(v...)
}
