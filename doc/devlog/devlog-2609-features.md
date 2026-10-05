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

### 微信手工菜单可行性复核与探针撤销

最初为验证未认证公众号的手工菜单是否推送 `VIEW`，曾加入后台事件探针。用户随后根据公众号后台实际选项确认，“跳转网页”只能选择公众号链接，不能填写 FmlySys 自定义网址。因此，菜单无法打开系统登录/票据页面，VIEW 回调与手机浏览器请求也无法自动关联，单独观察事件不能达成登录目标。本次撤销开发中心页面、导航和事件缓冲逻辑及其专用测试，不再为这条不可落地的菜单链路增加功能。

保留 `doc/wechat-code-login.md` 作为完整接入、维护和排障指南，并改为明确说明该账号菜单链接限制及其影响。对当前账号仍建议采用已实现的公众号二维码验证码登录；只有账号权限或可用链接条件变化后，才重新评估菜单一次性票据方案。`FMLYSYS_WECHAT_OA_TOKEN` 对应公众号表单 Token，当前回调只支持明文 XML，EncodingAESKey 不用于现有实现。

撤销探针后运行 `go test ./internal/httpserver ./web` 均通过，`git diff --check` 通过。保留 `/login/wechat-code` 路由注册测试；未暂存、未提交。

另核对线上 `/login/wechat-code` 异常：当前 `server.go` 明确注册 `GET /login/wechat-code`，因此 502 与 `Cannot GET` 并非此 handler 的应用响应。前者需检查 Cloudflare 到源站的连通与源站进程，后者表示请求到达未包含该路由的旧/不同服务或代理目标。保留路由注册测试，并将区分步骤补充到接入指南；无法在没有线上域名和部署访问权限的情况下验证具体源站状态。

### 微信验证码登录 502 手机诊断

用户反馈线上 `/login/wechat-code` 仍显示 Bad Gateway，因此增加管理员专用的 `/admin/developer` 诊断页。后台提供关联临时编号的源站探测和登录页测试链接，并展示当前进程捕获的最近 100 条异常请求；日志限制在微信登录页/提交/回调、健康检查和源站探测路径，仅存进程内存。用户进一步要求保留原始排障细节，因此记录失败请求的完整查询参数、Headers（含 Cookie）、正文、远端地址、响应正文及未脱敏错误，便于管理员导出后自行脱敏；正常 200 和重定向等响应不记录。

新增 `/__diag/ping` 只读探测响应，并在微信浏览器保护中允许此路径通过。二维码生成失败时完整记录原始错误，因此诊断日志可能包含 access_token 等敏感值，并由管理员权限保护。该工具帮助识别 Go 服务返回的 4xx/5xx；未到源站的错误仍须在 Cloudflare/WAF/反向代理平台查看。无记录时还需排除服务重启或多实例切换。

按用户补充，普通路径的 200 等 2xx/3xx 正常结果不写入记录；公众号回调和出站微信 API 作为专门的诊断类型，即使返回 200 也完整记录。出站 trace 覆盖公众号 access_token、临时二维码、公众号用户信息，以及原有网页登录 OAuth 的 token/profile 请求及其 URL、Headers、正文、响应、耗时和错误；入站 callback 覆盖 OAuth 回调、公众号 GET 验证与 POST 事件/文本消息，包括原始 query、Headers、XML 请求及 FmlySys 返回的 XML/文本正文。这样可以通过二维码 API 正文中的 scene 与回调 XML EventKey 对照，并检查 FromUserName、文本“登录”和验证码回复内容。

后台表格按“微信 API”“微信回调”“HTTP 错误”“服务异常”“登录错误”区分，单条可展开原始详情，支持复制或下载 JSON。每个 Headers 集合、请求/响应正文和错误字符串最多保留 64 KiB，超限标记截断；最多 100 条，保存在进程内存。明文可能包含 AppSecret、access_token、Cookie、验证码、OpenID 及客户端代理信息，供管理员自行导出脱敏。诊断页与 JSON 接口仍由 adminOnly 保护。

