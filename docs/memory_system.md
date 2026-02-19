# PicoClaw Memory 系统说明

本文档说明当前 MemSkill 化 Memory 系统的实现原理、使用方式与注意点。

---

## 一、实现原理

### 1.1 整体架构

Memory 系统分为三层：

- **存储层**（`pkg/agent/memory.go`）：负责长期记忆与每日笔记的读写、备份与原子写入。
- **策略层**（`pkg/agent/memorypolicy.go`）：可配置参数（检索条数、最近几天、会话摘要阈值、长期压缩阈值、进化开关等），支持从配置文件与工作区覆盖文件加载。
- **使用层**：`ContextBuilder` 按当前用户消息做检索并注入 system prompt；`loop` 按策略做会话摘要与长期压缩；可选工具 `memory_search` / `memory_append` 供模型主动查写记忆。

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────────────┐
│  config.json    │────▶│  MemoryPolicy    │────▶│ ContextBuilder / loop   │
│  + overrides    │     │  (策略参数)       │     │ (检索、摘要、压缩、进化)  │
└─────────────────┘     └──────────────────┘     └─────────────────────────┘
                                │
                                ▼
                        ┌──────────────────┐
                        │  MemoryStore     │
                        │  (MEMORY.md +    │
                        │   YYYYMM/DD.md)  │
                        └──────────────────┘
```

### 1.2 存储

- **长期记忆**：`workspace/memory/MEMORY.md`，单文件 Markdown。
- **每日笔记**：`workspace/memory/YYYYMM/YYYYMMDD.md`，按日分文件。
- **备份**：覆盖 `MEMORY.md` 前自动复制到 `memory/backups/YYYYMMDD_HHMMSS_MEMORY.md`。
- **写入方式**：先写临时文件，成功后再 `rename` 覆盖目标文件，避免写一半损坏。
- **策略快照**：策略更新前可把当前覆盖配置写入 `memory/policy_snapshots/YYYYMMDD_HHMMSS.json`，便于回滚。

### 1.3 检索（Retrieve）

- **触发**：每次构建 system prompt 时，用**当前轮用户消息**作为 query 去取记忆。
- **有 query 时**：对 `MEMORY.md` 按 `##` 或 `\n\n` 拆成块，用关键词/子串打分，取 top-N 条（N = 策略中的 `retrieve_limit`），再拼接「最近 N 天」的每日笔记（`recent_days`）。
- **无 query 时（回退）**：注入 `MEMORY.md` 全文 + 最近 N 天每日笔记，与旧版“整段读”行为一致。
- **默认**：`retrieve_limit=10`，`recent_days=3`；可通过配置或策略覆盖修改。

### 1.4 策略参数（MemoryPolicy）

| 参数 | 含义 | 默认值 |
|------|------|--------|
| `retrieve_limit` | 按 query 检索时最多返回的记忆块数 | 10 |
| `recent_days` | 无 query 或拼接时使用的「最近几天」每日笔记 | 3 |
| `session_summary_message_threshold` | 会话消息数超过此值触发摘要 | 20 |
| `session_summary_token_percent` | 会话 token 占 context 比例超过此值触发摘要 | 75 |
| `session_summary_keep_count` | 摘要后保留的最近消息条数 | 4 |
| `session_relevant_history_limit` | 按 query 检索时最多保留的相关轮数；0=关闭按需注入 | 0 |
| `session_relevant_fallback_keep` | 无相关轮时回退保留的最近消息条数；0=不注入任何 history；省略=8 | 8 |
| `long_term_compress_char_threshold` | MEMORY.md 字符数超过此值触发压缩（0=不压缩） | 0 |
| `evolution_enabled` | 是否开启评估/反思/策略更新 | false |

策略来源：先读全局 `config.json` 的 `memory` 块，再叠加工作区 `memory/policy_overrides.json`（若存在）。覆盖文件用于「反思后更新」的持久化，不写回主配置。

### 1.5 会话摘要（Session）

- 当会话历史条数 &gt; `session_summary_message_threshold` 或估计 token &gt; context 的 `session_summary_token_percent`% 时，触发摘要。
- 摘要由 LLM 生成，保留最近 `session_summary_keep_count` 条消息，其余用摘要替代；紧急时还会做 `forceCompression`（丢弃最旧一半对话），同样受策略控制。

### 1.6 长期记忆压缩

- 当 `long_term_compress_char_threshold` &gt; 0 且 `MEMORY.md` 字符数超过该值时，会**异步**触发压缩（最多约 24 小时一次，由 `memory/.last_longterm_compress` 时间戳控制）。
- 流程：备份 → 调用 LLM 总结/压缩 → 原子写回 `MEMORY.md`。压缩结果整份替换，不局部改写。

### 1.7 进化（Evolution）

