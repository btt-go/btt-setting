# Redis 数据结构说明

默认前缀: `btt-setting:` (可通过 `SetPrefix` 修改)

### 1. 规则集合 (Rules)
*   **Key**: `btt-setting:rules:{AllHash}`
*   **Type**: `Hash`
*   **Field**: `{ConfigKey}` (配置项的 Key，如 `timeout`)
*   **Value**: `JSON` List (规则列表 `[{"tags":..., "val_hash":...}, ...]`)
*   **说明**: 无状态存储。每次配置集变更都会生成新的 `AllHash` 及对应的 Key。

### 2. 值存储 (Values)
*   **Key**: `btt-setting:values`
*   **Type**: `Hash`
*   **Field**: `{ValueHash}` (值的哈希，16位前缀)
*   **Value**: `JSON` (实际的配置值)
*   **说明**: 内容寻址存储 (CAS)，全局去重复用。

### 3. 版本映射 (Versions)
*   **Key**: `btt-setting:versions`
*   **Type**: `Hash`
*   **Field**: `{AppVersion}` (整数版本号，如 `1`)
*   **Value**: `{AllHash}` (对应规则集合的哈希)
*   **说明**: 指向该版本当前生效的配置快照。

### 4. 变更通知 (Updates)
*   **Key**: `btt-setting:updates`
*   **Type**: `Stream`
*   **Fields**:
    *   `data`: `JSON` (包含 `event`, `version`, `all_hash`, `timestamp`)
*   **说明**: 发布更新时写入，客户端监听此 Stream 触发重载。固定长度 (MaxLen 1000)。

### 5. 版本历史 (History)
*   **Key**: `btt-setting:history`
*   **Type**: `List`
*   **Value**: `JSON` List of `HistoryRecord`
    *   Structure: `{"version": int, "all_hash": string, "timestamp": int64}`
*   **说明**: 记录所有发布的历史记录，用于审计或回滚。每次发布新记录追加到列表尾部 (RPush)。