验证：`go test ./internal/httpserver ./internal/wechat ./web ./cmd/fmlysys` 通过；覆盖原始请求/响应和错误内容、公众号及 OAuth 成功回调、普通成功 ping 的排除，以及公众号和网页登录 OAuth API exchange trace。`node --check web/static/admin-developer.js` 和 `git diff --check` 通过。诊断日志与 API 由管理员会话保护；未暂存、未提交。

### 公众号二维码 48001 排查与出站网络跟踪

线上原始诊断显示 `/cgi-bin/token` 成功返回 access token，紧接着 `/cgi-bin/qrcode/create` 由微信以 HTTP 200 返回 `errcode=48001, errmsg=api unauthorized`。因此网络、TLS、AppID/AppSecret 和 token 获取链路均已走通；实际失败点是当前公众号没有“生成带参数二维码”接口权限，不是 Cloudflare 入站代理，也不是 IP 白名单。典型白名单错误为 token 接口的 `40164 invalid ip`。个人未认证公众号需以后台“接口权限”页为准；若没有该能力，需更换具备权限的公众号或使用公众平台接口测试账号联调。

为后续确认出站路径，微信 HTTP trace 增加代理 URL、DNS 结果、TCP connect 目标、本地/远端 socket、连接是否复用、TLS 服务名/版本及连接错误。后台表格的“来源信息”和原始 JSON 都会显示这些字段：直连时远端通常是微信边缘地址，使用 `HTTPS_PROXY` 时远端通常是代理地址。Cloudflare CDN 只影响访问 FmlySys 的入站流量；NAT 或上游代理后的最终公网出口地址无法从本机 socket 反推，应以微信 `40164` 回显或同代理链路的出口探测结果为准。后台对二维码接口的 `48001` 和微信 API 的 `40164` 增加直接判断提示。

验证：`go test ./internal/httpserver ./internal/wechat ./web ./cmd/fmlysys`、`node --check web/static/admin-developer.js` 和 `git diff --check` 均通过；网络 trace 测试确认能够捕获实际 socket 本地/远端地址和 connect 目标。未执行 `git add`，未提交。

### 静态公众号二维码与 OpenID 验证码登录

用户质疑在个人公众号无法调用动态二维码接口时，静态二维码加公众号消息验证码是否能完整替代原流程，并提出在新关注时直接回复验证码、已关注用户在网站提示发送“登录”。结论是：不再调用 `/cgi-bin/qrcode/create`、而通过已配置的消息推送回调发放一次性验证码是可行实现；但静态二维码不携带每次网页独有的 scene，因此无法把验证码绑定到某个浏览器页。验证码只能与 `FromUserName` 对应的 OA OpenID 关联，属于短时 bearer credential，必须提醒勿转发，并做单次消费、五分钟到期、每 OpenID 冷却、错误次数和提交频率限制。真实账号是否接收 subscribe 与文本回调、以及 OA OpenID 是否能通过现有绑定或 UnionID 匹配成员，仍需上线前用该个人公众号实际验证；代码无法替代这项账号实测。

实现已将 `/login/wechat-code` 改为本地渲染，不向微信请求 token 或动态二维码。`FMLYSYS_WECHAT_OA_QR_CODE_URL` 默认显示仓库内的 `/static/shabigongzhonghao.jpg`，也允许替换为 HTTPS 图片 URL 或站内路径。`subscribe` / `SCAN` 回调立即同步回复验证码，已关注用户的“登录”文本回调也发放验证码。数据库新增 `000012_wechat_openid_login_codes.sql`，按 OpenID 保存 HMAC 摘要和时效；成功兑换后原子消费并按 OpenID/UnionID 匹配已审核成员。页面的 state cookie 继续防跨站提交，但不被误称为验证码的浏览器绑定。

同时补齐 Windows/Linux 首次配置模板、README、登录页说明及 `doc/wechat-code-login.md` 接入、回调和排障文档。保留旧的动态场景尝试表和 API 客户端方法供数据库升级及历史代码兼容，但新登录页不再调用它们。当前 turn 按环境约束未运行或新增测试；后续部署仍应实测公众号 `subscribe` 被动回复、已关注用户文本回复和 OpenID/UnionID 成员匹配。未执行 `git add`，未提交。

### 公众号关注与“登录”消息无回复排查

