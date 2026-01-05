package bttsetting

import (
	"testing"
)

func TestMatch(t *testing.T) {
	rules := []Rule{
		{Tags: map[string]any{"city": "bj"}, ValueHash: "v1"},
		{Tags: map[string]any{}, ValueHash: "v2"},
	}

	// 精确匹配
	r := Match(rules, map[string]any{"city": "bj", "uid": 1})
	if r == nil || r.ValueHash != "v1" {
		t.Errorf("Expected v1, got %v", r)
	}

	// 匹配默认
	r = Match(rules, map[string]any{"city": "sh"})
	if r == nil || r.ValueHash != "v2" {
		t.Errorf("Expected v2, got %v", r)
	}

	// 匹配类型转换 (int vs float64)
	rules2 := []Rule{
		{Tags: map[string]any{"level": 1.0}, ValueHash: "f1"},
	}
	r = Match(rules2, map[string]any{"level": 1})
	if r == nil || r.ValueHash != "f1" {
		t.Errorf("Expected f1, got %v", r)
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