- 当 `evolution_enabled=true` 时，每约 20 条会话消息会触发一次「反思」：用最近对话 + 当前策略描述调用 LLM，让其输出建议（如 JSON：`{"retrieve_limit": 15}`）。
- 解析建议后写入 `memory/policy_overrides.json`（写前会保存策略快照）。**同一进程内**已加载的 agent 仍用旧策略，新进程或新 agent 会读到更新后的策略。
- 关闭 `evolution_enabled` 即关闭反思与策略更新，行为与「无进化」一致。

---

## 二、使用方式

### 2.1 配置

在 `config.json` 中增加 `memory` 块即可（均为可选，不配则用默认值）：

```json
{
  "memory": {
    "retrieve_limit": 10,
    "recent_days": 3,
    "session_summary_message_threshold": 20,
    "session_summary_token_percent": 75,
    "session_summary_keep_count": 4,
    "long_term_compress_char_threshold": 0,
    "evolution_enabled": false
  }
}
```

- 需要自动压缩长期记忆时，将 `long_term_compress_char_threshold` 设为正数（如 50000）。
- 需要策略自我调整时，将 `evolution_enabled` 设为 `true`（注意下文「注意点」）。

### 2.2 模型侧：写记忆

- **推荐**：使用工具 **`memory_append`**，参数 `content`（要记的内容）和 `slot`（`long_term` 或 `today`）。长期记忆会追加到 `MEMORY.md`，每条自动格式化为 `## YYYY-MM-DD` + 内容，便于检索分块。
- **write_file 写 memory/MEMORY.md**：会被**重定向为追加**（不覆盖），且使用与 memory_append 相同的格式规范；模型用 write_file 写长期记忆时也会得到一致格式，无需改调 memory_append。
- **append_file** 或 write_file 写 `memory/YYYYMM/YYYYMMDD.md` 仍按原样写入每日笔记。

### 2.3 模型侧：查记忆

- **自动**：每轮会用当前用户消息做 query，把相关记忆注入 system prompt，模型无需主动查也能用到。
- **主动**：可调用工具 **`memory_search`**，参数 `query`（关键词或问题）、可选 `limit`（条数，默认 10），返回相关记忆片段。

### 2.4 工作区目录结构

```
workspace/
  memory/
    MEMORY.md              # 长期记忆
    YYYYMM/
      YYYYMMDD.md          # 每日笔记
    backups/               # 写前/压缩前备份
      YYYYMMDD_HHMMSS_MEMORY.md
    policy_snapshots/      # 策略更新前快照（进化时）
      YYYYMMDD_HHMMSS.json
    policy_overrides.json  # 反思产生的策略覆盖（进化时）
    .last_longterm_compress  # 上次长期压缩时间戳
```

---

## 三、注意点

### 3.1 备份与安全

- 覆盖 `MEMORY.md` 前会自动备份到 `memory/backups/`，可定期清理旧备份以省空间。
- 原子写入保证单次写要么完整成功要么不生效，不会出现「写一半」损坏。
- 若怀疑某次压缩或写错，可从 `backups/` 或 `policy_snapshots/` 恢复。

### 3.2 进化（evolution_enabled）

- 开启后会有额外 LLM 调用（反思）和策略文件写入，并可能改变检索/摘要/压缩行为。
- 策略更新仅写入 `policy_overrides.json`，不修改主 `config.json`；若要「关掉进化并恢复默认」，可删除或清空 `memory/policy_overrides.json`。
- 同一进程内已创建的 agent 不会自动重载策略，需新进程或新 agent 才会用到新策略。

### 3.3 检索与 token

- 有 query 时只注入「相关」记忆块 + 最近几天笔记，可减少 system 长度；无 query 时仍是整段 MEMORY + 最近 N 天，与旧版一致。
- 检索为轻量实现（关键词/段落匹配），无向量库；若 MEMORY 很大且 query 很多，可适当提高 `retrieve_limit` 或依赖长期压缩控制体积。

### 3.4 长期压缩

- 仅在配置了 `long_term_compress_char_threshold` &gt; 0 时才会触发，且约 24 小时最多一次。
- 压缩会丢失部分细节，重要内容建议在 prompt 或压缩逻辑中强调「保留关键事实、日期、人名」等（当前实现已尽量保留）。
- 压缩前会备份，出错可从 `backups/` 恢复。

### 3.5 策略覆盖与回滚

- `memory/policy_overrides.json` 会覆盖全局 `config.json` 的 `memory` 段；删除该文件即回退到仅用全局配置。
- 每次通过反思更新前会保存快照到 `policy_snapshots/`，可按时间戳找到上一次「正确」的配置做回滚。

### 3.6 默认回退行为

- 未配置 `memory` 或未提供某字段时，使用文档中的默认值（如 3 天、10 条、20 条摘要阈值等）。
- 无 query、压缩关闭、进化关闭时，行为与「旧版固定规则」一致，便于稳定部署与排错。

---

## 四、通过对话验证策略

以下用「对话 + 观察」的方式验证检索、写入、策略是否按预期工作。建议在本地或测试环境跑通 PicoClaw 后按顺序做。

