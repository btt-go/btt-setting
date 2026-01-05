package bttsetting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// CalculateHash8 返回 SHA256 Hex 字符串的前 8 位 (用于 AllHash)。
func CalculateHash8(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:8]
}

// CalculateHash16 返回 SHA256 Hex 字符串的前 16 位 (用于 ValueHash)。
func CalculateHash16(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

func ComputeValueHash(val any) (string, []byte, error) {
	// encoding/json 默认会对 map keys 进行排序，保证了 determinism。
	data, err := json.Marshal(val)
	if err != nil {
		return "", nil, err
	}
	return CalculateHash16(data), data, nil
}

// ComputeAllHash 计算配置集的全局 Hash。
// 它遍历所有配置项，按 Key 排序，并对它们的元数据组合进行 Hash。
func ComputeAllHash(items map[string][]Rule) string {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		rules := items[k]
		// Hash 格式: Key + RulesHash
		// 这里简单序列化 Item
		data, _ := json.Marshal(rules)
		h.Write(data)
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)[:8]
}
