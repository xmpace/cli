---
req_id: sheet-history-revert
mode: from-prd
tracks: [be]
created_at: 2026-06-23T13:09:08Z
---

> ⚠️ **请勿直接编辑此文档**
> 修改请通过 `/ccm-harness:draft-spec sheet-history-revert "<变更描述>"`（路由器会进入 update capability）
> 手改不会被 check-spec-drift 放过，且会破坏 Frozen 快照关联性
> 本文档由 /ccm-harness:draft-spec 生成于 2026-06-23T13:09:08Z；req-id：sheet-history-revert

---

# Sheet 历史版本查询与回滚

<!-- BEGIN generated overview — scripts/render_spec_overview.py 自动生成，勿手改；改 spec 请走 /ccm-harness:draft-spec <req-id> "<变更>"（update） -->
## 方案概览 / TL;DR

> 本段由机器从下方子任务字段**确定性生成**，供快速把握方案；**权威细节仍以各子任务 `yaml` 块为准**。

- **需求** `sheet-history-revert` ｜ 模式 `from-prd` ｜ 端 BE
- **规模** 5 BE ｜ **涉及 PSM** `tooling.lark_cli`, `tooling.sheet_skill_spec`, `sheet.facade.agg`, `bear.server.sheet_data` ｜ **Thrift 影响** 无 ×5

| 子任务 | 一句话 | 关键信息 |
|---|---|---|
| BE-1 | lark-cli `+history-list` 历史记录列表 shortcut | `tooling.lark_cli` · thrift:无 · `history_list[read]（callTool tools/invoke_read；内部对接 /space/api/v3/sheet/histories）` |
| BE-2 | lark-cli `+history-revert` 与 `+history-revert-status` 回滚流 shortcut | `tooling.lark_cli` · thrift:无 · `history_revert[write] / history_revert_status[read]（callTool tools/invoke_write\|read；内部对接 /space/api/v2/sheet/recover、/api/v2/sheet/recover/status）` |
| BE-3 | sheet-skill-spec 上游事实源（skill 正文 + shortcut/flag 定义） | `tooling.sheet_skill_spec` · thrift:无 · `A` |
| BE-4 | sheet-facade-agg 在现有 ToolsCall 上新增 3 个历史/回滚工具 | `sheet.facade.agg` · thrift:无 · `history_list[read] / history_revert[write] / history_revert_status[read]` |
| BE-5 | sheet/data 透传 scene 并在 RecoverMsg 上新增 Scene 字段 | `bear.server.sheet_data` · thrift:无 · `RecoverHistory / QueryRecoverStatus（复用）；MQ RecoverMsg 加 Scene 字段` |

> 完整字段见各子任务 `yaml` 块。
<!-- END generated overview -->

## 概述与范围

为 `lark-cli` 的 lark-sheets 能力补齐**电子表格历史版本查询与回滚**，新增 3 个 shortcut，让 AI / 用户可以列出某张表的历史版本、回滚到指定版本、并查询回滚的异步状态。三个 shortcut 封装的是飞书电子表格已上线的 space 接口（见下表的「前端接口参考」），本需求不新建产品能力，只是把它们包装成稳定、AI 友好的命令面。

| 功能 | shortcut | 前端接口参考（搜索 lark/idl） | 实现差异点 |
|---|---|---|---|
| 查历史记录列表 | `+history-list` | `/space/api/v3/sheet/histories` | ① 仅返回 `minor_histories` 列表；② `minor_histories` 的 `id` 字段重命名为 `history_version_id`；③ 每条仅保留 `history_version_id` / `create_time`（序列化成 AI 优化的可读格式）/ `action` / `all_block_revision` 四个字段 |
| 历史记录回滚 | `+history-revert` | `/space/api/v2/sheet/recover` | 传入 `+history-list` 拿到的 `history_version_id`，回滚到指定版本 |
| 查询回滚状态 | `+history-revert-status` | `/api/v2/sheet/recover/status` | 查询 `+history-revert` 发起的异步回滚的当前状态 |

