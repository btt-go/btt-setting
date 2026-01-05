package bttsetting

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPublisher_Publish(t *testing.T) {
	useMiniredis := true

	// 1. 初始化 Redis
	addr := "127.0.0.1:6379"
	if useMiniredis {
		mr, err := miniredis.Run()
		if err != nil {
			t.Fatalf("Failed to start miniredis: %v", err)
		}
		defer mr.Close()
		addr = mr.Addr()
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})

	// 确保干净的状态
	prefix := "testpub:"
	SetPrefix(prefix)

	// 清理旧数据 (如果有)
	ctx := context.Background()
	keys, _ := rdb.Keys(ctx, prefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	targetVer := 10
	op := NewPublisher(rdb, targetVer)

	// 2. 发布数据
	req := PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"feature_flag": {
				{
					Tags:  map[string]any{"region": "us"},
					Value: true,
				},
				{
					Tags:  map[string]any{},
					Value: false,
				},
			},
		},
	}

	if err := op.Publish(ctx, req); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// 3. 直接验证 Redis 状态

	// 检查版本映射
	verKey := KeyVersions()
	allHash, err := rdb.HGet(ctx, verKey, "10").Result()
	if err != nil {
		t.Fatalf("Failed to get version hash: %v", err)
	}
	if allHash == "" {
		t.Fatal("Version hash shouldn't be empty")
	}

	// 检查历史记录
	histKey := KeyHistory()
	// 期望 List 长度至少为 1，且最后一个元素是本次发布的
	// RPush used, so -1 is the last element
	histJSON, err := rdb.LIndex(ctx, histKey, -1).Result()
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	var hist HistoryRecord
	if err := json.Unmarshal([]byte(histJSON), &hist); err != nil {
		t.Fatalf("Failed to unmarshal history: %v", err)
	}
	if hist.Version != targetVer {
		t.Errorf("History Value mismatch: %d != %d", hist.Version, targetVer)
	}
	if hist.AllHash != allHash {
		t.Errorf("History/Version hash mismatch: %s != %s", hist.AllHash, allHash)
	}

	// 检查规则
	rulesKey := KeyRules(allHash)
	rulesMap, err := rdb.HGetAll(ctx, rulesKey).Result()
	if err != nil {
		t.Fatalf("Failed to get rules: %v", err)
	}
	if len(rulesMap) == 0 {
		t.Fatal("Rules map is empty")
	}

	itemJSON, ok := rulesMap["feature_flag"]
	if !ok {
		t.Fatal("feature_flag rule missing")
	}

	var rules []Rule
	if err := json.Unmarshal([]byte(itemJSON), &rules); err != nil {
		t.Fatalf("Failed to unmarshal item: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rules))
	}

	// 检查值
	valHash := rules[0].ValueHash
	valKey := KeyValues()
	valData, err := rdb.HGet(ctx, valKey, valHash).Result()
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}

	var val bool
	if err := json.Unmarshal([]byte(valData), &val); err != nil {
		t.Fatalf("Unmarshal value failed: %v", err)
	}
	// RuleInput 切片顺序是被保留的。
	// index 0 -> {"region": "us", Value: true}
	// 所以 val 应该是 true。

	// 注意: 将 JSON "true" 反序列化为 bool 应该是有效的。

	// Case: Raw JSON bytes with unordered keys
	// {"b":2,"a":1} -> Should be normalized to {"a":1,"b":2}
	rawJSON := []byte(`{"b":2,"a":1}`)
	reqRaw := PublishRequest{
		Items: map[string][]RuleInput{
			"raw_json": {
				{
					Tags:      nil,
					Value:     rawJSON,
					ValueType: ValueTypeRawJSON,
				},
			},
		},
	}
	if err := op.Publish(ctx, reqRaw); err != nil {
		t.Fatalf("Publish raw json failed: %v", err)
	}

	// Remove cache/rules map fetch logic reuse... just fetch value
	// We need AllHash again.
	verKey = KeyVersions()
	allHash, _ = rdb.HGet(ctx, verKey, "10").Result()
	rulesKey = KeyRules(allHash)
	rawMap, _ := rdb.HGetAll(ctx, rulesKey).Result()
	itemJSON = rawMap["raw_json"]
	var rawRules []Rule
	json.Unmarshal([]byte(itemJSON), &rawRules)

	valHashRaw := rawRules[0].ValueHash
	valDataRaw, _ := rdb.HGet(ctx, KeyValues(), valHashRaw).Result()

	// Check if normalized
	expected := `{"a":1,"b":2}`
	if valDataRaw != expected {
		t.Errorf("Raw JSON normalization failed. Got %s, Expected %s", valDataRaw, expected)
	}
}

