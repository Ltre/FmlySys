# 2026-09 财务时间线开发记录

日期：2026-09-30  
分支：`dev-2609A-step1`

## 需求与实现方案

在公共资产页 `#timeline-card` 放置扁平入口卡片，点击后覆盖当前视口打开财务时间线；首页公共资产快捷操作区增加同名按钮，导航至 `/assets` 并自动打开浮层。时间线合并公共消费、成员转账、报销登记、资产变动四类有效记录，默认每页 20 条，按发生时刻倒序显示。

浮层由 `/assets` 页面服务端渲染。分页状态使用 `timeline=1&page=N` 表示，因此入口、分页和直接打开链接最终都走现有 `/assets` 路由。该路由由 `member("assets.view", ...)` 保护；首页快捷操作和资产页入口也只在 `assets.view` 权限存在时渲染。查询参数只控制浮层与页码，不授予权限，也没有新增可绕过成员权限的数据接口。

时间线在 Store 层用一条合并查询读取四张业务表的 `active` 记录，使用 `julianday(occurred_at)` 做跨来源时间比较，并以原始时间、类别和记录 ID 为同秒排序的稳定次序。服务端按总记录数计算页数，并把越界页码收敛到最后一页。页面沿用站点时区脚本格式化 ISO 时间戳，显示年月日、小时、分钟和秒。

记录呈现采用纵向动态卡片：类别 TAG、类别图标、参与成员、事项说明、时间和金额分层展示，并用颜色区分消费支出、报销支出、内部转账和资产增减。消费、报销及资产变动按账务方向显示正负号；成员间转账显示转出与转入双方。报销登记展示 `reimbursements` 表中的正式登记；消费发生时由既有业务逻辑自动使用代管资产的金额已经属于消费记录，不重复生成一条报销登记。

## 修改内容

- `internal/store/models.go`：增加时间线记录和分页数据结构。
- `internal/store/financial_timeline.go`：增加四类有效账务记录的统一查询、计数和分页。
- `internal/httpserver/server.go`：在资产页读取分页数据，并依据 `timeline=1` 决定初始浮层状态；时间线仍由既有 `assets.view` 路由权限拦截。
- `web/templates/assets.html`：完成 `#timeline-card` 入口和全屏对话浮层，包含空状态、秒级时间、金额方向及上一页/下一页导航。
- `web/templates/dashboard.html`：在首页 `.quick-actions` 增加“财务时间线”按钮，链接带自动打开参数。
- `web/static/financial-timeline.css`：实现扁平入口、全屏浮层、时间轴卡片、分页和窄屏布局。
- `web/static/financial-timeline.js`：实现打开/关闭、URL 状态同步、浏览器返回、Esc 关闭和对话框焦点管理。

## 检查与工作区状态

对 Go 文件执行了 `gofmt`。按本次任务要求，没有运行测试，没有执行 `git add`，也没有创建 Git commit。修改前工作区已有 `go.mod` 和 `go.sum` 未提交改动；本轮保留这两项改动，没有把它们纳入功能实现。

## 建议 Git 文案

**Title**

```text
feat: add permission-gated financial timeline
```

**Description**

```text
Add a full-screen timeline on /assets that merges public expenses, member transfers, reimbursements, and asset changes in occurrence-time descending order, with second-level localized timestamps and 20-entry pagination.

Expose the timeline from the assets card and dashboard quick actions, including direct links that open the overlay on arrival. Gate both entry points and timeline data through the existing assets.view permission and present records as tagged, color-coded activity cards.
```

## 财务时间线交互续作

日期：2026-09-30
分支：`dev-2609A-step1`

### 浏览状态与最小化

在时间线标题栏关闭按钮左侧增加 `_` 最小化按钮。最小化时把当前页码、浮层滚动位置以及可见记录锚点和锚点相对视口偏移保存在当前标签页的 `sessionStorage`，隐藏浮层并清理打开参数。卡片和资产页快捷菜单重新打开时优先读取保存状态：若当前页已渲染对应页码则原位恢复；否则带保存页码重新加载；从首页打开的无页码链接也会在页面加载后恢复保存页码。

关闭按钮、Esc 和背景点击保留原有关闭语义，不额外覆盖上次保存的位置；`_` 最小化按钮负责保存当前浏览位置。