**范围内**：
- `larksuite/cli` 仓新增 3 个 sheets shortcut（含 `+history-list` 的响应裁剪 / 字段重命名 / 时间格式转换逻辑）。
- `ee/sheet-skill-spec` 上游事实源补 skill 正文与 shortcut/flag 定义，经其工作流 `sync:cli` 同步到 `larksuite/cli` 的 `skills/lark-sheets/` 与 `shortcuts/sheets/data/`。
- `ee/sheet-facade-agg` **复用现有 `ToolsCall` 接口**，在其上新增 3 个工具（`history_list` / `history_revert` / `history_revert_status`）；并在**回滚消息消费侧**（消费 `sheet/data` 产出的 RecoverMsg）按 scene 给 `memberId` 赋值（doubao=10，lark-cli=11）后构造 recover cs。
- `sheet/data`（`bear.server.sheet_data`）把 scene 从入口透传到 `RecoverHistory`，并在产出的 `RecoverMsg`（MQ 消息）上**新增 `Scene` 字段**，供 agg 消费时区分场景。

**范围外**：
- 历史版本的底层存储 / 快照 / 过期清理逻辑、`RecoverHistory` 的回滚业务语义（`bear.server.sheet_data` 已有，本需求仅透传 scene + 加 MQ 字段，不改回滚逻辑本身）。
- 历史版本 diff、可视化、权限模型变更等产品侧扩展。
- `doubao-office` 消费方的同步（`sheet-skill-spec` 另有 `sync:doubao`，本需求不涉及）。

## 服务拓扑与 PSM 变更判定表

**调用链路（目标形态）**：

```
用户 / AI ──> lark-cli (+history-list / +history-revert / +history-revert-status)
                   │  callTool → POST /open-apis/sheet_ai/v2/.../tools/invoke_read|write（scene 随入口确定）
                   ▼
        sheet.facade.agg  ToolsCall（复用现有接口，新增 3 个工具:
                          history_list[read] / history_revert[write] / history_revert_status[read]）
        ├─ history_list      ──> 拉历史列表（裁剪 minor_histories / 4 字段 / AI 时间格式）
        ├─ history_revert ───scene(ctx 透传)──> bear.server.sheet_data: RecoverHistory
        │                                          │ service.RecoverHistory → SendRecoverMsg
        │                                          ▼
        │                                   MQ: RecoverMsg（新增 Scene 字段）
        │                                          ▼
        │   sheet.facade.agg  RecoverMsg 消费者 ── 读 Scene → memberId(doubao=10/lark-cli=11)
        │                                          → 构造 recover cs → 调既有 recover 下游
        └─ history_revert_status ──> bear.server.sheet_data: QueryRecoverStatus(transactionID)
```

`lark-cli` 现有 sheets shortcut 统一通过 `callTool`（`shortcuts/sheets/sheet_ai_api.go`）走 One-OpenAPI 的 `tools/invoke_read|write` 入口（`ToolKindRead` / `ToolKindWrite`），由 `sheet.facade.agg` 的 `OpenAPIToolCallRead/Write`（`biz/handler/lark_cli.go`，底层复用 `aiService.ToolsCall`）按 `tool_name` 分发。本需求的 3 个 shortcut 即按此路径新增 3 个工具，而非直连 space 接口或新增独立 OpenAPI 路由。

**回滚为异步两段式**：`history_revert` 工具调 `sheet/data` 的 `RecoverHistory`（`biz/history/service/recover.go`），后者通过 `SendRecoverMsg`（`infra/mq/producer/recover.go` 的 `RecoverMsg`）投递回滚消息并返回 `transactionID`；agg 侧的 RecoverMsg 消费者真正构造 recover cs，并在此时按 scene 给 `memberId` 赋值。`history_revert_status` 工具走 `sheet/data` 的 `QueryRecoverStatus(transactionID)` 查询异步结果。scene 从 ToolsCall 入口经 **ctx 透传**（沿用既有 `utils.WithSceneDoubao(ctx)` 范式）到 `RecoverHistory`，再写入 `RecoverMsg.Scene` 字段，使 agg 消费时可区分 doubao / lark-cli。

**PSM 变更判定表**：

