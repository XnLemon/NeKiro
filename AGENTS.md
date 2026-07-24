# Agent Operating Platform 项目约定

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
at specs/026-trusted-publication-acceptance/plan.md
<!-- SPECKIT END -->

本文件是整个仓库的长期项目宪章，适用于所有目录、模块和参与者。它记录稳定的产品目标、领域语言、架构边界和交付标准，不替代具体需求、API 文档或 ADR。

本文件应根据项目当前状态、已验证需求和正式架构决策持续自我更新迭代，但任何更新都必须说明原因与影响，并保持核心边界和兼容性变化清晰可追溯。

当实现细节与本文件冲突时，先判断是否发生了架构演进。如果确需改变本文件中的核心原则，必须先形成明确的架构决策并说明迁移影响，不得通过局部代码绕过边界。

## 1. 产品定位

本项目的目标不是先做一个展示 Agent 的商店，而是建设一个 **Agent Operating Platform**。

Marketplace 是未来建立在平台能力之上的一个模块。平台的长期价值来自统一的 Agent 描述、注册发现、运行通信、治理观测和生态协议，而不是 Marketplace 页面本身。

### 与 Agent Runtime Framework 的边界

NeKiro 管理 Agent 的外部生命周期和组织级信任边界：注册、版本化发布、发现、Workspace 授权安装、受控调用和跨 Agent 审计。Agent 内部如何完成模型调用、Prompt、Tool、Planner、Workflow、Memory、RAG、Session 和评测，属于 Agent Runtime。

`trpc-agent-go`、ADK 或其他 Agent Framework 可以作为参考 Runtime、示例 Agent 或协议适配对象，但不得成为 Control Plane / Data Plane 的产品定义或完整运行时依赖。核心平台能力必须在 Agent 更换语言、模型或 Framework 后仍然成立；只服务于单一 Runtime 的行为应留在适配器或该 Runtime 内部。

`sdks/agent-sdk` 必须保持轻量，只负责 Agent Card 一致性、平台上下文传播和通过 A2A Router 发起嵌套调用，不得扩展为模型、工具、工作流、记忆或通用 Agent 执行框架。

第一阶段必须通过至少两个采用不同 Runtime 实现的 Sample Agents 证明跨 Runtime 价值。它们之间的托管嵌套调用必须经过 Router，并在同一 Ledger lineage 中可查询。详细方向和决策分别见 `docs/architecture/platform-direction.md` 与 `docs/decisions/0003-runtime-agnostic-platform-boundary.md`。

第一阶段只证明一条完整、可追踪的核心闭环：

```text
Register -> Discover -> Install -> Invoke -> Record
```

也就是：

1. 开发者通过 Agent Card 注册一个 Agent 版本。
2. 用户按照 capability 发现合适的 Agent。
3. Workspace 安装并授权使用某个 Agent 版本。
4. 所有平台托管调用通过 A2A Router 到达目标 Agent。
5. 每次调用都产生可关联、可查询的 Invocation Ledger 记录。

任何第一阶段功能都应直接服务于这条闭环。不能证明闭环价值的能力，应推迟或通过 ADR 说明其必要性。

## 2. 统一领域语言

项目中的代码、API、数据库、文档和界面应统一使用以下术语，避免为同一概念创造不同名称。

### Agent

Agent 是能够通过受支持协议接收任务并返回结果的运行实体。Registry 不存储 Agent 进程，只存储描述 Agent 的元数据。

### Agent Card

Agent Card 是 Agent 的版本化、声明式元数据契约，也是平台各模块交流 Agent 信息的共同语言。

Agent Card 至少描述：

- 身份、名称、所有者和版本
- capabilities / skills
- 输入和输出 Schema
- A2A 协议版本与 endpoint
- 认证类型，不包含认证密钥
- 权限声明和调用限制

Agent Card 不应包含源码、密钥、实时健康状态、实时延迟、成功率或调用统计。动态状态属于 Registry 运行状态或 Observability 数据。

### Registry

Registry 是 Agent Card、Agent 版本和发布状态的事实来源。它负责注册、校验、版本管理、发布、停用和解析，不负责执行或部署 Agent。

