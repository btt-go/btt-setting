package bttsetting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"encoding/json"

	"github.com/redis/go-redis/v9"
)

// Publisher 处理（如发布）操作。
type Publisher struct {
	rdb     *redis.Client
	version int
}

// NewPublisher 创建发布者。
// client: Redis 客户端实例（外部传入，DI）。
// version: 本次操作针对的目标版本。
func NewPublisher(client *redis.Client, version int) *Publisher {
	return &Publisher{
		rdb:     client,
		version: version,
	}
}

// PublishRequest 代表发布新配置的请求。
type PublishRequest struct {
	// FullReplace 如果为 true，忽略当前版本已有内容，直接使用 Items 作为该版本的全部内容。
	// 如果 Items 为空且 FullReplace 为 true，则相当于创建一个空版本。
	FullReplace bool

	// Items 要更新或新增的 ConfigKey -> RuleList。
	// 这里会替换对应 ConfigKey 的所有规则。
	Items map[string][]RuleInput

	// Deletes 要删除的 ConfigKey 或特定的 Tag 组合。
	Deletes []DeleteOp
}

// DeleteOp 删除操作
type DeleteOp struct {
	Key  string
	Tags map[string]any // 如果为 nil，删除整个 Key；否则仅删除匹配 Tags 的规则
}

const (
	ValueTypeObject  = 0 // 普通对象 (Any)
	ValueTypeRawJSON = 1 // 预序列化的 JSON 字节 ([]byte)
)

type RuleInput struct {
	Tags      map[string]any
	Value     any
	ValueType int
}

