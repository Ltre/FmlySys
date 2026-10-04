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

## 微信公众号验证码登录

日期：2026-10-04
分支：`dev-2609A-step2-fake-wechat-qrlogin`

### 参考站点调查与身份链

尝试读取 `https://aiwave.site/` 的公开页面和检索结果。浏览器只返回页面标题“DevTools Hub - 程序员软件宝库”，页面内容读取超时；命令行 TLS 请求也失败。站点后端源码及其真实 API 因而无法公开检查，不能据此断言它具体如何存码或绑定用户。附图和说明提供了可见交互：扫描公众号二维码、发送“登录”、把收到的验证码填回网页。

本项目已有微信登录通过网站 OAuth 获得 OpenID/UnionID，将身份审核绑定到成员后签发 `fmly_session`。公众号回调收到的 `FromUserName` 是该公众号范围内的 OpenID，不能假定它等于网站应用 OpenID；实现先从公众号用户信息接口读取 UnionID，再与已审核 `wechat_identities` 中的 UnionID 匹配。微信公众号和网站应用需要绑定在同一微信开放平台账号下才能跨应用匹配；没有 UnionID 时只接受完全相同的 OpenID。该方式不会创建新成员或绕过既有审核绑定。

### 实现方案与修改

- 登录页新增“微信验证码登录”入口和独立验证码页。服务端向公众号申请 10 分钟有效的 `QR_STR_SCENE` 临时二维码，每个二维码绑定随机浏览器状态；公众号服务器回调必须通过 Token、timestamp、nonce 的 SHA-1 签名校验。
- 用户扫码后，公众号的 `subscribe` / `SCAN` 事件把 OA OpenID 绑定到此次登录尝试；只有这个微信身份在该次请求下发送“登录”才会收到验证码。验证码为随机 8 位数字，有效期 5 分钟，绑定扫码的网页登录状态，成功后一次性消费；错误输入 5 次会失效，短时间重复索取会限流。
- 数据库只保存登录 state、scene 的摘要和 HMAC 验证码摘要。新增 `000011_wechat_code_login.sql`，不保存明文验证码。成员登录只复用已有的成员会话创建逻辑。
- 新增 `FMLYSYS_WECHAT_OA_APP_ID`、`FMLYSYS_WECHAT_OA_APP_SECRET`、`FMLYSYS_WECHAT_OA_TOKEN` 配置、公众号 access_token 缓存、临时二维码和用户 UnionID 调用，并更新首次启动配置模板与 README。
- 新增公众号开发者回调 `/auth/wechat/code/callback`。微信公众平台需设置该 URL、同一个 Token、明文消息模式，服务器需从公网可通过 HTTPS 访问。

### 检查状态

已对新增和修改的 Go 文件执行 `gofmt`。`internal/config`、`internal/wechat`、`internal/httpserver` 检查通过；新增 Store 验证测试也通过。`internal/store` 全包检查仍被原有 `TestNormalizePermissionsAddsViewDependencies` 阻断：它提交已不在当前权限目录中的 `medication.manage`，本次未改动该测试或权限定义。站点实际后端未能读取，按公开 UI 与本项目现有身份流程实现；未暂存、未提交。

### 管理员开发中心与菜单 VIEW 探针

为确认未认证公众号手工菜单是否把跳转事件送入已配置的消息回调，增加管理员专用 `/admin/developer` 页面和导航入口。公众号 POST 回调在 Token 签名校验通过后，将事件类型、FromUserName、EventKey 和接收时间记录到进程内最近 50 条缓冲；开发中心通过受管理员会话保护的 JSON 端点轮询展示。只记录事件元数据，不记录普通消息正文；缓冲不落库，重启清空。新增回调测试覆盖 VIEW 事件捕获与成功确认。

验证菜单时，手工“跳转网页”指向 `/healthz?source=wechat-menu-view-test`，然后由关注者实际点击；若列表出现 VIEW，则可确认事件带回公众号 OpenID。菜单 URL 本身仍为静态链接，VIEW 是独立回调，不能直接把两条请求按时间或“最后点击”配对。后续安全登录方案应让回调生成短时一次性票据并通过公众号返回用户，先完成成员身份映射和防重放限流。该菜单登录尚未实现，现有变更只提供观察与验证工具。

新增 `doc/wechat-code-login.md`，覆盖首次接入、后台与环境变量配置、实际验证码流、菜单 VIEW 验证、个人未认证公众号的边界、密钥维护和排障；README 增加文档入口。`FMLYSYS_WECHAT_OA_TOKEN` 对应公众号表单 Token，当前只支持明文 XML，EncodingAESKey 不用于现有实现。

另核对线上 `/login/wechat-code` 异常：当前 `server.go` 明确注册 `GET /login/wechat-code`，因此 502 与 `Cannot GET` 并非此 handler 的应用响应。前者需检查 Cloudflare 到源站的连通与源站进程，后者表示请求到达未包含该路由的旧/不同服务或代理目标。增加路由注册测试，并将区分步骤补充到接入指南；无法在没有线上域名和部署访问权限的情况下验证具体源站状态。