### Discovery

Discovery 是从 Registry 派生的查询模型，负责按照 capability、名称、所有者和版本等条件发现 Agent。它不是 Agent Card 的第二事实来源。

### Catalog

Catalog 是由 Registry、Agent Card Store 和 Discovery 组成的控制面领域。第一阶段三者保持清晰的代码边界，但作为同一个部署单元运行。

### Workspace Installation

Installation 表示某个 Workspace 已接受权限并获准使用某个 Agent 版本。它不是收藏、源码复制或 Agent 部署。

一条 Installation 至少应表达：

```text
workspace_id
agent_id
version_constraint
installed_version
accepted_permissions
enabled
installed_at
```

### Invocation

Invocation 是一次具体的 Agent 调用。嵌套 Agent 调用会产生新的 Invocation，并通过 `parent_invocation_id` 与父调用关联。

### Task

Task 表示一次 A2A 任务生命周期。一个根 Task 可以包含多个具有父子关系的 Invocation。

### A2A Router

A2A Router 是数据面的 Agent 网络入口，负责端点解析、A2A 调用、流式传输、Task/Session/Trace 上下文传播，以及治理扩展点。它不拥有 Agent Card，也不承担 Registry 职责。

### Invocation Ledger

Invocation Ledger 是追加式调用事实记录，用于追踪、审计、故障分析和未来计费。Ledger 记录事实，不决定路由或业务策略。

### Control Plane 和 Data Plane

- Control Plane 管理 Agent：Gateway、Catalog、Workspace、Invocation Dispatch。
- Data Plane 调用 Agent：A2A Router、Task Context、Transport、Policy Hooks、Ledger。

## 3. 核心架构边界

以下规则贯穿整个项目：

1. Frontend 只能访问 Gateway，不得直接访问 Registry、数据库或 Agent。
2. Gateway 是唯一北向入口，负责认证上下文、请求校验、响应规范化和调用标识生成。
3. Gateway 不拥有 Agent Card，不得绕过 Catalog 直接读写其存储。
4. Gateway 不直接调用 Agent，所有平台托管调用必须交给 A2A Router。
5. Registry 是 Agent Card 的唯一事实来源，Discovery 只能构建派生索引或查询模型。
6. A2A Router 必须通过受控接口解析 Agent Card 和 endpoint，不维护独立的永久 Card 副本。
7. 平台托管的 Agent-to-Agent 调用必须再次经过 A2A Router，并传播父子调用关系。
8. Agent 不得直接访问平台数据库，也不得信任来自调用方的 Workspace、权限或审计字段。
9. 模块只能写入自己拥有的数据。即使第一阶段共用一个数据库，也不得跨模块直接修改表。
10. 所有跨边界通信必须使用 `contracts/` 中的版本化契约，不得共享内部实现类型。
11. Agent Card、日志、Ledger 和错误响应中不得出现 API Key、Token 或其他密钥。
12. 逻辑模块边界不等于微服务边界。第一阶段优先保持简单部署，只有在扩缩容、故障隔离或团队所有权需要时才拆分服务。

第一阶段无法通过代码完全阻止外部 Agent 私下直连。平台必须保证官方 SDK、示例 Agent 和托管网络路径统一经过 Router；更强的网络强制策略属于后续治理阶段。

## 4. 第一阶段目标结构

```text
agent-platform/
├─ apps/
│  ├─ console/                       # Web 控制台
│  ├─ control-plane/                 # Gateway + Catalog + Workspace + Dispatch
│  └─ a2a-router/                    # 独立数据面进程
├─ contracts/
│  ├─ agent-card/                    # Agent Card Schema
│  ├─ northbound-api/                # Console <-> Gateway
│  ├─ internal-api/                  # Control Plane <-> Router
│  ├─ a2a-profile/                   # 平台采用的 A2A 约束
│  └─ events/                        # Invocation 和审计事件
├─ sdks/
│  ├─ agent-sdk/                     # Agent 接入和 Router 调用
│  └─ client-sdk/                    # 外部应用调用平台
├─ agents/                           # 示例和验收 Agent
├─ tests/
│  ├─ contract/
│  ├─ integration/
│  └─ e2e/
├─ deploy/
└─ docs/
   ├─ architecture/
   ├─ contracts/
   └─ decisions/
```