// Publish 将新版本的配置推送到 Redis。
func (p *Publisher) Publish(ctx context.Context, req PublishRequest) error {
	// 1. 获取当前版本的基础 Hash (用于 CAS 和增量更新)
	versionsKey := KeyVersions()
	baseHash, err := p.rdb.HGet(ctx, versionsKey, fmt.Sprintf("%d", p.version)).Result()
	if errors.Is(err, redis.Nil) {
		baseHash = ""
		err = nil
	}
	if err != nil {
		return fmt.Errorf("get current version failed: %w", err)
	}

	currentItems := make(map[string][]Rule)

	if !req.FullReplace && baseHash != "" {
		// 加载当前版本的规则 (仅当非 FullReplace 且存在旧版本时)
		rulesKey := KeyRules(baseHash)
		rawMap, err := p.rdb.HGetAll(ctx, rulesKey).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return fmt.Errorf("load current version rules failed: %w", err)
		}

		for k, v := range rawMap {
			var rules []Rule
			if err := json.Unmarshal([]byte(v), &rules); err != nil {
				return fmt.Errorf("unmarshal config item %s failed: %w", k, err)
			}
			currentItems[k] = rules
		}
	}

	// 2. 应用删除 (Deletes)
	for _, del := range req.Deletes {
		if del.Tags == nil {
			// Tags 为 nil，删除整个 Key
			delete(currentItems, del.Key)
		} else {
			// 删除特定规则 (Tags 可能是空 map，表示删除无 Tag 的规则)
			if rules, ok := currentItems[del.Key]; ok {
				newRules := make([]Rule, 0, len(rules))
				for _, r := range rules {
					// 只有 Tags 不完全匹配时才保留 (即删除精确匹配的)
					if !MatchTagsExact(r.Tags, del.Tags) {
						newRules = append(newRules, r)
					}
				}
				currentItems[del.Key] = newRules
				if len(currentItems[del.Key]) == 0 {
					delete(currentItems, del.Key)
				}
			}
		}
	}

	// 3. 应用更新 (Items) - 覆盖/新增 Key 级别的规则列表
	valueMap := make(map[string][]byte) // Hash -> RawJSON 收集新值

	for key, inputs := range req.Items {
		var rules []Rule
		for _, input := range inputs {
			valToHash := input.Value

			if input.ValueType == ValueTypeRawJSON {
				// 如果是 Raw JSON，先反序列化为 any
				// 支持 []byte 和 string
				var rawBytes []byte
				if b, ok := input.Value.([]byte); ok {
					rawBytes = b
				} else if s, ok := input.Value.(string); ok {
					rawBytes = []byte(s)
				} else {
					return fmt.Errorf("invalid value type for RawJSON key %s: expected []byte or string", key)
				}

				if err := json.Unmarshal(rawBytes, &valToHash); err != nil {
					return fmt.Errorf("invalid json bytes for key %s: %w", key, err)
				}
			}

			valHash, rawData, err := ComputeValueHash(valToHash)
			if err != nil {
				return fmt.Errorf("failed to hash value for key %s: %w", key, err)
			}
			valueMap[valHash] = rawData

			rules = append(rules, Rule{
				Tags:      input.Tags,
				ValueHash: valHash,
			})
		}

		currentItems[key] = rules
	}

	// 4. 计算新状态的 AllHash
	allHash := ComputeAllHash(currentItems)

	// 5. 存储 (分两步：1. 写入数据 2. CAS 更新版本与通知)

	// Step 1: 写入 Values 和 Rules (幂等，并发写安全)
	pipe := p.rdb.Pipeline()

	// 写入 Values (NX)
	for h, data := range valueMap {
		pipe.HSetNX(ctx, KeyValues(), h, data)
	}

	// 写入 Rules (以 AllHash 为 Key)
	rulesKey := KeyRules(allHash)
	for k, rules := range currentItems {
		itemJSON, _ := json.Marshal(rules)
		pipe.HSet(ctx, rulesKey, k, itemJSON)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to save rules/values: %w", err)
	}

	// Step 2: CAS 更新版本并发送通知 (Lua 脚本)
	// 如果 Version 对应的 Hash 发生了变化（不等于 baseHash），则拒绝更新。

	luaScript := `
		local versionKey = KEYS[1]
		local historyKey = KEYS[2]
		local streamKey = KEYS[3]
		
		local version = ARGV[1]
		local oldHash = ARGV[2]
		local newHash = ARGV[3]
		local historyJSON = ARGV[4]
		local streamData = ARGV[5]
		
		-- 检查当前 Version 的 Hash
		local currentHash = redis.call('HGET', versionKey, version)
		
		-- 处理 nil 情况 (转为空字符串)
		if currentHash == false then
			currentHash = ""
		end
		
		if currentHash ~= oldHash then
			return redis.error_reply('version_mismatch: ' .. currentHash .. ' != ' .. oldHash)
		end
		
		-- 执行更新
		redis.call('HSET', versionKey, version, newHash)
		redis.call('RPUSH', historyKey, historyJSON)
		redis.call('XADD', streamKey, 'MAXLEN', '~', '1000', '*', 'data', streamData)
		
		return "OK"
	`

	// 准备参数
	now := time.Now().Unix()

	// History
	histRecord := HistoryRecord{
		Version:   p.version,
		AllHash:   allHash,
		Timestamp: now,
	}
	histJSON, _ := json.Marshal(histRecord)

	// Stream
	msg := UpdateMessage{
		Event:     EventPublish,
		Version:   p.version,
		AllHash:   allHash,
		Timestamp: now,
	}
	msgData, _ := json.Marshal(msg)

	// BaseHash 处理 (如果没读到，则是空字符串)
	// 在开头我们读到了 baseHash (string)

	keys := []string{
		KeyVersions(),
		KeyHistory(),
		KeyUpdates(),
	}

	argv := []any{
		fmt.Sprintf("%d", p.version), // ARGV[1] Version
		baseHash,                     // ARGV[2] OldHash
		allHash,                      // ARGV[3] NewHash
		string(histJSON),             // ARGV[4] Value
		string(msgData),              // ARGV[5] Stream Data
	}

	_, err = p.rdb.Eval(ctx, luaScript, keys, argv...).Result()
	if err != nil {
		return fmt.Errorf("cas update failed: %w", err)
	}

	return nil
}
