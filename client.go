package bttsetting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

var (
	ErrNotFound = errors.New("config not found")
)

// Config 是主要入口点。
type Config struct {
	rdb      *redis.Client
	version  int
	snapshot atomic.Value // 存储 *Snapshot
	mu       sync.RWMutex // 用于更新操作
	// 全局 ValueCache (L2) 减少反序列化开销
	// Key: ValueHash + string(reflect.Type), Value: any
	valueCache sync.Map
}

// New 创建一个新的 Config 实例。
// client: Redis 客户端实例（外部传入，DI）。
// version: 配置版本号，用于版本控制。
func New(client *redis.Client, version int) (*Config, error) {
	c := &Config{
		rdb:     client,
		version: version,
	}

	// 初始化空快照
	c.snapshot.Store(&Snapshot{
		Version: version,
		AllHash: "",
		Rules:   make(map[string][]Rule),
		Values:  make(map[string]string),
	})

	// 立即加载
	if err := c.Load(context.Background()); err != nil {
		return nil, err
	}

	return c, nil
}

// WithTags 创建一个感知上下文的 Getter。
func (c *Config) WithTags(tags map[string]any) *Getter {
	return &Getter{
		cfg:   c,
		tags:  tags,
		cache: make(map[string]CacheEntry),
	}
}

// UpdateTags 更新 Getter 的 Tags 并清空本地缓存。
func (g *Getter) UpdateTags(tags map[string]any) {
	g.tags = tags
	g.cache = make(map[string]CacheEntry)
}

// Getter 是一个感知上下文的配置访问器。
// 对于并发的同一个泛型调用如果不加锁是不安全的，
// 但通常 Getter 是每个请求一个。
// 如果需要并发使用，需要加锁。
// 考虑到“纳秒级”要求，单协程使用时避免锁是首选。
type Getter struct {
	cfg   *Config
	tags  map[string]any
	cache map[string]CacheEntry
}

// Get 获取配置值。
func Get[T any](g *Getter, key string) (T, error) {
	var zero T

	// 1. 获取当前快照
	ss := g.cfg.snapshot.Load().(*Snapshot)

	// 2. L1 缓存检查
	if entry, ok := g.cache[key]; ok {
		if entry.SnapshotHash == ss.AllHash {
			if val, ok := entry.ParsedValue.(T); ok {
				return val, nil
			}
			return zero, fmt.Errorf("type mismatch: %T", entry.ParsedValue)
		}
	}

	// 3. 匹配规则
	rules, ok := ss.Rules[key]
	if !ok {
		return zero, ErrNotFound
	}

	rule := Match(rules, g.tags)
	if rule == nil {
		return zero, ErrNotFound
	}

	// 4. 获取原始值
	rawJSON, ok := ss.Values[rule.ValueHash]
	if !ok {
		// 如果保持数据一致性，不应发生这种情况
		return zero, fmt.Errorf("value missing for hash: %s", rule.ValueHash)
	}

	// 5. 反序列化 (L2 缓存)
	// 我们使用 valueHash + TypeName 作为 Key
	typeKey := rule.ValueHash + "|" + reflect.TypeOf((*T)(nil)).Elem().String()

	var val T
	if cached, ok := g.cfg.valueCache.Load(typeKey); ok {
		val = cached.(T)
	} else {
		if err := json.Unmarshal([]byte(rawJSON), &val); err != nil {
			return zero, fmt.Errorf("unmarshal failed: %w", err)
		}
		g.cfg.valueCache.Store(typeKey, val)
	}

	// 6. 更新 L1 缓存
	g.cache[key] = CacheEntry{
		ParsedValue:  val,
		SnapshotHash: ss.AllHash,
	}

	return val, nil
}
