package bttsetting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Watch 开始监听 Update Stream。
// 它是阻塞的，应在 goroutine 中运行。
func (c *Config) Watch(ctx context.Context) error {
	// 使用 $ 只读取新消息
	lastID := "$"
	streamKey := KeyUpdates()

	// 内部函数：检查版本一致性
	checkConsistency := func() {
		// 获取远程最新 Hash
		versionsKey := KeyVersions()
		remoteHash, err := c.rdb.HGet(ctx, versionsKey, fmt.Sprintf("%d", c.version)).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			log.Printf("check consistency failed: %v", err)
			return
		}

		// 比较本地和远程
		currentSS := c.snapshot.Load().(*Snapshot)
		if remoteHash != "" && remoteHash != currentSS.AllHash {
			log.Printf("version hash mismatch detected, reloading: local=%s, remote=%s", currentSS.AllHash, remoteHash)
			_ = c.Load(ctx)
		}
	}

	// 1. 启动时立即检查一次（防止 New 和 Watch 之间的 Gap 导致漏更）
	checkConsistency()

	// 定期反熵检查 (1分钟)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			checkConsistency()
		default:
		}

		// 阻塞读取
		streams, err := c.rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamKey, lastID},
			Block:   5 * time.Second,
			Count:   1,
		}).Result()

		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			log.Printf("watch failed: %v", err)
			// 退避等待，防止死循环刷日志
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				lastID = msg.ID

				dataStr, ok := msg.Values["data"].(string)
				if !ok {
					continue
				}

				var updateMsg UpdateMessage
				if err := json.Unmarshal([]byte(dataStr), &updateMsg); err != nil {
					continue
				}

				// 处理更新
				// 仅当发布的版本号与当前客户端应用版本一致时才加载
				if updateMsg.Version == c.version {
					// 加载新配置
					_ = c.Load(ctx)
				}
			}
		}
	}
}
