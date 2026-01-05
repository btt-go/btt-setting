package bttsetting

import (
	"reflect"
)

// Match 为给定的输入标签查找最佳匹配规则。
// 规则按照 Slice 顺序匹配，一旦匹配成功立即返回（列表顺序即优先级）。
func Match(rules []Rule, inputTags map[string]any) *Rule {
	for i := range rules {
		rule := &rules[i]
		if matchOne(rule, inputTags) {
			return rule
		}
	}
	// 如果存在但未匹配到，则回退到空标签规则（默认规则）
	// (如果列表中包含空标签规则，应该在循环中处理。
	// 通常空标签规则优先级最低)
	return nil
}

// MatchTagsExact 检查两个 Tag map 是否完全相等
func MatchTagsExact(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if v2, ok := b[k]; !ok || v != v2 {
			return false
		}
	}
	return true
}

// matchOne 检查 rule.Tags 是否是 inputTags 的子集且值相等。
func matchOne(rule *Rule, inputTags map[string]any) bool {
	// 如果规则没有标签，它匹配所有情况（默认规则）
	if len(rule.Tags) == 0 {
		return true
	}

	// 如果输入的标签少于规则的标签，则无法匹配
	if len(inputTags) < len(rule.Tags) {
		return false
	}

	for key, ruleVal := range rule.Tags {
		inputVal, ok := inputTags[key]
		if !ok {
			return false // 输入中缺少标签 Key
		}

		// 简单的相等性检查
		if ruleVal != inputVal {
			// 处理数字类型不匹配 (例如 int vs float64 来自 JSON)
			if !valuesEqual(ruleVal, inputVal) {
				return false
			}
		}
	}
	return true
}

func valuesEqual(a, b any) bool {
	if a == b {
		return true
	}

	// 数字比较辅助方法
	// JSON unmarshal 通常产生 float64，而输入可能是 int
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)

	if isNumber(va.Kind()) && isNumber(vb.Kind()) {
		// 统一转换为 float64 进行比较
		fa, _ := toFloat(va)
		fb, _ := toFloat(vb)
		return fa == fb
	}

	return false
}

func isNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func toFloat(v reflect.Value) (float64, bool) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	}
	return 0, false
}