### 4.1 验证「写入 + 跨轮召回」（长期记忆）

1. **第一轮：让模型记一条事实**
   - 发一句明确要记住的话，例如：「请记住：我的咖啡偏好是无糖拿铁，大杯。」
   - 观察模型是否调用 `memory_append` 或 `write_file` 写 `memory/MEMORY.md`；若未写，可在 prompt 里强调「要记就写 memory」。

2. **第二轮（同会话或新会话）：问需要该事实的问题**
   - 例如：「我上次说的咖啡偏好是什么？」
   - **预期**：模型能答出「无糖拿铁、大杯」或类似。若答对，说明「当前轮 user message 作为 query → 检索 → 注入」这条链是通的。

3. **可选：新会话再问一次**
   - 重新开一个会话（或换一个 session key），再问「我的咖啡偏好是什么？」。
   - 若仍能答对，说明长期记忆被正确读出（整段或按 query 检索）。

### 4.2 验证「按 query 检索」（相关记忆注入）

1. 在 `MEMORY.md` 里事先写好几类内容（或通过多轮对话让模型写入），例如：
   - 一段关于「项目 A 的截止日期是 3 月 15 日」
   - 一段关于「宠物狗叫 Bob」
   - 一段关于「常用邮箱是 alice@example.com」

2. **用不同 query 提问**：
   - 问：「项目 A 什么时候截止？」——应触发含「3 月 15 日」的检索。
   - 问：「我的狗叫什么？」——应触发含「Bob」的检索。
   - 若回答里只出现与当前问题相关的记忆（而不是整份 MEMORY），说明「有 query 时按相关块注入」在工作。

3. **对比无 query 场景**：
   - 若发一条很空的消息（如「你好」），模型拿到的记忆更可能是「整段 + 最近几天」或低相关块；回答里不一定提到 Bob 或项目 A，除非模型主动调 `memory_search`。

### 4.3 验证工具：memory_search / memory_append

1. **memory_search**
   - 直接问：「用 memory_search 查一下和『咖啡』有关的记忆。」
   - 观察模型是否调用 `memory_search`，且返回内容里包含你之前写的咖啡偏好。若包含，说明工具与检索实现一致。

2. **memory_append**
   - 说：「请用 memory_append 把『我最喜欢的颜色是蓝色』记到长期记忆。」
   - 观察是否调用 `memory_append`，且 `slot` 为 `long_term`；然后看 `workspace/memory/MEMORY.md` 是否多了一段内容。再问「我最喜欢的颜色是什么？」验证能召回。

### 4.4 验证策略参数是否生效

1. **retrieve_limit / recent_days**
   - 在 config 里把 `retrieve_limit` 改为 3，`recent_days` 改为 1，重启进程。
   - 在 MEMORY 里准备多于 3 条「块」（用 `##` 或空行分隔），用一句 query 提问。
   - 观察模型回答所依据的记忆是否明显变少（最多约 3 块 + 最近 1 天），可间接说明策略被读取。

2. **session_summary_message_threshold**
   - 将 `session_summary_message_threshold` 设为 5（仅测试用），连续发多轮短消息，总条数超过 5。
   - 观察是否出现「Memory threshold reached. Optimizing conversation history...」或类似提示，且后续对话仍能继续，说明会话摘要触发与保留条数受策略控制。

3. **evolution_enabled 与 policy_overrides**
   - 将 `evolution_enabled` 设为 `true`，进行约 20 条消息的对话。
   - 查看 `workspace/memory/` 下是否出现 `policy_overrides.json` 或 `policy_snapshots/` 下新快照；若有，说明反思与策略更新流程被触发（内容是否合理需人工看一眼）。

### 4.5 验证备份与回滚

1. **写前备份**
   - 先看 `memory/backups/` 下有无文件；用 `memory_append` 或让模型写一次 `MEMORY.md`，再检查是否新增了 `YYYYMMDD_HHMMSS_MEMORY.md`。

2. **策略回滚**
   - 若存在 `memory/policy_overrides.json`，备份一份后删掉该文件，重启或新会话再问一个依赖记忆的问题；应回到「仅用 config 里 memory」的行为。再把 overrides 文件恢复，确认又按覆盖策略生效。

---

## 五、相关代码位置

| 功能 | 文件/包 |
|------|---------|
| 存储、检索、备份、原子写 | `pkg/agent/memory.go` |
| 策略定义、从配置/覆盖加载、反思更新 | `pkg/agent/memorypolicy.go` |
| 上下文构建与 memory 注入 | `pkg/agent/context.go` |
| 会话摘要、长期压缩、反思触发 | `pkg/agent/loop.go` |
| 配置结构 | `pkg/config/config.go`（`MemoryConfig`） |
| 工具 memory_search / memory_append | `pkg/tools/memory.go` |
| 策略快照/备份路径 | `memory/backups/`，`memory/policy_snapshots/` |

---

*文档版本：与当前 MemSkill 化 Memory 实现对应，供配置与排查时参考。*