| PSM | 需要代码变更？ | 变更内容 | 不变更原因 |
|---|---|---|---|
| `tooling.lark_cli`（`larksuite/cli`，无服务 PSM） | 是 | 新增 3 个 shortcut + `+history-list` 响应 transform | — |
| `tooling.sheet_skill_spec`（`ee/sheet-skill-spec`，无服务 PSM） | 是 | 新增 lark-sheets skill 正文 + 3 个 shortcut/flag 定义，生成后同步到 cli | — |
| `sheet.facade.agg`（`ee/sheet-facade-agg`） | 是 | ① 现有 `ToolsCall` 新增 3 个工具（`history_list[read]` / `history_revert[write]` / `history_revert_status[read]`）；② `history_revert` 工具调 `sheet/data` 时透传 scene（ctx）；③ **RecoverMsg 消费者**读 `Scene` 字段，按 scene 给 `memberId` 赋值后构造 recover cs。**无新 thrift**（工具按 `tool_name`+JSON 注册，scene 走 ctx baggage） | — |
| `bear.server.sheet_data`（`sheet/data`） | 是 | scene 从入口透传到 `RecoverHistory`（`biz/history/service/recover.go`），并在 `RecoverMsg`（`infra/mq/producer/recover.go`）上**新增 `Scene` 字段**随消息投递。`RecoverMsg` 是 JSON Go struct，加字段**非 thrift**；scene 透传走 ctx baggage | 回滚 / 快照业务语义不变（`RestoreHistorySnapshot` / `QueryRecoverStatus` 等复用） |

> **`memberId` 按 scene 赋值（实现硬约束，跨 sheet/data + agg）**：scene 区分 doubao 与 lark-cli（沿用既有 `utils.WithSceneDoubao(ctx)` 范式）。本需求要求 scene 从 ToolsCall 入口一路透传：
> 1. agg `history_revert` 工具调用 `sheet/data.RecoverHistory` 时，把 scene 经 ctx 透传；
> 2. `sheet/data` 在产出的 `RecoverMsg` 上写入 `Scene` 字段，随 MQ 投递；
> 3. **agg 的 RecoverMsg 消费者**在真正构造 recover cs 时，读 `Scene` 给 `memberId` 赋值——doubao 场景 = `10`，其他（lark-cli）场景 = `11`。
>
> memberId 赋值发生在 **agg 消费侧**（不是同步 ToolsCall 调用栈，因为回滚是异步消息驱动）。错误的 memberId 会导致回滚归属错误的调用方身份（审计 / 权限相关）。`RecoverMsg.MemberId` 字段已存在，但本需求要求按 scene 正确赋值并据此区分两个消费方。

## 后端 / Tooling 子任务

> 说明：`lark-cli` 与 `sheet-skill-spec` 均属 Tooling，按 ccm-harness 约定建 BE-*，`thrift_impact: 无`。本需求无前端（FE）子任务。

### BE-1: lark-cli `+history-list` 历史记录列表 shortcut

```yaml
psm: tooling.lark_cli
repo: larksuite/cli
module: shortcuts/sheets
be_deploy_required: false
thrift_impact: 无
api: facade-agg ToolsCall::history_list[read]（callTool tools/invoke_read；内部对接 /space/api/v3/sheet/histories）
depends_on: [BE-4]
estimate: 1.5d
```

**调用的下游服务**：经 `callTool(ToolKindRead, "history_list", ...)`（`shortcuts/sheets/sheet_ai_api.go`）走 `tools/invoke_read` 入口，由 facade-agg 的 `history_list` 工具内部对接 `/space/api/v3/sheet/histories`。入参：表格 token（沿用现有 sheets shortcut 的 `--spreadsheet-token` / `--token` 解析）。响应裁剪 / 字段重命名 / 时间格式可在 facade-agg 工具侧或 lark-cli 侧完成（见实现要点；以 AI 友好输出为准）。
**实现要点（实现差异点落地）**：
- 仅取响应中的 `minor_histories` 列表，丢弃其余顶层字段（如 major histories）。
- 将每条 `minor_histories` 的 `id` 字段重命名输出为 `history_version_id`。
- 每条仅保留 4 个字段：`history_version_id`、`create_time`、`action`、`all_block_revision`。
- `create_time` 序列化成 AI 优化的可读格式（如本地时区可读时间串），而非裸 unix 时间戳。
**验收场景**：
- Given 一张有多个历史版本的电子表格，When 执行 `lark-cli sheets +history-list --token <t>`，Then 返回 JSON 数组，每条恰好含 `history_version_id` / `create_time` / `action` / `all_block_revision` 四个键，且 `create_time` 为可读格式。
- Given 一张无历史记录的表格，When 执行 `+history-list`，Then 返回空列表且退码 0（不报错）。

