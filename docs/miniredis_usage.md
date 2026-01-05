# Miniredis 库使用说明

`github.com/alicebob/miniredis/v2` 是一个纯 Go 语言实现的 Redis 服务器，主要用于 Go 语言的单元测试。

## 核心用途
它允许在测试代码中直接运行一个“虚假”的 Redis 服务器，从而避免了以下麻烦：
- 在宿主机上安装 Redis。
- 运行 Docker 容器。
- 在 CI/CD 过程中管理外部依赖。

## 主要特性
1.  **快速且轻量**：在协程内运行，启动和销毁极快。
2.  **状态隔离**：每个测试都可以启动自己的实例，确保数据隔离。
3.  **真实 Redis 协议**：兼容标准的 Redis 客户端（例如 `go-redis`）。
4.  **测试辅助工具**：
    - `mr.FastForward(d)`:模拟时间流逝（非常适合 TTL 过期测试）。
    - `mr.Set(key, val)`: 直接注入数据，无需通过客户端命令。
    - `mr.CheckGet(t, key, val)`: 针对 Key 值的断言。

## 使用示例

```go
package main_test

import (
	"testing"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestSomethingWithRedis(t *testing.T) {
	// 1. 启动一个 miniredis 实例
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("无法启动 miniredis: %s", err)
	}
	defer mr.Close() // 测试结束后关闭

	// 2. 配置 Redis 客户端使用 miniredis 的地址
	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 3. 执行业务逻辑
	ctx := t.Context()
	err = rdb.Set(ctx, "foo", "bar", 0).Err()
	assert.NoError(t, err)

	// 4. 验证结果
	// 方式 A: 使用客户端查询
	val, _ := rdb.Get(ctx, "foo").Result()
	assert.Equal(t, "bar", val)

	// 方式 B: 使用 miniredis 辅助方法进行断言
	mr.CheckGet(t, "foo", "bar")
}
```