func TestPublisher_Incremental(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	SetPrefix("testinc:")

	ctx := context.Background()
	p := NewPublisher(rdb, 1)

	// 1. 初次发布
	req1 := PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"key1": {{Value: "v1"}},
			"key2": {{Value: "v2"}},
		},
	}
	if err := p.Publish(ctx, req1); err != nil {
		t.Fatalf("First publish failed: %v", err)
	}

	// 2. 增量更新 key1, 新增 key3, 删除 key2
	req2 := PublishRequest{
		FullReplace: false, // 增量
		Items: map[string][]RuleInput{
			"key1": {{Value: "v1-new"}},
			"key3": {{Value: "v3"}},
		},
		Deletes: []DeleteOp{
			{Key: "key2"},
		},
	}
	if err := p.Publish(ctx, req2); err != nil {
		t.Fatalf("Incremental publish failed: %v", err)
	}

	// 3. 验证结果
	allHash, _ := rdb.HGet(ctx, KeyVersions(), "1").Result()
	rulesMap, _ := rdb.HGetAll(ctx, KeyRules(allHash)).Result()

	if _, ok := rulesMap["key2"]; ok {
		t.Error("key2 should have been deleted")
	}
	if _, ok := rulesMap["key1"]; !ok {
		t.Fatal("key1 missing")
	}
	if _, ok := rulesMap["key3"]; !ok {
		t.Fatal("key3 missing")
	}

	// 验证 key1 的值
	var r1 []Rule
	json.Unmarshal([]byte(rulesMap["key1"]), &r1)
	valData, _ := rdb.HGet(ctx, KeyValues(), r1[0].ValueHash).Result()
	if valData != `"v1-new"` {
		t.Errorf("Expected v1-new, got %s", valData)
	}
}

func TestPublisher_DeleteByTags(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	SetPrefix("testdel:")

	ctx := context.Background()
	p := NewPublisher(rdb, 1)

	// 1. 发布带有多个 Tag 的规则
	req := PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"key": {
				{Tags: map[string]any{"color": "red"}, Value: 1},
				{Tags: map[string]any{"color": "blue"}, Value: 2},
				{Tags: nil, Value: 0},
			},
		},
	}
	p.Publish(ctx, req)

	// 2. 删除 color=red 的规则
	reqDel := PublishRequest{
		Deletes: []DeleteOp{
			{Key: "key", Tags: map[string]any{"color": "red"}},
		},
	}
	p.Publish(ctx, reqDel)

	// 3. 验证
	allHash, _ := rdb.HGet(ctx, KeyVersions(), "1").Result()
	rulesMap, _ := rdb.HGetAll(ctx, KeyRules(allHash)).Result()
	var rules []Rule
	json.Unmarshal([]byte(rulesMap["key"]), &rules)

	if len(rules) != 2 {
		t.Errorf("Expected 2 rules left, got %d", len(rules))
	}
	for _, r := range rules {
		if r.Tags["color"] == "red" {
			t.Error("Red rule should have been deleted")
		}
	}
}

func TestPublisher_CASMismatch(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	SetPrefix("testcas:")

	ctx := context.Background()
	p := NewPublisher(rdb, 1)

	// 1. 设置初始状态
	req1 := PublishRequest{
		FullReplace: true,
		Items:       map[string][]RuleInput{"key": {{Value: "v1"}}},
	}
	_ = p.Publish(ctx, req1)

	// 2. 注入 Hook 来模拟竞争
	rdb.AddHook(&casHook{
		onAfterHGet: func() {
			// 在 HGet 后，Eval 前修改 Redis 状态
			// 注意：这里需要一个新的客户端或直接操作 miniredis 避免死锁/递归 Hook
			rdb2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer rdb2.Close()
			rdb2.HSet(context.Background(), KeyVersions(), "1", "concurrent-hash")
		},
	})

	req2 := PublishRequest{
		FullReplace: false,
		Items:       map[string][]RuleInput{"key": {{Value: "v2"}}},
	}
	err := p.Publish(ctx, req2)
	if err == nil || !strings.Contains(err.Error(), "version_mismatch") {
		t.Fatalf("Expected version_mismatch error, got %v", err)
	}
}

type casHook struct {
	onAfterHGet func()
	once        sync.Once
}

func (h *casHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *casHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if cmd.Name() == "hget" {
			h.once.Do(h.onAfterHGet)
		}
		return err
	}
}

func (h *casHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestPublisher_Errors(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	SetPrefix("testerror:")
	ctx := context.Background()
	p := NewPublisher(rdb, 1)

	// Case 1: Invalid ValueTypeRawJSON (not []byte or string)
	req1 := PublishRequest{
		Items: map[string][]RuleInput{
			"key": {{Value: 123, ValueType: ValueTypeRawJSON}},
		},
	}
	err := p.Publish(ctx, req1)
	if err == nil || !strings.Contains(err.Error(), "expected []byte or string") {
		t.Errorf("Expected invalid type error, got %v", err)
	}

	// Case 2: Invalid JSON string
	req2 := PublishRequest{
		Items: map[string][]RuleInput{
			"key": {{Value: "{invalid-json}", ValueType: ValueTypeRawJSON}},
		},
	}
	err = p.Publish(ctx, req2)
	if err == nil || !strings.Contains(err.Error(), "invalid json bytes") {
		t.Errorf("Expected invalid json error, got %v", err)
	}
}

func TestMisc_Coverage(t *testing.T) {
	// 1. CalculateHash8
	h8 := CalculateHash8([]byte("test"))
	if len(h8) != 8 {
		t.Errorf("Expected 8 chars, got %d", len(h8))
	}

	// 2. Snapshot.GetRawValue
	ss := &Snapshot{
		Values: map[string]string{"h1": "v1"},
	}
	val, ok := ss.GetRawValue("h1")
	if !ok || val != "v1" {
		t.Errorf("GetRawValue failed")
	}
	_, ok = ss.GetRawValue("h2")
	if ok {
		t.Errorf("Expected not ok for missing hash")
	}
}
