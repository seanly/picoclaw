# sciclaw HOOK 功能分析与 picoclaw 移植评估

## 一、sciclaw 中 HOOK 功能概览

### 1.1 作用

HOOK 在 agent 生命周期中的**固定节点**触发，用于：

- **审计与可复现**：记录 turn/LLM/tool 等事件到 JSONL（如 `workspace/hooks/hook-events.jsonl`）
- **策略与合规**：按工作区策略（`HOOKS.md` + `hooks.yaml`）启用/禁用事件、配置脱敏、说明文案
- **扩展**：第三方可实现 `hooks.Handler`，在 before/after turn、before/after LLM、before/after tool、on_error 等时机执行逻辑

### 1.2 事件类型（`pkg/hooks/types.go`）

| 事件 | 含义 |
|------|------|
| `before_turn` | 用户消息进入，开始处理一轮对话前 |
| `after_turn` | 本轮对话结束（已得到最终回复） |
| `before_llm` | 每次调用 LLM 前 |
| `after_llm` | 每次 LLM 返回后 |
| `before_tool` | 每次执行工具调用前 |
| `after_tool` | 每次工具执行完成后 |
| `on_error` | 出错时（LLM 失败、工具报错等） |

### 1.3 核心组件

- **`pkg/hooks`**
  - `types.go`：`Event`、`Context`、`Result`、`Handler`、`AuditEntry`、`AuditSink`
  - `dispatcher.go`：按事件类型注册多个 `Handler`，`Dispatch()` 串行执行并写审计
  - `audit_jsonl.go`：实现 `AuditSink`，追加 JSONL 到文件
  - `builtin/provenance.go`：内置 handler，记录 event/turn_id/session 等元数据
  - `builtin/policy.go`：内置 handler，按工作区策略决定是否启用、并写入策略相关 metadata

- **`pkg/hookpolicy`**
  - `model.go`：策略结构（`Policy`、`EventPolicy`），默认策略
  - `load.go`：`LoadPolicy(workspace)`，合并 `HOOKS.md` 与 `hooks.yaml`（yaml 覆盖 md）
  - `nl_parser.go`：解析 `HOOKS.md`（自然语言 + `## before_turn` 等标题）
  - `yaml_parser.go`：解析 `hooks.yaml`（enabled、events、audit、redaction 等）

### 1.4 在 agent 中的使用（sciclaw `pkg/agent/loop.go`）

- **构造**：`NewAgentLoop` 内根据 `hookpolicy.LoadPolicy(workspace)` 决定是否启用 hook、是否开审计；创建 `hooks.NewDispatcher(auditSink)`，按策略为各事件注册 `ProvenanceHandler` 和 `PolicyHandler`。
- **TurnID**：每轮生成 `turnID`（如 `nextTurnID()`），传入各 hook 的 `Context`。
- **调用点**：
  - 进入 `processMessage` 主流程后：`EventBeforeTurn`
  - LLM 迭代失败返回前：`EventOnError`
  - 一轮正常结束：`EventAfterTurn`
  - 每次 LLM 调用前/后：`EventBeforeLLM`、`EventAfterLLM`
  - 每个 tool 调用前/后：`EventBeforeTool`、`EventAfterTool`；若 tool 返回错误再发 `EventOnError`
- **脱敏**：对用户消息、LLM 摘要、工具参数/结果等用 `sanitizeHookText` / `sanitizeHookArgs` 截断并简单清洗后再放入 `Context`。

---

## 二、picoclaw 现状

- **无** `pkg/hooks`、`pkg/hookpolicy`，仅有一处与 “webhook” 相关的钉钉 session webhook，与 agent 生命周期无关。
- **Agent 结构**：使用 `AgentRegistry` + `AgentInstance`，多 agent 多 workspace；单条消息经 `processMessage` → `runAgentLoop` → `runLLMIteration`，由当前选中的 `agent`（含 `Workspace`）处理。
- **无 TurnID**：`processOptions` 中没有 `TurnID` 字段。
- **观察/审计**：原 `observe.Observer` 已移除；对话与 prompt 审计由 hooks + `prompt_audit` 实现（见 [hooks.md](hooks.md)）。

