package bttsetting

// Rule 定义单个匹配规则。
type Rule struct {
	Tags      map[string]any `json:"tags"`     // 用于匹配的标签
	ValueHash string         `json:"val_hash"` // 值内容的 Hash
}

// HistoryRecord 版本历史记录
type HistoryRecord struct {
	Version   int    `json:"version"`
	AllHash   string `json:"all_hash"`
	Timestamp int64  `json:"timestamp"`
}

// Snapshot 代表特定版本的配置快照。
type Snapshot struct {
	Version int               // 版本号 (int)
	AllHash string            // 快照内容的全局 Hash (用于缓存失效)
	Rules   map[string][]Rule // Key -> Rules
	Values  map[string]string // ValueHash -> RawJSON
}

// CacheEntry 是存储在 Getter 中的 L1 缓存条目。
type CacheEntry struct {
	ParsedValue  any
	SnapshotHash string
}

// ValueCacheItem 是 L2 缓存项 (泛型解析对象)。
type ValueCacheItem struct {
	Value any
}

// GetRawValue 获取原始值（根据 Hash）。
func (s *Snapshot) GetRawValue(valueHash string) (string, bool) {
	val, ok := s.Values[valueHash]
	return val, ok
}