日期：2026-10-05。用户测试新关注、重新关注和发送“登录”，均未收到验证码；公众号后台消息推送 URL 为 `https://fmly.miku.us/auth/wechat/code/callback` 且处于启用状态。开发中心最初未显示真实 `subscribe` 或文本 POST。随后用户提供的两条 GET 诊断是本次对线上接口的手工探测，不是公众号事件：普通 User-Agent 返回 `403 invalid signature`，`MicroMessenger/8.0` 返回 `200` 的“请在手机自带浏览器中打开”HTML。线上路由与静态公众号二维码可达，直接证明微信浏览器保护误拦截了该回调路径；如果公众号服务器也带 `MicroMessenger` User-Agent，则 GET 验证响应会错误，POST 消息会在进入回调处理器之前被拒绝。两条 GET 不能证明公众号事件已经到达，也不能据此排除 Cloudflare/WAF 或公众号平台投递问题。

修复方案：将 `/auth/wechat/code/callback` 列入微信浏览器保护的精确路径例外，允许 GET 签名验证和 POST XML 消息进入原有处理器；全站超级审计对该外部 webhook 直接放行，避免被其他写操作的审计锁和附加数据库查询拖慢被动回复。原有回调签名验证仍然执行，其他微信内置浏览器功能页继续受保护。新增 GET/POST 带 `MicroMessenger` User-Agent 的回归测试，并以当前静态二维码发码表运行贯穿路由和中间件的签名 GET、关注 POST、“登录”文本 POST 测试，确认都能得到预期的 echostr 或验证码 XML。接入文档写明如何识别遮罩 HTML、手工探测与真实公众号消息。

验证：线上未部署修复前，额外用无签名 XML 探测 POST，`MicroMessenger/8.0` 返回 `403` 微信浏览器保护文案，普通 User-Agent 返回 `403 invalid signature`，证明公网 POST 可转发到 Go 应用，但现有保护会先于签名处理器拦截微信 UA。`go test ./internal/httpserver ./web ./cmd/fmlysys` 通过；同次运行的 `internal/store` 全包仍被原有 `TestNormalizePermissionsAddsViewDependencies` 阻断（测试引用不存在的 `medication.manage` 权限），与本次修改无关。`git diff --check` 通过。部署后需在公众号后台重新验证服务器配置，再测试关注与发送“登录”，确认开发中心出现实际 POST，返回正文为验证码 XML。如果仍没有 POST，应依据公众号后台状态、Cloudflare/WAF 与反向代理日志继续查投递链路，不能把代码侧修复视为真实公众号已验证成功。本次未执行 `git add`，未提交。

### 回调放行修复上线后的复测

日期：2026-10-05。用户反馈部署后关注公众号和发送“登录”仍未收到验证码。再次对 `https://fmly.miku.us/auth/wechat/code/callback` 发起无签名探测：带 `MicroMessenger/8.0` 的 GET 与 POST 均返回 `403 invalid signature`。这说明当前线上实例不再返回浏览器保护 HTML/403 文案，且无签名请求已走到回调处理器；该探测不是微信签名请求，也不证明真实关注/文本 POST 到达。待用户提供开发中心最新真实 POST 原始记录后，按结果区分：无 POST 继续查公众号投递/Cloudflare/代理；403 查线上 Token 和消息加密模式；200 `success` 查 XML 事件、正文是否是 `<Encrypt>`；验证码 XML 则查公众号客户端收件状态。排障文档已补充签名错误及加密 XML 的识别方法。

用户随后确认开发中心没有真实 POST 记录。现阶段可确认应用拦截修复在线上生效，但真实公众号消息仍未到当前观察的进程，无法仅凭本地代码继续判断是公众号投递、Cloudflare/WAF、反向代理、多实例还是日志观察进程不一致。新增 `doc/wechat-code-login-unresolved.md`，明确记录未验收状态、分层诊断步骤、日志结果判断、端到端验收条件，并遵照用户要求标记 `dev-2609A-step2-fake-wechat-qrlogin` 为专用分支：在真实公众号关注与文本回复验收完成前，不合并到主分支、`dev-2609A-step1` 或 `dev-2609A`。