---

## 三、能否移植到 picoclaw？结论：**可以**

- 逻辑与依赖均可在 picoclaw 中实现：事件类型、Context、Dispatcher、审计、策略解析均为通用设计。
- 唯一需要适配的是 **多 agent/多 workspace**：策略与审计路径应按**当前 agent 的 workspace** 解析和写入；sciclaw 是单 workspace，picoclaw 需在派发时带上 workspace（见下）。

---

## 四、移植要点

### 4.1 需要新增的包/文件（从 sciclaw 拷贝并微调）

| 目标路径 | 说明 |
|----------|------|
| `pkg/hooks/types.go` | 可直接用；若希望支持“按 workspace 策略”可给 `Context` 增加 `Workspace string`（见下） |
| `pkg/hooks/dispatcher.go` | 可直接用 |
| `pkg/hooks/dispatcher_test.go` | 可直接用 |
| `pkg/hooks/audit_jsonl.go` | 可直接用 |
| `pkg/hooks/audit_jsonl_test.go` | 若有则一并拷贝 |
| `pkg/hooks/builtin/provenance.go` | 可直接用（仅依赖 `pkg/hooks`） |
| `pkg/hooks/builtin/policy.go` | 需小改：若 `Context.Workspace` 非空，则用其作为 `LoadPolicy(workspace)` 的 workspace，否则用 `PolicyHandler.workspace`（便于单 agent 场景或测试） |
| `pkg/hookpolicy/model.go` | 可直接用（依赖 `github.com/sipeed/picoclaw/pkg/hooks`） |
| `pkg/hookpolicy/load.go` | 可直接用 |
| `pkg/hookpolicy/yaml_parser.go` | 可直接用 |
| `pkg/hookpolicy/nl_parser.go` | 可直接用 |
| `pkg/hookpolicy/load_test.go` | 若有则一并拷贝 |

### 4.2 建议在 `hooks.Context` 中增加字段

```go
// 在 pkg/hooks/types.go 的 Context 中增加
Workspace string `json:"workspace,omitempty"`
```

- 在 picoclaw 每次 `dispatchHook` 时设 `data.Workspace = agent.Workspace`。
- 在 `builtin/policy.go` 的 `Handle` 中：优先使用 `data.Workspace`，为空再用 `h.workspace`。这样 sciclaw 行为不变，picoclaw 可按 agent 维度应用策略。

### 4.3 picoclaw 中需要改动的调用链

1. **`processOptions`**（`pkg/agent/loop.go`）  
   - 增加字段：`TurnID string`。  
   - 在 `runAgentLoop` 入口生成并赋值，例如：  
     `opts.TurnID = al.nextTurnID()`（需在 `AgentLoop` 上实现 `nextTurnID()`，类似 sciclaw 的 `turnCounter` + 时间戳）。

2. **`AgentLoop` 结构体**  
   - 增加：`hooks *hooks.Dispatcher`、`hookAuditPath string`（可选，用于状态/调试）。  
   - 增加：`turnCounter uint64`（用于 `nextTurnID`）。

3. **`NewAgentLoop`（或等价构造处）**  
   - 不按“单 workspace”创建 Dispatcher，而创建一个**全局** Dispatcher（无固定 workspace）。  
   - 审计路径与策略按“当前 agent”决定：  
     - 在第一次需要派发 hook 时，用**当前 agent 的 workspace** 调用 `hookpolicy.LoadPolicy(agent.Workspace)`；  
     - 若希望与 sciclaw 一致“启动时即按某策略注册”，可改为：对每个 `AgentInstance` 在创建时或首次使用时，为其 workspace 创建/复用 Dispatcher，或使用“动态 workspace”的 PolicyHandler（见上）。  
   - 推荐：**单 Dispatcher**，PolicyHandler 用 `Context.Workspace` 在 `Handle` 内按需 `LoadPolicy`；审计路径可设为 `workspace/hooks/hook-events.jsonl`，在创建 AuditSink 时若需要可按 workspace 分文件（或先简单用默认 agent 的 workspace 建一个 sink，再迭代成分文件）。

