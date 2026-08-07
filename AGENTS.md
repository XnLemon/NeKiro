# NeKiro Core repository contract

本文件适用于整个 Core 仓库。它记录稳定的所有权、架构和交付边界；功能讨论与交付记录使用 GitHub Issues、Pull Requests、Reviews 和 Releases，架构变化使用 ADR。

## 1. 仓库范围

Core 只维护以下内容：

- `apps/control-plane`：Gateway、Catalog、Workspace、Invocation Dispatch。
- `apps/a2a-router`：A2A 路由、上下文传播、凭证、策略扩展点和 Invocation Ledger。
- `config_center/`：provider-neutral 的配置源读取、观察和显式发布边界。
- `registry`：provider-neutral 的临时 Agent 实例拓扑与适配器；它不是
  Catalog Registry 的第二事实源，也不拥有 Release、Endpoint Binding、权限或持久化。
- `contracts`：语言无关的 JSON Schema、OpenAPI、A2A Profile 及其 Go 映射。
- Catalog、Workspace、Ledger 各自拥有的 PostgreSQL migrations。
- Core 单元、契约和服务集成测试。
- `docs/architecture`、`docs/contracts`、`docs/decisions`、`docs/usage`。

以下内容由独立仓库维护，不得在 Core 建立第二份可写源码：

- Console：`NeKiro-project/NeKiro-Console`
- Go SDK：`NeKiro-project/nekiro-sdk-go`
- Sample Agents：`NeKiro-project/NeKiro-Samples`
- 多组件装配与产品 E2E：`NeKiro-project/NeKiro-Stack`
- 通用 A2A wire transport：`NeKiro-project/nekiro-a2a-transport-go`

## 2. 产品与架构边界

NeKiro Core 管理 Agent 的外部生命周期和组织级信任边界：注册、版本化发布、发现、Workspace 授权安装、受控调用和跨 Agent 审计。模型、Prompt、Tool、Planner、Workflow、Memory、RAG、Session 和 Runtime 内部执行不属于 Core。

核心闭环是：

```text
Register -> Discover -> Install -> Invoke -> Record
```

必须遵守：

1. Console 和外部应用只能访问 Gateway。
2. Gateway 不直接调用 Agent；平台托管调用全部交给 A2A Router。
3. Registry 是 Agent Card 与 Release 的唯一事实来源；Discovery 只提供派生查询。
   根级 `registry/` 仅报告已经精确授权 Release 的临时实例拓扑，不得读取、写入、
   解析或替代 Catalog Registry 事实。
4. A2A Router 通过受控接口解析精确 Agent Card/Release，不保存第二份永久 Card。
5. Agent-to-Agent 托管调用必须再次经过 Router，并传播 `root_task_id`、`parent_invocation_id` 和 `trace_id`。
6. Ledger 只记录追加式 metadata 事实，不保存 Agent 输入、输出、凭证或密钥。
7. 模块只能写入自己拥有的数据；共享数据库实例不改变数据所有权。
8. 跨进程数据必须使用 `contracts/` 中的版本化契约，不共享内部实现类型。
9. Control Plane 与 Router 是独立部署边界；Router 不导入 Control Plane 的 internal package。
10. Core 的 PR required CI 不 checkout、构建或测试卫星仓源码。独立的
    post-merge 编排 workflow 可以调用由卫星仓拥有、并固定到完整 commit
    SHA 的 reusable workflow；源码解析、测试命令和成功判据仍由对应 owner
    仓定义，且该编排不得成为 Core 本地构建的备用路径。

### Config Center ownership

根级 `config_center/` 只拥有 provider-neutral 的不透明字节源语义：严格
Key、不可变快照、当前读取、原子 initial/watch handoff、局部 revision、
typed outcome，以及读能力和显式发布能力的边界。它不拥有 Control Plane 或
A2A Router 的 typed configuration document、Agent Card/Release、Catalog
Registry、Workspace、Secret 分发或任何运行时治理策略。

