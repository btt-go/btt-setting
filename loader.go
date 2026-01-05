package bttsetting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Load 从 Redis 加载当前版本的配置。
func (c *Config) Load(ctx context.Context) error {
	// 1. 获取 Version 对应的 Hash
	versionsKey := KeyVersions()
	// HGet 只需要获取单个字段
	allHash, err := c.rdb.HGet(ctx, versionsKey, fmt.Sprintf("%d", c.version)).Result()
	if errors.Is(err, redis.Nil) {
		// 初始时，版本不存在时，防止程序无法启动
		return nil
	}
	if err != nil {
		return fmt.Errorf("get version hash failed: %w", err)
	}

	// 2. 加载 Rules (by Hash)
	rulesKey := KeyRules(allHash)
	rulesMap, err := c.rdb.HGetAll(ctx, rulesKey).Result()
	if err != nil {
		return fmt.Errorf("get rules failed: %w", err)
	}

	configItems := make(map[string][]Rule)
	neededHashes := make(map[string]bool)

	for k, v := range rulesMap {
		var rules []Rule
		if err := json.Unmarshal([]byte(v), &rules); err != nil {
			return fmt.Errorf("unmarshal rules failed: %w", err)
		}
		// 确保 Key 匹配
		configItems[k] = rules

		for _, rule := range rules {
			neededHashes[rule.ValueHash] = true
		}
	}

	// 3. 加载 Values
	valuesMap := make(map[string]string)
	missingHashes := make([]string, 0)

	// 获取旧快照以复用 Values
	var oldValues map[string]string
	if oldSS, ok := c.snapshot.Load().(*Snapshot); ok && oldSS != nil {
		oldValues = oldSS.Values
	}

	for h := range neededHashes {
		if val, ok := oldValues[h]; ok {
			valuesMap[h] = val
		} else {
			missingHashes = append(missingHashes, h)
		}
	}

	if len(missingHashes) > 0 {
		// HMGet 仅获取缺失的值
		valuesKey := KeyValues()
		vals, err := c.rdb.HMGet(ctx, valuesKey, missingHashes...).Result()
		if err != nil {
			return fmt.Errorf("get values failed: %w", err)
		}
		for i, v := range vals {
			if v == nil {
				return fmt.Errorf("value %s not found", missingHashes[i])
			}
			if strVal, ok := v.(string); ok {
				valuesMap[missingHashes[i]] = strVal
			}
		}
	}

	// 4. 构建快照
	ss := &Snapshot{
		Version: c.version,
		AllHash: allHash,
		Rules:   configItems,
		Values:  valuesMap,
	}

	// 5. 原子更新
	c.snapshot.Store(ss)

	// 6. 清理 L2 缓存 (GC)
	// 移除不在新快照中的 Hash 对应的值，防止内存泄漏
	c.valueCache.Range(func(key, _ any) bool {
		kStr, ok := key.(string)
		if !ok {
			return true
		}
		// Key format: Hash|Type
		if idx := strings.Index(kStr, "|"); idx > 0 {
			hash := kStr[:idx]
			if _, exists := ss.Values[hash]; !exists {
				c.valueCache.Delete(key)
			}
		}
		return true
	})

	log.Printf("load config success: version=%d, allHash=%s", c.version, allHash)

	return nil
}
