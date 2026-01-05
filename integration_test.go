package bttsetting

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestIntegration(t *testing.T) {
	// 1. 初始化 Miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// 2. 初始化发布者 (指定版本 1)
	prefix := "testapp:"
	SetPrefix(prefix) // 全局设置
	op := NewPublisher(rdb, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. 发布版本 1
	req1 := PublishRequest{
		// 第一次发布，或者覆盖
		FullReplace: true,
		Items: map[string][]RuleInput{
			"timeout": {
				{
					Tags:  map[string]any{"env": "prod"},
					Value: 1000,
				},
				{
					Tags:  map[string]any{}, // 默认
					Value: 5000,
				},
			},
		},
	}
	err = op.Publish(ctx, req1)
	if err != nil {
		t.Fatalf("Publish v1 failed: %v", err)
	}

	// 3. 初始化客户端 (指定版本 1) - 在发布之后初始化，应该立即加载到数据
	cfg, err := New(rdb, 1)
	if err != nil {
		t.Fatalf("New config failed: %v", err)
	}

	// 检查 Get
	g := cfg.WithTags(map[string]any{"env": "prod"})
	val, err := Get[int](g, "timeout")
	if err != nil || val != 1000 {
		t.Errorf("Get timeout failed, expected 1000, got %v (err: %v)", val, err)
	}

	// 6. 启动监听器
	go cfg.Watch(ctx)
	time.Sleep(100 * time.Millisecond) // 等待监听器启动

	// 7. 发布版本 2 (客户端仍在 V1，不应受到 V2 影响，除非我们模拟客户端升级)
	// 等等，用户需求是 "int version at init... gray release".
	// 如果 Watcher 收到 Version 2 的更新，但 Client 是 Version 1，它应该忽略吗？
	// 是的，watcher.go 中我们实现了 `if updateMsg.Version == c.appVersion`.
	// 所以如果我想测试“更新”，我应该发布 Version 1 的更新？
	// 或者，如果客户端升级了版本（比如重启或者重新 New），它会加载 V2。
	// 但 "all hash indicates key update" -> 这通常意味着同一个版本下的内容变更。
	// 所以这里我应该测试：发布 Version 1 的新内容。

	req2 := PublishRequest{
		Items: map[string][]RuleInput{
			"timeout": {
				{
					Tags:  map[string]any{"env": "prod"},
					Value: 2000, // 修改值
				},
				{
					Tags:  map[string]any{}, // 默认
					Value: 5000,
				},
			},
		},
	}
	// 发布到 Version 1
	err = op.Publish(ctx, req2)
	if err != nil {
		t.Fatalf("Publish v1 update failed: %v", err)
	}

	// 8. 等待推送传播
	time.Sleep(500 * time.Millisecond) // Miniredis is fast/local

	// 9. 验证更新
	// 应该已更新缓存/快照
	val2, err := Get[int](g, "timeout")
	if err != nil || val2 != 2000 {
		t.Errorf("Get timeout updated failed, expected 2000, got %v (err: %v)", val2, err)
	}
}
