package bttsetting

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
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}

	// 使用 Type Switch 分别处理不同类型的比较，避免直接 a == b 导致 Panic (当类型不可比较时)
	switch va := a.(type) {
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case bool:
		vb, ok := b.(bool)
		return ok && va == vb
	case int:
		if vb, ok := b.(int); ok {
			return va == vb
		}
	case int64:
		if vb, ok := b.(int64); ok {
			return va == vb
		}
	case float64:
		if vb, ok := b.(float64); ok {
			return va == vb
		}
	}

	// 尝试将两边都转换为 float64 进行比较
	// 这处理了跨类型数字比较，如 int(3) == float64(3.0)
	fa, okA := toFloat64(a)
	if !okA {
		return false
	}

	fb, okB := toFloat64(b)
	if !okB {
		return false
	}

	return fa == fb
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case int:
		return float64(val), true
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case float32:
		return float64(val), true
	case int32:
		return float64(val), true
	case int16:
		return float64(val), true
	case int8:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint64:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint8:
		return float64(val), true
	}
	return 0, false
}