### 技术栈决策

本项目采用 **React 前端 + Go 后端 / Router** 的混合技术栈，这是项目级硬约束：

- `apps/console` 使用 React、Vite、TypeScript 和 TailwindCSS，并优先复用统一的 shadcn/ui 组件体系。
- `apps/control-plane` 使用 Go 实现 Gateway、Catalog、Workspace 和 Invocation Dispatch。
- `apps/a2a-router` 使用 Go 独立实现 A2A 路由、流式传输、Task Context、Policy Hooks 和 Ledger。
- `sdks/agent-sdk` 第一阶段优先提供 Go SDK；TypeScript 等其他语言 SDK 在核心协议稳定后增量提供。
- `contracts/` 必须以语言无关的 JSON Schema、OpenAPI 和明确的 A2A Profile 作为事实来源，再生成或映射 Go 与 TypeScript 类型。
- PostgreSQL 是第一阶段的持久化数据库，逻辑模块即使共用数据库实例也必须保持数据所有权边界。
- Node.js 只用于前端构建、契约生成和必要的工程工具，不得用于实现 Control Plane、A2A Router 或其他后端核心服务。

当前仓库已将跨边界契约事实来源迁移为语言无关的 JSON Schema、OpenAPI 和 A2A Profile。当前活动契约集为 Agent Card `0.2`、Trusted Publication `v1`、Workspace `1`、Installation `2`、Catalog/Northbound API `v3`、Control Plane Internal API `v2`（精确 Card 解析）与 `v3`（嵌套调用 installed-version 解析）、Northbound Invocation API `v4`、Router Internal dispatch API `v4`（Workspace/Invocation/Trace metadata reads 保持 `v3`）、Router Agent API `v1`、Router Invocation Credential `v1`、Invocation Event `0.3`、Platform Error `v4`（运行时）与 `v3`（Workspace/Installation/内部解析）、Invocation Result `v1` / Result Stream Event `v2`，以及 A2A Profile Schema `0.2` / protocol `0.3.0`；历史 dispatch v3 工件仅作为迁移证据保留，不提供运行时双读 fallback。

当前仓库已完成 Spec 002 的 Catalog、Spec 003/004/005/006/007/008/009 的 Workspace 与 Installation runtime，并完成 Spec 012 Control Plane Invocation Dispatch、Spec 013 A2A Router Foundation、Spec 014 Router-owned Invocation Ledger、Spec 015 Runtime B direct A2A sample、Spec 016 non-streaming dispatch、Spec 017 streaming A2A delivery、Spec 018 Invocation/Trace metadata reads、Spec 019 Agent SDK nested invocation、Spec 020 cross-runtime caller、Spec 021 Invoke-to-Record backend acceptance、Spec 024 Router-to-Agent authentication，以及已通过 PR #56 / CI run `30068997568` 合并的 Spec 025 Workspace Client SDK，以及已通过 PR #57 / CI run `30074754169` 合并的 Spec 026 Trusted Publication Acceptance and Operations。已落地的运行时边界包括 Workspace owner policy、精确 Release 解析、Gateway v4 调用、Router-mediated JSON/SSE 调用、metadata-only Ledger、Workspace-scoped Invocation/Trace 查询、Runtime A -> Router -> Runtime B 嵌套调用、每请求 Ed25519 Router 凭证、Agent 侧严格验签与进程内一次性 `jti`，以及绑定单个 Gateway/Workspace/Owner credential 的应用 Client SDK。Spec 026 已在一个空 Compose/PostgreSQL 环境中验证 Register -> Verify -> Publish -> Discover -> Install -> Invoke -> Record、跨 Runtime Release provenance、完整 publication/lifecycle/credential/direct/unavailable 负向矩阵、secrecy scan，以及 caller cancellation 与 stream/terminal Ledger commit 竞态下的单一终态；PR #57 的七项 CI 检查全部通过并以 commit `785f9cf` 合并，Issue #52 和父 Issue #47 均已关闭。Frontend Console 开发保持暂停且 `apps/console` 尚未存在。后续密钥轮换、跨副本 replay、验证证据 retention、suspension approval、自动 reconciliation、生产治理和完整部署集成仍需独立 Spec/ADR，不得实现为 retry、alternate endpoint、credential fallback 或旧协议兼容路径。本次状态更新原因是 Spec 026 已通过合并证据正式完成 Issue #52 的 trusted-publication 闭环和 Issue #47 的父级验收，影响是 provider、Workspace owner 和 operator 可通过现有 Gateway/Router/Ledger 边界发布、授权、调用、追踪和恢复，但不得绕过受管路径或原地恢复不可变 Release。

