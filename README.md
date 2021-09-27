# cache2go  v0.2

原项目地址 https://github.com/muesli/cache2go  
本项目为cache2go的学习与解读  

## 项目目录结构
* **example:**  cache2go的各种使用案例
* **cache.go:**  创建cache
* **cache_test.go:**  cache的各种单元测试
* **cachetable.go:**  table的初始化及增删改查
* **cacheitem.go:**  item的初始化及增删改查
* **errors.go**  错误申明

## 概述
缓存大概结构 `cache-table-item`,可以理解为数据库的 `数据库-表-表记录`  

官方描述:  
Concurrency-safe golang caching library with expiration capabilities.  
高并发安全,带有超时检测功能的缓存库  
**高并发安全:**  主要依靠读写锁实现  
**超时检测功能:**  table中有一个定时器与超时检测函数,定期做检查  

该缓存库为key-value形式的内存数据库,不支持数据持久化  
**优点:**
* 可以设置各种回调函数,在缓存数据的增删改查的时候,会触发对应的回调函数,比较灵活  
  
**缺点:**  
* 缓存数据的存储本质上是使用golang map,由于map的自身缺陷(标记删除),在数据较多,增删比较频繁的情况下会造成OOM
* 超时检测原理为遍历所有数据,查看是否到期及最小到期时间,性能很低,这里考虑构建跳表进行优化  

## 源码解读  
项目中只涉及到2个复杂的数据类型,分别是
* CacheItem  
* CacheTable
含义和字面意义一致，一个是缓存表，一个是缓存表中的条目  
一个Cache中以map[key(string)]*CacheTable的形式存储多个CacheTable
一个CacheTable中以map[key(interface{})]*CacheItem的形式存储多个CacheItem

CacheTable
```go
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
```

CacheItem
```go
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
```
该项目比较初级,源码较少,配有注释就不做过多解释了,仅供golang初学者学习交流