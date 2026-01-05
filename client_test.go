package bttsetting

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestGet(t *testing.T) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	// mr, _ := miniredis.Run() // Removed
	// rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()}) // Removed
	cfg, err := New(rdb, 1)
	if err != nil {
		// 这里允许错误，因为 miniredis 初始可能没有数据，或者根据逻辑 Load 会忽略缺失版本?
		// Load 在 miniredis 空数据时会返回 nil (Version hash not found -> nil)
		// 所以 New 应该成功。
		t.Fatalf("New failed: %v", err)
	}

	// 准备数据
	rules1 := []Rule{
		{Tags: map[string]any{"city": "bj"}, ValueHash: "h1"},
		{Tags: map[string]any{}, ValueHash: "h2"},
	}

	// 值
	val1, _ := json.Marshal(100)
	val2, _ := json.Marshal(200)

	ss := &Snapshot{
		Version: 1,
		AllHash: "hash1",
		Rules: map[string][]Rule{
			"limit": rules1,
		},
		Values: map[string]string{
			"h1": string(val1),
			"h2": string(val2),
		},
	}
	cfg.snapshot.Store(ss)

	// 测试用例 1: 命中第一条规则
	g := cfg.WithTags(map[string]any{"city": "bj"})
	v, err := Get[int](g, "limit")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if v != 100 {
		t.Errorf("Expected 100, got %v", v)
	}

	// 测试用例 2: 命中默认规则
	g2 := cfg.WithTags(map[string]any{"city": "ny"})
	v2, err := Get[int](g2, "limit")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if v2 != 200 {
		t.Errorf("Expected 200, got %v", v2)
	}

	// 测试用例 3: L1 缓存命中
	// 我们无法轻易检查内部状态，但可以验证其是否正常工作
	v3, err := Get[int](g, "limit")
	if v3 != 100 {
		t.Errorf("Cached Get failed, expected 100, got %v", v3)
	}

	// 测试用例 4: 版本变更导致缓存更新
	val3, _ := json.Marshal(300)
	// 创建指向 h3 的新规则
	rules2 := []Rule{
		{Tags: map[string]any{"city": "bj"}, ValueHash: "h3"}, // Hash 变更
		{Tags: map[string]any{}, ValueHash: "h2"},
	}
	ss2 := &Snapshot{
		Version: 2,
		AllHash: "hash2",
		Rules: map[string][]Rule{
			"limit": rules2,
		},
		Values: map[string]string{
			"h3": string(val3), // h3 -> 300
			"h2": string(val2),
		},
	}
	cfg.snapshot.Store(ss2)

	// g 检测到缓存失效，检查版本并重新加载
	v4, err := Get[int](g, "limit")
	if v4 != 300 {
		t.Errorf("Expected 300 after update, got %v", v4)
	}
}

func TestGet_TypeSafety(t *testing.T) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cfg, err := New(rdb, 1)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	valStr, _ := json.Marshal("hello")
	ss := &Snapshot{
		Version: 1,
		Rules: map[string][]Rule{
			"msg": {{ValueHash: "h1"}},
		},
		Values: map[string]string{
			"h1": string(valStr),
		},
	}
	cfg.snapshot.Store(ss)

	g := cfg.WithTags(nil)

	// 获取字符串
	s, err := Get[string](g, "msg")
	if err != nil || s != "hello" {
		t.Errorf("Get string failed")
	}

	// 获取 int (应因 JSON 反序列化失败而报错)
	_, err = Get[int](g, "msg")
	if err == nil {
		t.Errorf("Expected error unmarshalling string to int")
	}
}