Go 的 HTTP Router、数据库访问、A2A SDK 和代码生成工具应通过独立 ADR 选择。可以评估 `trpc-agent-go` 及成熟 A2A Go 实现，但完整 Agent Framework 不得成为 Control Plane 或 A2A Router 的核心依赖。外部 Framework 只能作为 Agent Runtime、示例实现或隔离的协议适配器，不能取代本项目的 Control Plane / Data Plane 架构边界。

`control-plane` 第一阶段是一个部署进程，内部按以下领域隔离：

```text
gateway/
catalog/
  registry/
  discovery/
  card-store/
workspace/
  installations/
invocation/
  dispatch/
```

`a2a-router` 从第一天起独立部署，内部至少包含：

```text
routing/
task-context/
transport/
policy-hooks/
ledger/
```

## 5. 依赖方向

允许的主要依赖方向如下：

```text
Console -> Northbound API Contract -> Control Plane
Control Plane -> Internal API Contract -> A2A Router
A2A Router -> A2A Profile -> Agent
Agent -> Agent SDK -> A2A Router
Registry -> Discovery Projection
Router -> Invocation Events -> Ledger
```

具体要求：

- `contracts/` 只能包含 Schema、DTO、协议约束和兼容性工具，不包含业务流程。
- `sdks/` 可以依赖 `contracts/`，不得依赖平台服务的内部实现。
- `a2a-router` 不得导入 `control-plane` 的内部模块。
- `console` 不得复制 Agent Card、权限或状态机规则作为第二套业务逻辑。
- 通用工具包必须保持领域无关，禁止形成无边界的 `common` 或 `utils` 垃圾场。
- 禁止循环依赖。新增共享抽象前，先确认它是真正稳定的契约，而不是为了绕开模块边界。

外部 Agent Framework 可以作为 Agent SDK 或 Runtime 的实现参考，但不得用框架内部的包结构替代平台的 Control Plane / Data Plane 边界。

## 6. 核心调用流程

### 注册与发现

```text
Developer
  -> Console
  -> Gateway
  -> Registry validates Agent Card
  -> Card Store persists version
  -> Discovery updates query projection
```

注册成功必须保证 Agent Card 已通过 Schema 和版本规则校验。Discovery 更新失败时不得悄悄返回完整成功，必须保留可重试和可观测状态。

### 安装

```text
User
  -> Gateway
  -> Workspace resolves Agent version from Registry
  -> User accepts declared permissions
  -> Installation is persisted
```

调用前必须校验 Installation 是否存在、是否启用、版本是否可用，以及 capability 是否在已接受权限范围内。

### 调用

```text
User
  -> Gateway creates invocation_id / trace_id
  -> Invocation Dispatch validates Workspace installation
  -> A2A Router resolves the exact Agent Card version
  -> Router invokes Agent through A2A
  -> Router streams result back
  -> Ledger records lifecycle events
```

### Agent-to-Agent 调用

```text
Agent A
  -> Agent SDK
  -> A2A Router
  -> resolve Agent B
  -> create child Invocation
  -> Agent B
```

子调用必须保留 `root_task_id`、`parent_invocation_id` 和 `trace_id`，不得创建无法关联到根任务的孤立调用。

## 7. Invocation Ledger 最小事实集

第一阶段每次 Invocation 至少记录：

```text
invocation_id
root_task_id
parent_invocation_id
trace_id
caller_type
caller_id
workspace_id
target_agent_id
agent_card_version
capability
status
latency_ms
error_code
created_at
```