### BE-2: lark-cli `+history-revert` 与 `+history-revert-status` 回滚流 shortcut

```yaml
psm: tooling.lark_cli
repo: larksuite/cli
module: shortcuts/sheets
be_deploy_required: false
thrift_impact: 无
api: facade-agg ToolsCall::history_revert[write] / history_revert_status[read]（callTool tools/invoke_write|read；内部对接 /space/api/v2/sheet/recover、/api/v2/sheet/recover/status）
depends_on: [BE-1, BE-4, BE-5]
estimate: 1.5d
```

**调用的下游服务**：
- `+history-revert` → `callTool(ToolKindWrite, "history_revert", ...)`，agg 工具调 `sheet/data.RecoverHistory`（异步），返回 `transactionID`。
- `+history-revert-status` → `callTool(ToolKindRead, "history_revert_status", ...)`，agg 工具调 `sheet/data.QueryRecoverStatus(transactionID)` 查异步结果。
- 注：`memberId` 按 scene 赋值（doubao=10 / lark-cli=11）发生在 agg 的 **RecoverMsg 消费者**侧（见 BE-4 / BE-5），lark-cli 侧不感知；scene 由 callTool 入口（read/write）确定。
**实现要点**：
- `+history-revert` 的 `--history-version-id`（命名对齐 BE-1 的输出字段）为必填；缺失时在 Validate 阶段给出可执行错误提示。
- 回滚为异步操作，`+history-revert` 返回受理结果，`+history-revert-status` 供轮询最终状态（成功 / 进行中 / 失败）。
**验收场景**：
- Given 由 `+history-list` 取得的合法 `history_version_id`，When 执行 `+history-revert --token <t> --history-version-id <id>`，Then 后端受理回滚并返回可被 `+history-revert-status` 查询的标识。
- Given 一次已发起的回滚，When 轮询 `+history-revert-status`，Then 能区分「进行中 / 成功 / 失败」三种状态。
- Given 缺省 `--history-version-id`，When 执行 `+history-revert`，Then 返回明确的参数缺失错误，不发起请求。

### BE-3: sheet-skill-spec 上游事实源（skill 正文 + shortcut/flag 定义）

```yaml
psm: tooling.sheet_skill_spec
repo: ee/sheet-skill-spec
module: canonical-spec
be_deploy_required: false
thrift_impact: 无
api: N/A
depends_on: [BE-1, BE-2]
estimate: 1d
```

**调用的下游服务**：无（构建期工作流）。
**实现要点（按 `sheet-skill-spec` README 工作流）**：
- 在飞书 base 表登记 3 个新 shortcut 的 tool ↔ shortcut 映射与 flag 定义，`npm run sync:tool-shortcut-map` 镜像入仓。
- 在 `canonical-spec/references/<相关 skill>/cli-reference.md` 补三个 shortcut 的描述 / 示例 / Validate-DryRun-Execute 约束。
- 跑 `npm run generate:all && npm run check:all` 验证，产出 `generated/lark-cli/skills/lark-sheets/` 与 `generated/lark-cli/data/{flag-defs.json,flag-schemas.json}`。
- 跑 `npm run sync:cli` 把 generated 同步到 `larksuite/cli` 的 `skills/lark-sheets/`（mirror）与 `shortcuts/sheets/data/`（mirror），在 cli 仓作为 PR 提交。
**边界**：skill 命名 / 切分 / 正文 / flag 定义一律先落 `sheet-skill-spec`，禁止直接改 cli 仓的 `generated`/`skills/lark-sheets/` 产物（README「对齐原则」）。
**验收场景**：
- Given 在 `sheet-skill-spec` 完成上述编辑，When 跑 `npm run check:all`，Then 全部门禁通过（generated 与 canonical 一致、map 与 base 表一致）。
- Given 跑 `npm run sync:cli`，Then cli 仓 `skills/lark-sheets/` 与 `shortcuts/sheets/data/` 出现对应 3 个 shortcut 的 skill 正文与 flag 定义。