func TestConfig_LoadIncremental(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	SetPrefix("testload:")

	ctx := context.Background()

	// 1. 发布版本 1
	p := NewPublisher(rdb, 1)
	p.Publish(ctx, PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"k1": {{Value: "v1"}},
			"k2": {{Value: "v2"}},
		},
	})

	cfg, _ := New(rdb, 1) // 内部已 Load

	// 获取初始 Snapshot
	ss1 := cfg.snapshot.Load().(*Snapshot)
	h1 := ss1.AllHash

	// 2. 发布版本 1 的变动 (k2 没变，k1 变了)
	p.Publish(ctx, PublishRequest{
		FullReplace: false,
		Items: map[string][]RuleInput{
			"k1": {{Value: "v1-new"}},
		},
	})

	// 3. 再次加载
	err := cfg.Load(ctx)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	ss2 := cfg.snapshot.Load().(*Snapshot)
	if ss2.AllHash == h1 {
		t.Fatal("Hash should have changed")
	}

	// 验证 k2 的 Value 是否是复用的 (内存地址可能不同，但我们可以检查逻辑)
	// 实际上 Load 会 HMGet 缺失的。我们可以通过 miniredis 监控命令或简单验证逻辑。
	v2, _ := Get[string](cfg.WithTags(nil), "k2")
	if v2 != "v2" {
		t.Errorf("k2 value lost: %s", v2)
	}
}

func TestConfig_Watch(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	SetPrefix("testwatch:")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 初始负载
	p := NewPublisher(rdb, 1)
	p.Publish(ctx, PublishRequest{
		FullReplace: true,
		Items:       map[string][]RuleInput{"k": {{Value: "v1"}}},
	})

	cfg, _ := New(rdb, 1)

	// 2. 启动 Watch
	go func() {
		if err := cfg.Watch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			// t.Errorf here might not work well from goroutine
		}
	}()

	// 给 Watcher 一点点启动时间
	time.Sleep(100 * time.Millisecond)

	// 3. 发布更新
	p.Publish(ctx, PublishRequest{
		FullReplace: false,
		Items:       map[string][]RuleInput{"k": {{Value: "v2"}}},
	})

	// 4. 等待自动重载 (可通过轮询 Snapshot 标志位)
	var finalVal string
	for i := 0; i < 20; i++ {
		g := cfg.WithTags(nil)
		val, _ := Get[string](g, "k")
		if val == "v2" {
			finalVal = val
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if finalVal != "v2" {
		t.Errorf("Watcher failed to reload config in time")
	}
}

func TestConfig_ValueCacheGC(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	SetPrefix("testgc:")

	ctx := context.Background()
	p := NewPublisher(rdb, 1)

	// 1. 发布 v1 包含 k1
	p.Publish(ctx, PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"k": {{Value: "original"}},
		},
	})

	cfg, _ := New(rdb, 1)
	g := cfg.WithTags(nil)
	Get[string](g, "k") // 加载到 L2 缓存

	// 检查 L2 缓存是否有值
	found := false
	cfg.valueCache.Range(func(_, _ any) bool {
		found = true
		return false
	})
	if !found {
		t.Fatal("Value should be in L2 cache")
	}

	// 2. 发布 v2，不包含 k1 (FullReplace)
	p.Publish(ctx, PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"other": {{Value: "something"}},
		},
	})

	// 3. 触发 Load
	cfg.Load(ctx)

	// 4. 检查 L2 缓存是否被清理
	// 之前的 key 应该在 GC 步骤被删除了
	found = false
	cfg.valueCache.Range(func(_, _ any) bool {
		found = true
		return true
	})
	// 注意：新版本加载后，"other" 的值还没有被 Get，所以 L2 应该是空的（除非 Load 预加载了所有值到 cache，但目前逻辑是 Get 时加载）
	// Load 仅同步 Snapshot.Values 映射。
	if found {
		// 如果 L2 还有项，检查它是否是旧的项目的 Hash
		// 目前版本中，k 被移除了，所以其对应的 hash 应该不在 ss.Values 中，因此 L2 应该被清理该项。
		t.Errorf("L2 cache should be empty or cleaned of old hashes")
	}
}