Ledger 应采用状态事件或等价的追加式语义。不得依赖覆盖更新后的单行状态来还原完整调用过程。

调用日志与 Ledger 必须使用统一标识。超时、取消、路由失败、协议失败和 Agent 业务失败必须可区分，不得统一压缩成未知错误。

## 8. 第一阶段范围

第一阶段必须包含：

- Frontend Console
- Gateway / Northbound API
- Agent Card Schema 与校验
- Registry、版本管理和 Card Store
- Capability Discovery
- Workspace Installation
- Invocation Dispatch
- A2A Router
- Invocation Ledger
- 至少两个采用不同 Runtime 实现的可运行 Sample Agents
- 契约测试、集成测试和核心闭环 E2E 测试

第一阶段明确不包含：

- 通用 Planner 或 Scheduler
- LLM Provider、Prompt、Tool、Memory、RAG、Session 或 Graph Workflow Runtime
- Agent 自动部署、弹性伸缩和 Kubernetes Runtime
- Rating、Billing、Revenue Sharing
- 完整 Marketplace 审核和推荐系统
- 企业级多租户、完整 RBAC/OIDC 和审批流
- Agent Certification、Benchmark 平台和 CI/CD
- 跨组织 Federation
- 为未来规模提前引入不必要的消息队列、搜索集群或服务拆分

基础身份识别、权限校验和调用记录仍然必须存在，但不在第一阶段建设完整企业治理体系。

## 9. 契约与兼容性

契约是平台资产，应先于具体实现被定义和评审。

- Agent Card 必须有独立的 Schema 版本。
- Agent 自身版本与 Agent Card Schema 版本是两个不同概念，不得混用。
- API、事件和 A2A Profile 必须声明版本及兼容范围。
- 新增可选字段属于增量变更。
- 删除字段、修改字段类型或改变既有语义属于破坏性变更。
- 破坏性变更必须升级契约版本，并提供迁移说明和兼容窗口。
- Registry 必须保留历史 Agent 版本，Installation 和 Ledger 必须引用实际解析后的精确版本。
- endpoint 可以更新，但历史 Ledger 中的 Agent 身份和 Card 版本不能被改写。

## 10. 开发流程

任何需求在实现前都应依次回答：

1. 它服务于哪个领域对象和第一阶段闭环步骤？
2. 它属于 Control Plane 还是 Data Plane？
3. 哪个模块拥有该行为和数据？
4. 是否新增或修改跨模块契约？
5. 是否影响版本兼容、权限、Trace 或 Ledger？
6. 如何通过契约测试、集成测试或 E2E 测试验证？

推荐工作顺序：

```text
澄清领域语义
-> 确认模块所有权
-> 定义或更新契约
-> 实现拥有者模块
-> 接入适配器
-> 增加测试
-> 验证可观测与错误路径
-> 更新架构文档或 ADR
```

架构边界、核心数据所有权、协议选择和破坏性契约变更必须记录 ADR。局部实现细节无需滥用 ADR。

### Spec-Driven Development

本项目采用 GitHub `github/spec-kit` 的 **Spec-Driven Development（SDD）** 方式开发。`AGENTS.md` 是长期项目宪章和所有 Spec 的上位约束；Spec Kit 生成的 constitution 必须与本文件保持一致，不得形成第二套冲突原则。

除纯文案修正等不改变系统行为的改动外，每个功能或架构变更都必须拥有独立的 `specs/<feature>/` 工件，并按以下顺序推进：

```text
observe
-> constitution
-> specify
-> clarify
-> plan
-> tasks
-> analyze
-> implement
-> tests
-> review
-> converge
```

各阶段必须遵守以下门禁：