### BE-4: sheet-facade-agg 在现有 ToolsCall 上新增 3 个历史/回滚工具

```yaml
psm: sheet.facade.agg
repo: ee/sheet-facade-agg
module: biz/handler
be_deploy_required: true
thrift_impact: 无
api: ToolsCall::history_list[read] / history_revert[write] / history_revert_status[read]
depends_on: []
estimate: 1.5d
```

**调用的下游服务**：`sheet/data` 的 `RecoverHistory` / `QueryRecoverStatus`（见 BE-5）+ 既有历史查询；agg 的 RecoverMsg 消费者复用既有 recover 下游（`biz/service/spreadsheet.go::ProcessRecoverCs`、`model.RecoverParam`），不新增 thrift。
**实现要点**：
- **ToolsCall 扩展**：在现有 `ToolsCall` 框架（`biz/handler/handler.go::ToolsCall` / `biz/handler/lark_cli.go::OpenAPIToolCallRead|Write`）注册 3 个新工具：`history_list`（read）、`history_revert`（write）、`history_revert_status`（read），按 `constants.IsReadTool` / `IsWriteTool` 归类，从 `tools/invoke_read` / `invoke_write` 入口可达。
- **scene 透传**：`history_revert` / `history_revert_status` 工具调 `sheet/data` 时，把 scene 经 ctx（沿用 `utils.WithSceneDoubao` 范式）透传下去，使 `sheet/data` 能写入 `RecoverMsg.Scene`。
- **RecoverMsg 消费者按 scene 赋 memberId（硬约束）**：agg 消费 `sheet/data` 投递的 `RecoverMsg`、构造真正 recover cs 时，读 `RecoverMsg.Scene` 给 `memberId` 赋值——doubao = `10`，lark-cli = `11`。这是异步消费侧逻辑，不在同步 ToolsCall 调用栈。
- `history_list` 工具对接历史列表查询；响应裁剪（仅 `minor_histories`、`id`→`history_version_id`、4 字段、`create_time` AI 友好格式）建议落在此工具侧（两个消费方共享，避免 lark-cli / doubao 双实现漂移）。
**边界**：只在 ToolsCall 上加工具 + 改 RecoverMsg 消费者；不新增独立 OpenAPI 路由、不改 `ai.ToolsCallRequest` thrift 契约、不改 `RecoverHistory` 回滚业务语义。
**验收场景**：
- Given lark-cli 经 `tools/invoke_read` 调用 `history_list`，Then 返回裁剪后的 `minor_histories`（4 字段，`history_version_id` 命名）。
- Given lark-cli（scene=lark-cli）经 `history_revert` 发起回滚，When agg 消费对应 RecoverMsg，Then 构造的 recover cs 中 `memberId == 11`；doubao 场景下同一路径 `memberId == 10`。
- Given 已发起回滚，When 调用 `history_revert_status`，Then 经 `QueryRecoverStatus` 返回可区分的回滚状态。

### BE-5: sheet/data 透传 scene 并在 RecoverMsg 上新增 Scene 字段

```yaml
psm: bear.server.sheet_data
repo: sheet/data
module: biz/history
be_deploy_required: true
thrift_impact: 无
api: bear.server.sheet_data::RecoverHistory / QueryRecoverStatus（复用）；MQ RecoverMsg 加 Scene 字段
depends_on: []
estimate: 1d
```