4. **注册内置 handler**  
   - 在创建 Dispatcher 后，对所有 `hooks.KnownEvents()` 注册 `ProvenanceHandler` 和 `PolicyHandler`。  
   - `PolicyHandler` 用 `NewPolicyHandler("")`，依赖 `Context.Workspace`。

5. **在 `runAgentLoop` / `runLLMIteration` 中插入 dispatch**  
   - 与 sciclaw 一一对应：  
     - 进入 `runAgentLoop` 后、构建 messages 前：`EventBeforeTurn`（带 TurnID、SessionKey、Channel、ChatID、Model、UserMessage、**Workspace**）。  
     - `runLLMIteration` 返回 err 时：`EventOnError`（phase: llm_iteration）。  
     - 一轮正常结束、写 session 并准备 return 前：`EventAfterTurn`。  
     - 每次调用 LLM 前：`EventBeforeLLM`。  
     - 每次 LLM 返回后：`EventAfterLLM`。  
     - 每个 tool 执行前：`EventBeforeTool`。  
     - 每个 tool 执行后：`EventAfterTool`；若 `toolResult.IsError` 再 `EventOnError`（phase: tool_execution）。  
   - 所有传入的 `Context` 中：`TurnID`、`SessionKey`、`Channel`、`ChatID`、`Model`、`UserMessage`、`Workspace` 等与 sciclaw 一致；对长文本/参数做与 sciclaw 相同的截断与脱敏（可抽成 `sanitizeHookText` / `sanitizeHookArgs`）。

6. **审计路径**  
   - 若一个进程多 agent：可按 workspace 分文件，例如 `filepath.Join(agent.Workspace, "hooks", "hook-events.jsonl")`；此时需要在派发时根据 `agent.Workspace` 选择或创建对应的 `AuditSink`（或 Dispatcher 支持按 workspace 的 sink 映射）。  
   - 简单实现：先使用“默认 agent 的 workspace”的单一审计文件，与 sciclaw 行为接近，后续再扩展为按 workspace 分文件。

### 4.4 依赖

- `pkg/hookpolicy` 仅依赖 `pkg/hooks` 和 `gopkg.in/yaml.v3`（picoclaw 已有）。  
- 无需新增第三方库。

### 4.5 测试

- 拷贝 `pkg/hooks/dispatcher_test.go`、`pkg/hooks/audit_jsonl_test.go`（若有）、`pkg/hookpolicy/load_test.go`（若有）。  
- sciclaw 的 `pkg/agent/loop_test.go` 中有基于 `hooks.yaml` 的测试，可摘取与 hook 相关的用例到 picoclaw 的 `loop_test.go`，保证 `processMessage` 在开启 hook 时仍通过。

---

## 五、小结

| 项目 | 结论 |
|------|------|
| 能否移植 | **可以**，无技术障碍 |
| 工作量 | 中等：拷贝 2 个包 + 在 loop 中加 TurnID、Dispatcher、约 7 处 dispatch 与脱敏 |
| 多 workspace 适配 | 在 `Context` 中增加 `Workspace`，PolicyHandler 与审计路径按当前 agent 的 workspace 处理 |
| 与原有 observe 的关系 | observe 已移除；调试/观测与对话分析由 hooks + prompt_audit 替代，见 [hooks.md](hooks.md) |

按上述步骤即可将 sciclaw 的 HOOK 功能完整移植到 picoclaw，并支持多 agent/多 workspace。

---

## 六、移植与替代说明

- **observe 已移除**：原 `pkg/agent/observe` 及 config 中的 `observation` 已删除；不再支持 `ObservationDir()` 与 observation 配置。
- **对话审计替代方案**：如需记录完整 prompt、用户消息与 LLM 回复用于分析或调试，请使用 **hooks + prompt_audit**：在 config 中启用 `hooks.prompt_audit.enabled`，可选 `include_full_prompt: true`，输出写入 `workspace/hooks/prompt-audit.jsonl`（或 `prompt_audit.path` 指定路径）。详见 [hooks.md](hooks.md)。
