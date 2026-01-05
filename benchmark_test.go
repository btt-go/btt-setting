package bttsetting

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func BenchmarkGet(b *testing.B) {
	// Setup Redis and Config
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	// 1. Publish a config
	op := NewPublisher(rdb, 1)
	req := PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"bench_key": {
				{
					Tags:      map[string]any{"env": "prod"},
					Value:     100,
					ValueType: ValueTypeObject,
				},
				{
					Tags:      map[string]any{},
					Value:     200,
					ValueType: ValueTypeObject,
				},
			},
		},
	}
	if err := op.Publish(ctx, req); err != nil {
		b.Fatalf("Publish failed: %v", err)
	}

	// 2. Client Init
	cfg, err := New(rdb, 1)
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	// 3. Prepare Getter
	g := cfg.WithTags(map[string]any{"env": "prod"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		val, err := Get[int](g, "bench_key")
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
		if val != 100 {
			b.Fatalf("Value mismatch")
		}
	}
}

func BenchmarkGet_Parallel(b *testing.B) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	op := NewPublisher(rdb, 1)
	req := PublishRequest{
		FullReplace: true,
		Items: map[string][]RuleInput{
			"bench_key": {
				{
					Tags:      map[string]any{"env": "prod"},
					Value:     100,
					ValueType: ValueTypeObject,
				},
			},
		},
	}
	op.Publish(ctx, req)
	cfg, _ := New(rdb, 1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		g := cfg.WithTags(map[string]any{"env": "prod"})
		for pb.Next() {
			_, _ = Get[int](g, "bench_key")
		}
	})
}