### 从动态记录跳转到流水

每条时间线卡片及轴上的节点图标都可点击，链接携带记录类别与 ID。点击后先保存当前页和所选时间线节点的位置，再写入既有 `fmlyRecordFocus` 状态、隐藏浮层，随后导航到 `/assets#<类别>-record-<ID>`。消费、成员转账、报销登记和资产变动流水行分别增加对应 DOM 锚点和现有 `data-record-key`，由 `record-focus.js` 继续完成精确滚动与高亮。

为确保时间线中的任意有效资产变动都有目标行，资产变动流水查询不再只取最新 100 条；其它三类流水此前已完整提供对应记录。

资产页浮动快捷菜单现在把“财务时间线”放在第一项，并复用卡片入口的恢复逻辑。

### 本轮改动与检查

- 更新 `FinancialTimelineEntry` 和合并查询，向模板提供每条记录的 ID。
- 更新 `financial-timeline.js`、`financial-timeline.css` 及资产页模板，加入最小化恢复、记录跳转和流水锚点。
- 在 `#member-assets-action-menu` 首项加入受 `assets.view` 控制的财务时间线入口。
- 移除 `Store.AssetEvents` 的 100 条 SQL 上限，使旧资产事件仍有可定位流水行。
- 对 Go 文件执行 `gofmt`，并对已跟踪改动执行 `git diff --check`。未运行测试，未执行 `git add`，未提交。

**Title**

```text
feat: add resumable financial timeline navigation
```

**Description**

```text
Add a minimize control that saves the current timeline page and reading position, then restores them when the timeline is reopened from the assets card or floating quick menu. Make timeline records clickable and route each one to its matching expense, transfer, reimbursement, or asset movement row, reusing the existing record focus and highlight behavior.

Put the financial timeline first in the assets quick menu and ensure older asset movement records remain available as jump targets.
```

## 2FA 动态验证码登录与注册

日期：2026-10-05
分支：`dev-2609A-step3-2fa-login`

### 需求与实现方案

在普通登录页增加“通过 2FA 登录”入口，进入默认选中“登录”的双选项卡界面。登录表单以用户名和验证器生成的 6 位 TOTP 动态码完成认证；注册表单收集用户名、可选补充备注、绑定二维码和动态码。二维码使用 `otpauth://totp`，标签按 `Fmly: 用户名 [补充备注]` 更新，补充备注为空时省略方括号部分。空用户名时不生成二维码；用户名或备注变化时先隐藏旧码，再刷新二维码，服务端保持同一个短时注册密钥。页面显示服务端随二维码响应返回的完整标签，便于核对扫码内容。

2FA 身份单独存储，不自动创建成员或授予家庭权限。注册成功后由管理员在“2FA 身份”后台页面将身份关联到有效家庭成员；登录仅在关联有效时创建现有成员会话。2FA 种子使用现有持久主密钥 AES-GCM 加密，注册二维码状态使用短时 HttpOnly Cookie 和哈希 token 保存，验证码按 TOTP 时间步原子消费以阻止重放，并对登录尝试按 IP 和用户名/IP 限流。

### 修改内容

- 新增分区迁移和 Store 方法，保存加密 TOTP 种子、注册临时状态、成员关联与最近消费时间步。
- 新增 `/login/2fa` 登录/注册页面、动态二维码接口、注册与登录处理，以及管理员身份列表和成员关联操作。
- 在普通登录页添加“通过 2FA 登录”入口，在后台导航加入“2FA 身份”管理入口。
- 注册错误回显时默认回到注册选项卡；选项卡支持左右方向键切换。

### 安全边界与检查

未关联成员的注册身份不能进入家庭系统；管理员解绑或关联成员失效后不能登录。登录在一个数据库事务中校验仍有效的成员关联、消费当前 TOTP 时间步并创建成员会话，避免并发解绑导致身份越权。验证码密钥不通过 URL 或表单明文存入数据库。

对 Go 文件执行 `gofmt`，`go build ./...`、`node --check web/static/totp-login.js` 和 `git diff --check` 均通过。本轮未运行测试、未执行 `git add`，未创建 Git commit；工作区原有 `go.mod` 与 `go.sum` 改动保留，不属于本功能改动。