1. `observe`：先只读扫描现有仓库、文档、契约、数据所有权和依赖方向，不得在尚未理解当前结构时直接落业务代码。
2. `constitution`：确认需求不违反 `AGENTS.md`。若原则需要演进，先形成可追溯决策并同步更新本文件及 Spec Kit constitution。
3. `specify`：在 `spec.md` 中描述用户场景、需求、范围、验收标准和非目标，不提前绑定实现细节。
4. `clarify`：实现前消除会影响公共契约、领域语义、权限、数据所有权或失败行为的歧义。不能确认的内容必须明确标记，禁止猜测 fallback。
5. `plan`：基于已澄清 Spec 形成 `plan.md`，并按需要生成 `research.md`、`data-model.md`、`contracts/` 和 `quickstart.md`。技术计划必须服从本文件的 Control Plane / Data Plane 边界。
6. `tasks`：从 Spec 和 Plan 派生 `tasks.md`，标明依赖顺序、可并行项、模块所有者和互斥写入范围；任务不得引入 Spec 中不存在的产品行为。
7. `analyze`：落代码前检查 constitution、Spec、Plan、契约和 Tasks 的一致性。存在未解决的高影响冲突时不得进入实现。
8. `implement`：只实现已批准 Spec 和 Tasks 中的行为。实现中发现需求缺口或契约矛盾时，先回写并重新分析 Spec，不得让代码成为事实来源。
9. `tests`：本项目采用先完成约定实现、再补测试的顺序，不采用 TDD 作为强制流程。测试必须映射到 Spec 验收场景和失败语义，且不得用同一变更新增的测试反向证明未经 Spec 批准的行为。
10. `review`：实现和测试完成后，由未参与该实现的 Review Agent 按 Spec、Plan、Tasks、契约和本文件独立评审。测试通过不能替代 Review。
11. `converge`：Review 或验收发现的剩余工作必须回写到 Spec 或 Tasks；修复后重新经过独立 Review，直至不存在阻塞性交付问题。

任何范围、用户可见行为、公共 API、事件语义、数据模型或兼容性变化，都必须先更新对应 Spec，再修改代码和测试。历史 Review 结论和线上缺陷可以触发 Spec 修订，但不能绕过 SDD 门禁直接成为局部补丁。

### Git 提交身份

本仓库的 Commit 必须使用以下仓库级身份：

```text
user.name  = Nene7ko_
user.email = 1604009816@qq.com
```

该身份应配置在当前仓库的本地 Git Config 中，不得为此修改开发者机器上的全局 Git 身份。提交前应确认本地配置与本节一致。

### AI Fallback 禁用策略

涉及默认值、空结果、异常吞并、重试、降级、兼容分支、备用 Provider、可选链或配置兜底的设计、开发、修复和评审任务，必须读取并遵循以下技能：

```text
C:\Users\16040\.codex\skills\ai-fallback-disable\SKILL.md
```

该技能在本项目中的核心原则是：**Fallback 必须是有证据的产品、领域、接口或运维策略，不能只是为了避免报错或让流程继续运行。**

必须遵守以下规则：

1. 默认的 fallback 新增预算为 `0`，优先删除无依据的 fallback。
2. 只有既有需求、契约、测试、ADR、Runbook、SLO 或调用方行为能够证明策略时，才允许保留或新增 fallback。
3. 不得用 `null`、`false`、`[]`、`{}`、空字符串、零值或成功响应掩盖依赖故障和系统错误。
4. 必须区分缺失、空值、非法输入、未找到、无权限、功能禁用和依赖失败，不能把这些状态压缩成同一个返回值。
5. 身份、用户、租户、角色、权限、价格、余额、库存、账单、状态机、存储目标、数据路由和生产 endpoint 不得使用推测出的默认值。
6. Secret、Token、API Key、JWT/Session Secret、私钥、证书和签名材料不得设置默认值，也不得静默 `trim` 后继续使用。
7. 必需的数据库地址、生产服务地址和安全配置缺失、空白或非法时必须在所属边界明确失败，不得回退到 localhost、Mock 值或弱密钥。
8. 数字、布尔值、枚举和 URL 必须显式解析和验证，不得使用 truthiness 或 `X || default` 混淆缺失、非法值和合法 falsy 值。
9. 如果边界或类型已经保证值存在，核心逻辑应直接使用该契约，不得重复增加可选链、保护分支、catch/rethrow 或防御包装。
10. 不得为了“更安全”“更友好”“兼容未来”而自行增加重试、备用数据源、旧字段兼容、静默降级或 Feature Disable。
11. 当策略不清晰且修改会影响公共契约时，必须标记为 `Needs policy`，说明缺失的决策，不得猜测临时方案。
12. 新增测试只能验证已存在的策略，不能用同一变更中新写的测试反向证明 AI 刚刚发明的 fallback 合理。

