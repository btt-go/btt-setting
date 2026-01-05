package bttsetting

import (
	"testing"
)

func TestMatch(t *testing.T) {
	rules := []Rule{
		{Tags: map[string]any{"city": "bj"}, ValueHash: "v1"},
		{Tags: map[string]any{}, ValueHash: "v2"},
	}

	// Case 1: 精确匹配
	r := Match(rules, map[string]any{"city": "bj", "uid": 1})
	if r == nil || r.ValueHash != "v1" {
		t.Errorf("Expected v1, got %v", r)
	}

	// Case 2: 匹配默认规则 (输入不匹配 v1，回退到 v2)
	r = Match(rules, map[string]any{"city": "sh"})
	if r == nil || r.ValueHash != "v2" {
		t.Errorf("Expected v2, got %v", r)
	}

	// Case 3: 输入缺少 tag，无法匹配 v1
	r = Match(rules, map[string]any{"other": "val"})
	if r == nil || r.ValueHash != "v2" {
		t.Errorf("Expected v2 (default), got %v", r)
	}

	// Case 4: 空规则列表
	var emptyRules []Rule
	if Match(emptyRules, map[string]any{"a": 1}) != nil {
		t.Error("Expected nil for empty rules")
	}
}

func TestMatch_Priority(t *testing.T) {
	rules := []Rule{
		{Tags: map[string]any{"a": 1, "b": 2}, ValueHash: "v1"},
		{Tags: map[string]any{"a": 1}, ValueHash: "v2"},
		{Tags: map[string]any{}, ValueHash: "v3"},
	}

	// 匹配最具体的 (因为 v1 在前面)
	r := Match(rules, map[string]any{"a": 1, "b": 2, "c": 3})
	if r == nil || r.ValueHash != "v1" {
		t.Errorf("Expected v1, got %v", r)
	}

	// 顺序决定优先级
	rulesSwapped := []Rule{
		{Tags: map[string]any{"a": 1}, ValueHash: "v2"},
		{Tags: map[string]any{"a": 1, "b": 2}, ValueHash: "v1"},
	}
	r = Match(rulesSwapped, map[string]any{"a": 1, "b": 2})
	if r == nil || r.ValueHash != "v2" {
		t.Errorf("Expected v2 due to order, got %v", r)
	}
}

func TestMatch_NumericTypes(t *testing.T) {
	// 验证各种数字类型的相互匹配
	tests := []struct {
		name     string
		ruleVal  any
		inputVal any
		want     bool
	}{
		{"int_vs_float64", 1, 1.0, true},
		{"int_vs_float64_diff", 1, 1.1, false},
		{"int8_vs_int", int8(5), int(5), true},
		{"uint_vs_int", uint(10), int(10), true},
		{"float32_vs_float64", float32(3.14), 3.14, false}, // float32 转 float64 精度问题，通常不相等
		{"float32_vs_float64_integer", float32(3.0), 3.0, true},
		{"int64_vs_int32", int64(100), int32(100), true},
		{"uint8_vs_int", uint8(255), int(255), true},
		{"string_numeric_mismatch", "123", 123, false},
		{"bool_mismatch", true, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := []Rule{
				{Tags: map[string]any{"key": tt.ruleVal}, ValueHash: "matched"},
			}
			r := Match(rules, map[string]any{"key": tt.inputVal})
			got := r != nil && r.ValueHash == "matched"
			if got != tt.want {
				t.Errorf("Match(%v, %v) = %v; want %v", tt.ruleVal, tt.inputVal, got, tt.want)
			}
		})
	}
}

func TestValuesEqual_Comprehensive(t *testing.T) {
	// 直接测试底层的 valuesEqual 函数，覆盖所有 switch 分支
	tests := []struct {
		a, b any
		want bool
	}{
		// Same types
		{1, 1, true},
		{"s", "s", true},
		{true, true, true},
		{nil, nil, true},

		// Mixed Integer types
		{int(1), int8(1), true},
		{int(1), int16(1), true},
		{int(1), int32(1), true},
		{int(1), int64(1), true},
		{int(1), uint(1), true},
		{int(1), uint8(1), true},
		{int(1), uint16(1), true},
		{int(1), uint32(1), true},
		{int(1), uint64(1), true},

		// Floats to Integers
		{1.0, int(1), true},
		{1.0, int64(1), true},
		{float32(1.0), int(1), true},

		// Values not equal
		{1, 2, false},
		{1, 1.0000001, false},
		{"1", 1, false}, // String vs Number
		{true, 1, false},
		// {struct{}{}, struct{}{}, true}, // Empty structs checks equality (Removed: generic struct comparison relies on reflection or direct interface comparison which risks panic on uncomparables)
		// slice/map 等不可比较类型，valuesEqual 返回 false (避免 panic)
	}

	for i, tt := range tests {
		if got := valuesEqual(tt.a, tt.b); got != tt.want {
			t.Errorf("Case %d: valuesEqual(%v (%T), %v (%T)) = %v; want %v", i, tt.a, tt.a, tt.b, tt.b, got, tt.want)
		}
	}
}

func TestMatchTagsExact(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]any
		b    map[string]any
		want bool
	}{
		{
			"Equal empty",
			map[string]any{},
			map[string]any{},
			true,
		},
		{
			"Equal simple",
			map[string]any{"a": 1, "b": "2"},
			map[string]any{"a": 1, "b": "2"},
			true,
		},
		{
			"Different lengths",
			map[string]any{"a": 1},
			map[string]any{"a": 1, "b": 2},
			false,
		},
		{
			"Different keys",
			map[string]any{"a": 1},
			map[string]any{"b": 1},
			false,
		},
		{
			"Different values",
			map[string]any{"a": 1},
			map[string]any{"a": 2},
			false,
		},
		// 注意: MatchTagsExact 当前实现是严格相等的，不支持数字弱类型匹配
		{
			"Mixed types match failure",
			map[string]any{"a": 1},
			map[string]any{"a": 1.0},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchTagsExact(tt.a, tt.b); got != tt.want {
				t.Errorf("MatchTagsExact() = %v, want %v", got, tt.want)
			}
		})
	}
}