**调用的下游服务**：复用既有回滚链路（`biz/history/service/recover.go::RecoverHistory` → `infra/mq/producer/recover.go::SendRecoverMsg`）。
**实现要点**：
- **scene 透传**：把 agg 经 ctx 传入的 scene 接住，贯穿 `RecoverHistory`（`biz/history/service/recover.go`）到 `RecoverMsg` 构造处。
- **RecoverMsg 加 `Scene` 字段**：在 `infra/mq/producer/recover.go` 的 `RecoverMsg` struct 上新增 `Scene` 字段并在投递时赋值。`RecoverMsg` 是 JSON Go struct（`recoverProducer.NewMessage`），**加字段非 thrift**——`thrift_impact: 无`。
- `QueryRecoverStatus` 与回滚业务语义保持不变，仅承载 scene 透传。
**边界**：不改回滚 / 历史快照业务逻辑；只加 scene 透传与 `RecoverMsg.Scene` 字段。
**scene 透传方式（已定）**：经 **ctx baggage**（`metainfo` / 沿用既有 `utils.WithSceneDoubao(ctx)` 范式）从 agg 透传到 `RecoverHistory`，**不**在 `RecoverHistoryReq` thrift 上加字段 → 零 IDL 变更，`thrift_impact: 无`。
**验收场景**：
- Given agg 以 scene=lark-cli 调 `RecoverHistory`，Then 投递的 `RecoverMsg.Scene` 标识 lark-cli；doubao 同理。
- Given 回滚已发起，When `QueryRecoverStatus(transactionID)`，Then 返回回滚状态（语义与现状一致）。
- Given lark-cli（scene=lark-cli）经 `tools/invoke_write` 调用 `history_revert`，Then 构造的 recover cs 中 `memberId == 11`；doubao 场景下同一工具 `memberId == 10`。
- Given 已发起回滚，When 调用 `history_revert_status`，Then 返回可区分的回滚状态。

## API 契约引用

本需求三个接口均为飞书电子表格已上线 space 接口，契约以各仓库最新 master 为准；对应 thrift 定义按 PRD 提示在 `lark/idl` 中搜索确认（实现阶段补全精确路径）：

- 查列表：`/space/api/v3/sheet/histories`（取 `minor_histories`）
- 回滚：`/space/api/v2/sheet/recover`
- 回滚状态：`/api/v2/sheet/recover/status`

> 契约本体不进本 spec 正文；精确 `lark/idl/...thrift::Service::Method` 路径在实现阶段确认并回填到对应 BE-* 的 `api` 字段说明。

## 验收场景（汇总）

- 列表：`+history-list` 仅返回 `minor_histories`，每条恰好 4 个字段，`id` 重命名为 `history_version_id`，`create_time` 为 AI 优化可读格式。
- 回滚：`+history-revert` → agg `history_revert` → `sheet/data.RecoverHistory`（异步），受理后返回可查询标识。
- memberId/scene：agg 消费 `RecoverMsg` 构造 recover cs 时，按 `RecoverMsg.Scene` 赋 `memberId`——lark-cli=11、doubao=10（facade-agg 侧单测断言）。
- 状态：`+history-revert-status` → `QueryRecoverStatus` 能查询并区分回滚的进行中 / 成功 / 失败。
- skill 同步：`sheet-skill-spec` 生成产物经 `sync:cli` 落地到 cli 仓，`check:all` 全绿。
- 三个 shortcut 在 cli 中遵循统一的 Validate / DryRun / Execute 三段约定与现有 sheets shortcut 一致。

## 非功能要求与约束

- **复用既有模式**：3 个 shortcut 必须沿用 `shortcuts/sheets` 现有的 token 解析（`--spreadsheet-token` / `--token` 别名）、错误封装（`errs`）、`callTool`（`tools/invoke_read|write`）调用与 DryRun 渲染范式，不另起调用框架。facade-agg 侧必须复用现有 `ToolsCall` 接口扩展工具，不新增独立 OpenAPI 路由。
- **AI 友好输出**：`+history-list` 的字段裁剪与 `create_time` 可读格式是硬约束（PRD「实现差异点」），目的是降低 AI 消费成本。
- **工作流约束**：skill 内容与 flag 定义的唯一事实源是 `ee/sheet-skill-spec`；cli 仓的 `skills/lark-sheets/` 与 `shortcuts/sheets/data/` 为同步产物，不手改。
- **回滚为异步**：`+history-revert` 与 `+history-revert-status` 分离，调用方需理解「发起 → 轮询」两步语义。
- **事实基准**：所有外部仓库事实（space 接口、facade-agg 路由、sheet_data 能力）以各仓库最新 master 为准。

## 安全设计