Control Plane 和 A2A Router 继续各自拥有服务配置 schema、业务验证、readiness、
动态字段接受范围和 policy 决策。File provider 是本地/受控部署适配器；Nacos、
loader 注入和 dynamic governance 必须由独立 Issue/ADR 明确数据所有权、失败语义
与 readiness 策略，不能把 `config_center/` 变成隐式 source precedence 或 fallback。

## 3. 技术约束

- Core 服务使用 Go；`go.mod` 的正式身份是 `github.com/NeKiro-project/NeKiro`。
- PostgreSQL 是持久化存储。SQL migration 必须与 Catalog、Workspace 或 Ledger 的 owning package 一起发布，并通过 `go:embed` 加载。
- Node.js、React、Vite、pnpm 与 Playwright 不属于 Core 工具链。
- 通用 A2A HTTP/JSON-RPC/SSE mechanics 由独立 transport module 提供；Core 只维护 NeKiro policy 和 contract adaptation。
- 不得使用本地 `replace`、复制源码、浮动 branch/tag、备用组件或旧 module shim 连接跨仓依赖。

## 4. 契约与兼容性

契约先于跨边界实现。Agent Card Schema、Agent version、HTTP API、internal API、event、result、A2A Profile 和 Router credential 各自独立版本化。

- 新增可选字段只有在省略时保持既有语义才是兼容变更。
- 删除/改名字段、修改类型或 requiredness、改变状态码/媒体类型/错误语义、移动 owner 都是破坏性变更。
- 破坏性变更必须新增契约版本、迁移说明和明确兼容窗口；没有政策证据时不得增加双读、双写或 fallback。
- 历史 Release、Installation 和 Ledger 引用的精确版本与 provenance 不得被改写。

## 5. 开发与评审

变更前先确认领域对象、Control Plane/Data Plane 归属、数据 owner、跨边界契约、兼容性、Trace/Ledger 影响和验证方式。

用户可见行为、公共契约、数据模型或核心架构变化必须先有 GitHub Issue；架构/所有权/破坏性兼容决策必须更新或新增 ADR。实现完成后必须由未参与实现的人独立评审。

提交前至少运行与风险相称的检查：

```text
gofmt -l apps config_center contracts registry tests
go mod tidy
go build ./...
go test ./...
go test -race ./...
go vet ./...
```

数据库变更必须运行 Catalog、Workspace 和 Ledger 对应的 PostgreSQL integration suite。跨仓产品行为由 NeKiro-Stack 使用精确 revisions 验收。每次 Core `main` 合并后，独立的 Satellite Integration workflow 必须用该完整 Core commit SHA 调用 SDK compatibility、Samples compatibility 和 Stack backend/browser acceptance；失败必须保持为可见的跨仓失败，不得回退到旧 Core revision。

## 6. 失败与 fallback

Fallback 默认预算为 0。缺失、空值、非法输入、未找到、无权限、功能禁用和依赖失败必须保持不同语义；不得用空结果、零值、成功响应、重试、备用 endpoint、旧字段兼容或缓存旧组件掩盖失败。

Secret、Token、JWT key、证书、数据库地址、生产服务地址和安全配置不得有推测默认值。必需配置缺失或非法时必须在所属边界明确失败。

新增 fallback 必须有既有契约、ADR、Runbook 或 SLO 的明确政策证据，并在变更说明中报告：

```text
Fallback delta: removed N, retained N, added N, net +/-N
Added fallback evidence: none | policy source
```

## 7. 完成标准

- 行为和数据位于正确 owner，未绕过 Gateway/Router/Registry/Ledger 边界。
- 跨模块数据通过版本化契约传递。
- 成功、失败、超时、取消和依赖故障具有明确语义。
- Invocation/Task/Trace lineage 完整，且 Card、日志、事件、错误和 Ledger 不泄漏密钥。
- 单元、契约、PostgreSQL integration 或 Stack E2E 覆盖与风险匹配。
- Core checkout 不依赖 Console、SDK、Samples、Stack、Node 或 Spec Kit 源码即可通过 `CI / required`。

## 8. Git identity

本仓库提交必须使用仓库级配置，不修改全局 Git identity：

```text
user.name  = Nene7ko_
user.email = 1604009816@qq.com
```
