# BTT Setting 配置管理库

基于 Redis 实现的高性能动态配置管理库。支持动态更新、版本控制、多维规则匹配（Tags）以及极致的读取性能（L1/L2 多级缓存）。

## 核心特性

*   **多维配置**: 支持基于标签（如 `env=prod`, `city=bj`）下发不同的配置值。
*   **版本控制**: 完整的配置版本历史追踪。
*   **动态更新**: 基于 Redis Stream 实现毫秒级配置变更通知与热加载。
*   **极致性能**:
    *   **L1 缓存**: `Getter` 上下文内的本地内存缓存（请求级），无锁设计。
    *   **L2 缓存**: 协程级共享的值缓存，减少 JSON 反序列化开销。
    *   **内容寻址**: 基于内容 Hash (CAS) 存储，全局自动去重。
*   **增量加载**: 更新配置时仅从 Redis 拉取发生变更的数据，最小化 IO 与网络开销。
*   **原子快照**: 配置更新是原子的，读取者永远看到一致的配置快照视图，无中间状态。

## 安装

```bash
go get github.com/btt-go/btt-setting
```

## 快速开始

### 1. 初始化客户端

使用 Redis 连接和应用版本号初始化配置客户端。

```go
import (
    "context"
    "github.com/redis/go-redis/v9"
    "github.com/btt-go/btt-setting"
)

func main() {
    rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
    
    // 初始化配置 (加载版本 1)
    // New 方法会立即从 Redis 加载配置，如果加载失败会返回错误
    cfg, err := bttsetting.New(rdb, 1)
    if err != nil {
        panic(err)
    }

    // 启动后台监听器以接收动态更新
    ctx := context.Background()
    go cfg.Watch(ctx)
}
```

### 2. 获取配置

使用 `WithTags` 创建一个感知上下文的 Getter，然后获取类型安全的值。

```go
// 创建带有特定标签的 Getter
tags := map[string]any{
    "city": "bj",
    "env":  "prod",
}
getter := cfg.WithTags(tags)

// 获取配置值 (支持泛型)
timeout, err := bttsetting.Get[int](getter, "timeout")
if err != nil {
    // 处理错误 (例如: 配置项不存在)
}
fmt.Println("Timeout:", timeout)
```

### 3. 发布配置 (管理端)

使用 `Publisher` 推送新的配置版本。

```go
publisher := bttsetting.NewPublisher(rdb, 1) // 目标版本 1

req := bttsetting.PublishRequest{
    FullReplace: true,
    Items: map[string][]bttsetting.RuleInput{
        "timeout": {
            // 规则 1: 生产环境
            {
                Tags:  map[string]any{"env": "prod"},
                Value: 1000,
            },
            // 规则 2: 默认值 (空标签匹配所有)
            {
                Tags:  map[string]any{}, 
                Value: 5000,
            },
        },
    },
}

err := publisher.Publish(ctx, req)
```

### Raw JSON 支持

对于已经预序列化好的 JSON 数据（`[]byte`），指定 `ValueTypeRawJSON` 可以在存储前自动进行规范化处理（Key 排序），确保 Hash 计算的一致性。

```go
rawJSON := []byte(`{"b": 2, "a": 1}`) // Key 乱序的 JSON
req := bttsetting.PublishRequest{
    Items: map[string][]bttsetting.RuleInput{
        "my_config": {
            {
                Tags:      nil,
                Value:     rawJSON,
                ValueType: bttsetting.ValueTypeRawJSON, // 显式指定类型
            },
        },
    },
}
```

## 性能基准

Apple M4 芯片下的 Benchmark 测试结果：

```
BenchmarkGet-10                 178088917                6.256 ns/op
BenchmarkGet_Parallel-10        1000000000               1.042 ns/op
```

## Redis 数据结构

详细的 Redis 存储结构说明请参考 [docs/redis_schema.md](docs/redis_schema.md)。

## License

MIT