- security_knowledge_ref: UNCONFIGURED
- 风险判断依据: 未配置安全知识库，待安全侧补齐。需关注点（供安全侧复核）：`+history-revert` 是**写 / 不可逆**操作（覆盖当前表格内容到历史版本），必须校验操作者对目标表格具备编辑 / 回滚权限；历史版本列表可能暴露协作者操作痕迹（`action` 字段），需确认读权限边界。
- 身份归属风险（memberId/scene）: `memberId` 须按 scene 正确赋值（lark-cli=11 / doubao=10）。错配会使回滚操作归属错误的调用方身份，影响审计与权限判定——属安全/审计相关，须在 agg RecoverMsg 消费侧保证赋值正确。
- 需要安全侧补充: 回滚操作的权限校验口径、历史 `action` 字段的可见性范围是否需脱敏、memberId 与真实操作者身份的映射是否需对齐审计要求。

## Codegen Delivery Plan

applicable: true

### A. Branch Plan

| `key` | value |
|---|---|
| `psm` | `sheet.facade.agg` |
| `business_branch` | `feat/sheet-history-revert` |
| `generated_branch` | `N/A` |
| `idl_branch` | `N/A` |
| `kitex_branch` | `N/A` |

### B. Delivery Targets

| repo | required | branch | artifact_paths | reason |
|---|---|---|---|---|
| larksuite/cli | yes | feat/sheet-history-revert | shortcuts/sheets/ , skills/lark-sheets/ | 3 个 shortcut 实现 + 同步落地的 skill 正文与 flag 数据 |
| ee/sheet-skill-spec | yes | feat/sheet-history-revert | canonical-spec/references/ , generated/lark-cli/ | skill / flag 上游事实源，生成后 sync 到 cli |
| ee/sheet-facade-agg | yes | feat/sheet-history-revert | biz/handler/ | 现有 ToolsCall 新增 3 个工具 + scene 透传 + RecoverMsg 消费者按 scene 赋 memberId |
| sheet/data | yes | feat/sheet-history-revert | biz/history/ , infra/mq/producer/ | RecoverHistory 透传 scene + RecoverMsg 新增 Scene 字段（JSON，非 thrift） |

### C. Generation Decision

| `key` | value |
|---|---|
| `needs_kitex_gen` | no |
| `needs_apacana` | no |
| `needs_kite_via_sdp` | no |
| `decision_basis` | facade-agg 复用现有 ToolsCall 框架按 tool_name 注册 3 个工具（JSON input，不动 ai.ToolsCallRequest thrift）；sheet/data 仅在 RecoverMsg（JSON Go struct）加 Scene 字段 + 经 ctx baggage 透传 scene，复用既有 RecoverHistory/QueryRecoverStatus，均非 thrift；lark-cli 经现有 callTool 包装。scene 已定走 ctx baggage（不加 RecoverHistoryReq thrift 字段），无新增/修改 kitex/apacana/SDP 契约 |

### D. Branch Naming Rule

业务分支统一用 `feat/sheet-history-revert`；本需求无 codegen，故 `generated_branch` / `idl_branch` / `kitex_branch` 均为 `N/A`。

## thrift 变更需求清单

无（按推荐实现路径）。三个接口的能力已存在；本需求的新增内容是：facade-agg 在 ToolsCall 上注册 3 个工具（`tool_name`+JSON，不动 `ai.ToolsCallRequest`）、sheet/data 在 `RecoverMsg`（JSON Go struct）上加 `Scene` 字段、scene 经 **ctx baggage** 透传——均不涉及 thrift。

**scene 透传方式：已定为 ctx baggage**（`metainfo` / 沿用既有 `utils.WithSceneDoubao(ctx)` 范式），明确**不**在 `bear.server.sheet_data` 的 `RecoverHistoryReq` thrift 上加字段。故本需求确无任何 thrift struct / RPC method / enum 的新增或修改，Generation Decision 三路保持 `no`、无 codegen。

## N. AI Capability Manifest

applicable: false

本需求为确定性 CLI 命令封装，不含 LLM / prompt 驱动的 AI 能力。`+history-list` 中「`create_time` 序列化成 AI 优化格式」仅指对机器/AI 更易读的时间字符串格式化，属确定性数据转换，非 AI capability。