处理 fallback 相关改动时，应先清点范围内的 fallback，再将每项分类为 `Remove`、`Keep` 或 `Needs policy`。每个 `Keep` 必须说明政策证据、触发条件、返回语义、恢复责任边界、降级可见性和覆盖测试。

代码改动完成时必须报告：

```text
Fallback delta: removed N, retained N, added N, net +/-N
Added fallback evidence: none | 每个新增 fallback 的明确政策来源
```

`added N` 通常必须为 `0`。把 fallback 移入 helper、catch、retry、validator wrapper、alternate source、compatibility branch 或 degraded result 仍然属于新增或保留 fallback，不能通过改名规避检查。

## 11. 测试策略

测试按风险和边界分层：

- 单元测试验证模块内部规则和状态转换。
- 契约测试验证 Agent Card、Northbound API、Internal API、A2A Profile 和事件兼容性。
- 集成测试验证 Control Plane、数据库、Router 和 Agent 之间的真实边界。
- E2E 测试验证用户可见的完整闭环。

第一阶段必须至少覆盖以下 E2E 场景：

1. 注册 Agent Card 并发布版本。
2. 按 capability 搜索 Agent。
3. 安装、禁用和卸载 Agent。
4. 调用已安装 Agent 并接收结果或流式事件。
5. 拒绝调用未安装、已禁用或无权限的 Agent。
6. Agent A 通过 Router 调用 Agent B，并形成完整父子调用链。
7. 超时、取消、endpoint 不可达和 Agent 错误均能在 Ledger 中定位。

## 12. 完成标准

功能只有同时满足以下条件才算完成：

- 行为属于正确的模块，没有绕过架构边界。
- 跨模块数据通过已版本化契约传递。
- 成功、失败、超时和取消路径都有明确语义。
- Invocation 和 Trace 标识能够贯穿相关调用链。
- 不在 Card、日志、事件或 Ledger 中泄漏密钥。
- 按风险补充了单元、契约、集成或 E2E 测试。
- 用户可见行为、契约或架构发生变化时已更新相应文档。
- 第一阶段核心演示仍能完整运行。

## 13. 第一阶段验收闭环

项目必须能够现场演示：

```text
提交 Agent Card
-> Registry 注册并发布版本
-> 用户按 capability 搜索
-> 安装到 Workspace 并接受权限
-> Console 发起调用
-> Gateway 创建调用上下文
-> Router 解析 endpoint 并执行 A2A 调用
-> Agent 返回结果
-> 嵌套调用再次经过 Router
-> Console 查询完整 Ledger 调用链
```

第一阶段成功的判断标准不是目录数量、页面数量或支持多少 Provider，而是平台是否真正统一了 Agent 的描述、发现、授权、调用和追踪。

验收中的两个 Sample Agents 必须来自不同 Runtime 实现。至少一次 Agent-to-Agent 调用必须在不共享 Runtime 内部类型或存储的前提下完成，以证明 NeKiro 提供的是跨 Framework 平台能力，而不是某个 Agent Framework 的二次封装。

## 14. 演进原则

未来可以按真实压力逐步演进：

- Discovery 查询量和匹配复杂度足够高时，再替换为独立搜索引擎。
- Ledger 吞吐或实时订阅需要时，再引入事件流和独立存储。
- Control Plane 内部模块出现独立扩缩容或团队边界时，再拆为服务。
- Policy Hooks 逐步承载 RBAC、Quota、Approval、Cost 和数据治理。
- Runtime 层逐步扩展 Agent 部署、健康检查、伸缩、升级和回滚。
- Marketplace 最终建立在 Registry、Installation、Certification、Billing 和生态协议之上。

演进时必须保留 Agent Card、Registry API、A2A Profile 和调用事件的兼容性。技术实现可以替换，平台共同语言和边界不能被无意破坏。
