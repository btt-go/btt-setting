package bttsetting

// prefix 目前使用的 Redis Key 前缀
var prefix = "btt-setting:"

// SetPrefix 设置全局 Redis Key 前缀。
// 这应该在任何其他操作之前调用。
func SetPrefix(p string) {
	prefix = p
	if len(prefix) > 0 && prefix[len(prefix)-1] != ':' {
		prefix += ":"
	}
}

// Suffix defs
const (
	SuffixRules    = "rules:"   // 配置规则列表
	SuffixValues   = "values"   // 配置值
	SuffixVersions = "versions" // 版本映射
	SuffixHistory  = "history"  // 版本历史
	SuffixUpdates  = "updates"  // 更新通知
)

// Redis Key Helper

// KeyRules 返回规则集合的 Redis Key。
// hash: 规则集合的全局 AllHash。
func KeyRules(hash string) string {
	return prefix + SuffixRules + hash
}

// KeyValues 返回配置值存储的 Redis Key。
// 该 Hash 存储 ValueHash -> MapValue。
func KeyValues() string {
	return prefix + SuffixValues
}

// KeyVersions 返回版本映射的 Redis Key。
// 该 Hash 存储 AppVersion -> AllHash。
func KeyVersions() string {
	return prefix + SuffixVersions
}

// KeyHistory 返回版本历史记录的 Redis Key。
// 该 List 存储 HistoryRecord JSON 字符串 (RPush)。
func KeyHistory() string {
	return prefix + SuffixHistory
}

// KeyUpdates 返回发布订阅更新通知的 Redis Stream Key。
func KeyUpdates() string {
	return prefix + SuffixUpdates
}

// Stream 事件类型
const (
	EventPublish = "publish"
	EventReload  = "reload"
)

// Redis Stream 消息载荷
type UpdateMessage struct {
	Event     string `json:"event"`     // 事件类型
	Version   int    `json:"version"`   // 版本号
	AllHash   string `json:"all_hash"`  // 全局 Hash
	Timestamp int64  `json:"timestamp"` // 时间戳
}
