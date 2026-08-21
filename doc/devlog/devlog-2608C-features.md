# dev-2608C-step1 开发记录

日期：2026-08-21  
分支：`dev-2608C-step1`

## 1. Step1 总体目标与既有实现

Step1 采用 Go + SQLite 单体架构，业务数据位于独立数据分区，当前默认结构：

```text
data/
├── system.db
└── partitions/
    └── p_default/
        ├── fmlysys.db
        └── uploads/
```

此前已完成公共资产、家族事务、信息共享三条基础业务线，以及 Windows 本地代理开发启动脚本。核心原则保持不变：

- 公共财产为虚拟账本，不对应真实家族金融账户；
- 按成员记录当前代管的公共财产总额，不记录其具体银行卡/微信/支付宝余额；
- 存资金事实，不把 before/current/after balance 作为权威字段；
- 内部转账只改变双方代管金额，不改变公共财产净额；
- 消费发生时即影响公共财产净额，报销只结清待报销债务，不再次扣减净额；
- 金额修改允许直接修正当前有效记录，但必须保留 Audit；
- 事务采用通用 Matter，资料采用 Archive + Attachment。

当前账务一致性仍按：

```text
所有成员当前代管公共财产合计 - 待报销金额 = 公共财产净额
```

## 2. Windows 本地开发脚本历史

`scripts/dev-windows.cmd` 已完成：

- 临时设置 `HTTP_PROXY=http://127.0.0.1:58591`；
- 临时设置 `HTTPS_PROXY=http://127.0.0.1:58591`；
- 临时设置 `ALL_PROXY=socks5://127.0.0.1:51837`；
- 同时设置对应小写变量及 `NO_PROXY=127.0.0.1,localhost`；
- 使用 `setlocal`，不污染 Windows 永久环境变量；
- 通过 `%~dp0` 从脚本自身位置解析仓库根目录，因此根目录调用、进入 scripts 后运行、资源管理器双击均可；
- 启动前依次执行 `go mod tidy`、`go mod download all`、`go mod verify`；
- 失败统一 `pause`，避免双击窗口闪退；
- Step1 未完成正式认证前固定监听 `127.0.0.1:8080`。

## 3. 本轮：前后台公共资产职责重构

### 3.1 前台 `/assets`

前台定位调整为普通成员日常操作页，不再承担管理员配置职责。

移出前台：

- 成员新增/管理；
- 完整版“登记公共资产”；
- 初始资产；
- 财务调整；
- 任意指定 A → B 的管理员内部转账。

前台保留并简化：

- 当前登录成员自己的资产新增；
- 当前登录成员自己的资产减少；
- 公共消费；
- 与其他成员的转账；
- 报销；
- 公共资产/消费/转账/报销流水查看。

当前仍处 Step1 开发态，所谓“登录成员”由 `ActorID` 对应的开发成员模拟；正式微信身份接入后这些前台操作将直接绑定真实 session member_id。

### 3.2 后台 `/admin`

后台由占位页升级为公共资产完整版管理页，包括：

- 成员管理；
- 完整资产变动登记；
- 任意成员的公共消费登记；
- 任意成员间内部转账；
- 指定报销付款持有人；
- 全部资产变动、消费、转账、报销流水。

正式管理员密码 + Google Authenticator TOTP 仍属于下一阶段认证工作；当前 `/admin` 仍仅用于本地开发验证，不应公开部署。

## 4. 资产变动语义

### 4.1 初始资产 `INITIAL_ASSET`

定义为“系统开始记账时，该成员已经代管的公共财产起始值”。

本轮增加硬约束：**每个成员最多只有一条有效初始资产记录**。

新增 migration 在创建唯一索引前会处理既有异常数据：如果某成员已经存在多条有效 `INITIAL_ASSET`，保留最早一条，其余自动改为 `ASSET_IN`，避免升级旧数据库失败。

### 4.2 资产新增 `ASSET_IN`

定义为系统开始记账以后，新的公共资产流入某成员手中。成员已有初始资产后，后续流入一律使用资产新增。

### 4.3 资产划出 `ASSET_OUT`

本轮明确其业务边界：

> 公共财产退出共同资产体系，而不是普通消费，也不是家族成员之间的代管转移。

典型场景包括：

- 退回当初资金来源者；
- 撤资；
- 经协商后将某笔原本进入公共财产的资金退出共同资产。

后台允许 `ASSET_OUT` 通过 `related_event_id` 关联该持有人此前的 `INITIAL_ASSET` / `ASSET_IN` 流入记录，形成“原始流入 → 退出/退回”的证据链。前台成员自己的资产减少保持简化，不强制选择来源记录，但要求填写说明。

### 4.4 财务调整 `ADJUSTMENT`

定义为盘点、校对、纠错时的账面修正，不代表一次真实收付款。

- 仅后台提供；
- 金额允许正数或负数；
- UI 要求填写调整原因；
- 不应替代普通资产流入、资产划出、消费或内部转账。

## 5. 公共消费简化：取消用户选择 funding_type

前台和后台均不再显示：

- 实际付款人（与经手人重复）；
- `funding_type` 选择；
- “直接支出时选择持有人”。

业务规则统一为：

> 经手人 = 付款人 = 直接支出人。

数据库旧字段 `payer_member_id`、`funding_type`、`holder_member_id` 暂时保留，以保证已经生成的 SQLite 数据可增量升级，不做破坏性重建；**新业务计算不再要求用户提供 funding_type**。

本轮新增 `public_paid_amount_cent`，系统自动拆分一笔消费中实际由经手人代管公共资产承担的部分和需要报销的部分。

例如：

```text
经手人当前代管：¥1000
本次公共消费：¥1200
```

自动得到：

```text
public_paid_amount_cent = ¥1000
reimbursable_amount_cent = ¥200
```

无需用户判断“直接公共资产”还是“个人垫付”。

如果经手人代管余额足够，则全部从其代管公共资产扣减、待报销为 0；余额不足则只使用可用代管金额，差额自动形成待报销。

消费修改时同样重新校验可由公共资产承担的金额，并保证新的应报销金额不能小于已经完成的报销总额。

## 6. 支付/转账渠道标准化

消费、内部转账、报销统一只提供以下渠道：

- 支付宝；
- 微信；
- 银行；
- 现金；
- 其它。

Store 层同步校验，不能仅靠 HTML select 防止构造非法请求。

## 7. 支付/转账凭证

新增通用 `record_attachments`，用于：

- `expense`：支付凭证；
- `transfer`：转账凭证；
- `reimbursement`：报销转账凭证。

规则：

- 一个业务记录支持多个文件；
- 单文件最大 10MB；
- 一次最多 20 个文件，防止异常请求无限占用资源；
- 图片支持 jpg/jpeg/png/gif/webp/bmp/heic/heif；
- 文档支持 PDF、TXT、DOC/DOCX、XLS/XLSX、PPT/PPTX；
- 文件内容计算 SHA-256；
- 磁盘文件名使用 hash 前缀 + 时间戳，不直接信任用户文件名；
- 数据保存在当前 Partition 的 `uploads/evidence/`；
- 数据库保存原始文件名、MIME、大小、SHA-256、上传者等元数据；
- 上传前先完成数量、扩展名和 FileHeader 大小校验；
- 写文件/写附件表在附件保存阶段使用事务及失败清理；
- 下载通过 `/evidence/{id}` Handler，不直接静态暴露目录。

Archive 原有附件仍保持独立逻辑和原开发期限制，本轮没有把信息共享附件强行改为 10MB。

## 8. 前台内部转账

前台不再出现“转出人 / 转入人”两个管理员式下拉框。

当前登录成员只需：

1. 选择方向：`接收自` / `转给`；
2. 选择另一个成员；
3. 输入金额；
4. 选择标准转账渠道；
5. 可选日期时间，留空取当前时间；
6. 可选关联事务与说明；
7. 上传多个转账凭证。

服务端根据 ActorID 和方向自动确定 from/to，客户端不能借前台表单任意指定双方。

后台仍保留完整 A → B 指定能力。

## 9. 报销

报销渠道使用固定五项，并支持多文件转账凭证。

前台报销默认由当前登录成员的代管公共资产支付；后台允许管理员明确指定报销付款持有人。

报销继续保持原账务规则：

- 不得超过该消费剩余待报销额；
- 报销付款持有人的可用代管额必须足够；
- 报销只减少付款持有人代管额和待报销，不二次减少公共财产净额。

## 10. 消费详情 / 编辑页

`/assets/expenses/{id}/edit` 从单纯编辑表单升级为消费详情页：

- 显示消费金额、经手人/付款人；
- 显示无需报销 / 待报销 / 已全额报销状态；
- 显示支付凭证；
- 显示该消费全部报销流水及对应转账凭证；
- 尚有待报销金额时显示“报销”操作；
- 显示 `audit_logs` 中该消费的创建/修改记录；
- 修改前/后 JSON 公开展示，便于家族成员核对历史修正。

## 11. Migration

新增：

```text
migrations/partition/000002_asset_ui_and_evidence.sql
```

该 migration：

- 规范化历史重复初始资产；
- 为 `asset_events` 增加 `related_event_id`；
- 为 `public_expenses` 增加 `public_paid_amount_cent`；
- 按旧 `funding_type` 回填历史消费的 public-paid 金额；
- 为每成员一条有效初始资产增加 partial unique index；
- 创建通用 `record_attachments` 及索引。

本轮没有修改已经执行过的 `000001_init.sql`，以保证用户现有 `data/partitions/p_default/fmlysys.db` 可以直接通过 migration 升级，不要求删库重建。

## 12. 本轮验证

当前执行环境仍无法联网获取仓库及真实 Go Module，因此采用与此前一致的编译隔离方式验证本轮新增代码：

- 新增 Store 业务代码通过 `gofmt`；
- 新增 HTTP Server 路由/handler 在完整接口 stub 下通过 Go 编译；
- `assets.html`、`admin.html`、`expense-edit.html` 与模板 FuncMap 通过 ParseFS 测试；
- 新增 payment channel 单元测试通过；
- 新增凭证文件类型/10MB 限制单元测试通过；
- 使用 Python 标准库 SQLite 对 `000002_asset_ui_and_evidence.sql` 做实际 SQL 执行验证，migration 语法及 ALTER/索引/新表创建成功。

本轮新增测试：

```text
internal/store/assets_v2_test.go
internal/store/evidence_test.go
```

用户 Windows 环境已经生成真实 `go.mod` indirect dependencies / `go.sum`；本轮提交以远端最新分支为 base，不覆盖这些本地真实依赖结果。

正式合并或继续下一阶段前，建议用户本地再次执行：

```bat
scripts\dev-windows.cmd
```

并实际完成：初始资产 → 资产新增 → 部分公共资产消费/自动待报销 → 成员转账 → 报销 → 凭证下载 → 消费审计的完整 SQLite 冒烟测试。

## 13. 当时仍未完成的生产边界

以下条目记录的是上一轮结束时的状态，其中微信成员认证和后台 TOTP 已在后续第 14 节完成：

- 微信正式成员身份与加入审核；
- `/admin` 管理员密码 + Google Authenticator TOTP；
- CSRF；
- 正式 RBAC / Archive ACL；
- 资产事件/内部转账/报销的修改与逻辑撤销 UI；
- 完整 Audit 浏览与筛选；
- 遗产模块；
- 提醒；
- 备份包、Google Drive、外部备份导入及 Partition 切换。

## 14. 微信成员认证、Pending 审核、权限与后台 TOTP

### 14.1 前台成员身份

本轮将前台从开发态固定 `ActorID` 切换为服务器端成员 Session。正式页面 `/`、`/assets`、`/matters`、`/share` 均要求有效成员 Session；本地开发仅在 `FMLYSYS_DEV_AUTH_ENABLED=1` 时提供显式“本地开发身份登录”，正式部署必须关闭。

微信网站扫码流程使用开放平台 `snsapi_login`：

```text
/login/wechat
→ 微信扫码授权
→ /auth/wechat/callback
→ 已绑定 member_id：创建成员 Session
→ 未绑定：进入 /join
```

OAuth 使用随机 `state` Cookie 校验，AppSecret 仅保留在服务端环境变量。微信身份与业务成员分离：业务表继续只引用内部 `member_id`。

### 14.2 Pending 加入家族

新增业务分区表：

```text
wechat_identities
join_requests
member_permissions
member_sessions
```

未知微信身份不会直接看到家族数据，而是获得短期 join token，填写真实姓名与关系后进入 `pending`。后台 `/admin` 显示 Pending 列表，管理员可以：

- 绑定到已有成员；或
- 新建成员；
- 同时勾选该成员权限；
- 审核通过或拒绝。

审核通过后 `wechat_identities.member_id` 指向内部成员；用户重新扫码即可创建正式 Session。拒绝后允许用户再次扫码并重新提交。

### 14.3 成员权限

当前权限粒度：

```text
assets.view
assets.self_change
expenses.create
expenses.edit
transfers.create
reimbursements.create
matters.view
matters.manage
share.view
share.manage
```

后台既可在审核 Pending 时设置权限，也可后续对已有成员修改权限。权限在每次请求时从数据库读取，不把权限长期固化进 Cookie，因此后台修改后无需用户重新登录即可生效。

前台不仅隐藏无权限控件，服务端路由也执行权限检查。普通成员新增公共消费时，经手人/付款人由当前成员 Session 强制绑定，不能通过篡改表单指定他人。`admin` 可见共享资料不再通过普通成员路径返回或下载。

### 14.4 公共资产页面

恢复公开的“公共资产持有人”列表，展示各成员当前代管公共财产总额；仍不展示任何个人真实银行卡、微信、支付宝或现金账户明细。

“我的公共资产变动”从独立大区块改为持有人列表右上角的“登记我的资产变动”折叠入口：桌面端展开为列表角落浮层，移动端展开为正常块级面板，明确提示该入口只用于公共资产流入/退出，不是公共消费。

### 14.5 多文件凭证

再次确认前台三处均使用 `multipart/form-data + multiple`，后端 `ParseMultipartForm` 后读取同名 `evidence` 的全部 `FileHeader`，并逐个交给 `SaveEvidenceFiles` 保存：

- 新增公共消费：支付凭证；
- 与其他成员转账：转账凭证；
- 登记报销：转账凭证。

保持单文件 10MB、一次最多 20 个文件以及既有图片/办公文档白名单。请求解析上限提高到 220MB，避免 20 个接近 10MB 的合法文件在 multipart 解析阶段被过早拒绝。

### 14.6 后台管理员密码 + Google Authenticator

新增 system.db 表：

```text
admin_users
admin_sessions
```

首次管理员不提供匿名 Web setup 页面。若 system.db 尚无管理员，可通过：

```text
FMLYSYS_ADMIN_USERNAME
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD
```

在启动时创建。密码验证成功后：首次进入 TOTP 绑定流程，服务端生成 160-bit Base32 Secret，并生成 `otpauth://` URI 与本地二维码；绑定完成后，以后每次后台登录均为“密码 → Google Authenticator 6 位 TOTP → authenticated session”。

TOTP 实现遵循 RFC 6238：HMAC-SHA1、6 位、30 秒 timestep、允许 ±1 timestep 时钟漂移，并记录最后成功 timestep 防止同一验证码重放。TOTP Secret 使用 AES-256-GCM 加密后存入 system.db；主密钥来自 `FMLYSYS_MASTER_KEY`，未配置时自动生成 `data/system.key`。

后台 Session 与普通成员 Session 使用不同 Cookie 与不同数据库，互不混用。

### 14.7 新 migration 与依赖

新增：

```text
migrations/partition/000003_member_auth_permissions.sql
migrations/system/000002_admin_auth.sql
```

均为增量 migration，不要求删除现有 `data`。

Google Authenticator 绑定二维码由 `github.com/skip2/go-qrcode` 在服务端本地生成，不依赖第三方在线二维码服务；对应模块校验已加入 `go.sum`。

### 14.8 本轮验证与剩余边界

已检查/验证：

- 微信 OAuth state、code 换身份及 Pending 状态机代码路径；
- 成员 Session 使用随机 256-bit token，数据库只保存 SHA-256 token hash；
- TOTP 使用 RFC 6238 官方测试向量；
- TOTP 重放拒绝；
- 后台密码 hash/verify 单元测试；
- 两个新增 SQLite migration 的建表/索引结构；
- 三处凭证均为多文件 HTML 控件并贯通现有多文件后端；
- 后台 Pending/成员权限页面与服务端 handler 字段一致；
- 无权限的事务管理、消费编辑、报销操作在 UI 与路由两层收口。

真实微信扫码仍必须在用户自己的微信开放平台网站应用、已审核回调域名以及真实 AppID/AppSecret 下做端到端测试。本地 `localhost` 只能测试 dev login、权限、Pending 数据层和后台 TOTP。

仍未完成的生产级安全/功能边界包括 CSRF token、登录限流/锁定策略、更完整的安全日志、资产事件/转账/报销修改撤销 UI、遗产、提醒、备份/Google Drive/数据分区切换等；因此当前 Step1 仍应先在受控环境测试后再决定部署范围。

## 15. TOTP 密钥别名与 `data/config.env` 本机配置

### 15.1 Google Authenticator 密钥别名

不同测试环境如果都显示成固定的 `FmlySys:admin`，在 Google Authenticator 中很难判断当前验证码属于本机、测试服务器还是正式环境。因此本轮把“环境区分”放在 OTPAuth 的账号标签层，而不是修改 TOTP Secret 或验证码算法。

绑定页新增“密钥别名”输入框：

- 默认值为当前管理员用户名；
- 可填写例如 `FmlySys 本机测试`、`FmlySys 测试服务器`、`FmlySys 正式环境`；
- 输入后二维码在约 250ms 防抖后自动刷新；
- `/admin/totp/qr?alias=...` 只在处于 `totp_setup` 阶段的管理员 Session 中可用；
- 别名去除首尾空格，最长 80 个字符，并拒绝控制字符；
- 最终二维码仍使用原 TOTP Secret，只把自定义别名传给现有 `OTPAuthURI()` 作为 account label；
- 6 位验证码验证不依赖别名，因此别名不需要成为安全凭据，也无需新增数据库字段或 migration。

这样同一管理员可以在不同部署环境中使用不同可读名称，而不会改变 RFC 6238 的验证语义。

### 15.2 `data/config.env`

为了避免 Windows 本地开发每次启动都重新设置管理员初始化账号/密码和微信开发者环境变量，本轮增加：

```text
data/config.env
```

程序启动时先根据 `FMLYSYS_DATA_DIR` 确定 data 目录，再读取其中的 `config.env`。配置优先级为：

```text
环境变量 > data/config.env > 程序默认值
```

因此：

- 本机测试可以长期把配置写在 `data/config.env`；
- CI、容器或正式部署仍可以用环境变量覆盖；
- `FMLYSYS_DATA_DIR` 自身不能从这个文件读取，因为必须先知道 data 目录才能定位配置文件。

当前可从文件读取的现有配置包括管理员初始化信息、微信 OAuth、Master Key、监听地址、开发身份等 `FMLYSYS_*` 键。用户本轮重点需要的配置为：

```text
FMLYSYS_ADMIN_USERNAME=admin
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=...
FMLYSYS_WECHAT_APP_ID=...
FMLYSYS_WECHAT_APP_SECRET=...
FMLYSYS_WECHAT_REDIRECT_URL=...
```

配置解析支持空行、以 `#` 开头的注释、可选 `export ` 前缀以及单/双引号值；格式错误会让启动明确失败并报告文件和行号，避免配置悄悄失效。

管理员密码仍遵守原来的 bootstrap 边界：仅在 `system.db` 尚无管理员时用于创建首个管理员；已有管理员不会因为配置文件内容变化而在每次启动时被自动重置密码。

### 15.3 Windows 启动脚本

`scripts/dev-windows.cmd` 继续负责确定 `%REPO_ROOT%\data`，并在首次运行发现 `data/config.env` 不存在时自动生成模板。脚本不再主动设置默认 `FMLYSYS_ADMIN_USERNAME`，否则环境变量会无意覆盖配置文件中的管理员用户名。

模板包含管理员初始化账号/密码、微信 AppID/AppSecret/回调地址以及可选 Master Key。`data/` 已被 `.gitignore` 整体排除，所以这些本机 Secret 不会随正常 Git 提交进入仓库，但仍应把 `data/config.env` 当作敏感文件管理。

### 15.4 验证

本轮验证包括：

- `internal/config` 独立 Go 测试通过：确认 `data/config.env` 可以读取、环境变量可以覆盖文件、带空格及 `#` 的双引号密码可以解析；
- 增加 malformed config 测试，错误行不会被静默忽略；
- Google Authenticator 别名归一化增加单元测试；
- 绑定模板使用 Go `html/template` 实际解析和渲染检查，确认 JavaScript 中管理员用户名被正确输出为字符串；
- 不新增数据库 schema，现有 `system.db` 和业务 Partition 不需要 migration。

## 16. 管理员密码移出数据库

### 16.1 目标与边界

本轮按新的安全/可恢复性要求调整管理员密码存储：**管理员密码以及密码摘要均不再以有效数据形式保存在 `system.db` 中**。数据库继续保存管理员身份、启停状态、TOTP Secret（加密）、TOTP 状态和后台 Session，但密码验证改为独立本机文件。

新的密码凭据文件固定为：

```text
data/admin-credentials.enc
```

文件不会保存明文密码。内部先生成 PBKDF2-SHA256 密码摘要，再把包含用户名、摘要、版本号和更新时间的 JSON 凭据整体使用 AES-256-GCM 加密。加密仍复用当前 `data/system.key` 或 `FMLYSYS_MASTER_KEY` 派生的主密钥。

这意味着即使查看 `admin-credentials.enc` 原始内容，也看不到用户名、PBKDF2 标记或密码摘要；密码比对时由服务端先解密凭据，再执行原有 PBKDF2 常量时间比较。

### 16.2 为什么不直接增加“清空 password_hash”的 migration

现有 `migrations/system/000002_admin_auth.sql` 中已经存在 `password_hash TEXT NOT NULL`。不能简单新增 migration 在数据库打开时立刻把它清空，因为 migration 会早于管理员认证服务初始化执行：如果先清空，旧安装的密码摘要还没有机会搬到文件中，会造成管理员不可登录。

因此本轮采用启动期安全迁移：

1. 启动并打开既有 `system.db`；
2. 如果 `data/admin-credentials.enc` 不存在，检查旧 `admin_users.password_hash`；
3. 如果 `data/config.env` 临时提供了新密码，则优先使用新密码生成摘要，等同于一次忘密重置；否则直接迁移旧 PBKDF2 摘要；
4. 使用临时文件 + rename 写入并确认 `admin-credentials.enc` 成功；
5. 最后才把数据库中的旧 `password_hash` 更新为空字符串。

旧列暂时仅作为 schema 兼容壳保留，新代码创建管理员时从第一天起只向该列写空字符串。以后如果整体重建 system schema，再考虑物理删除该历史列；当前不为删列承担 SQLite 表重建风险。

### 16.3 忘记密码后的恢复

`FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD` 的语义扩展为“首次创建 / 本机密码重置”。如果管理员已经存在、加密凭据文件也存在，但 `data/config.env` 中临时填写的新密码与当前摘要不一致，启动时会：

- 为新密码生成新的 PBKDF2-SHA256 摘要；
- 重写 `data/admin-credentials.enc`；
- 删除已有后台 Session，避免旧已登录 Session 在密码重置后继续存活；
- 保留 `system.db`、TOTP Secret、TOTP 绑定和全部家族数据。

所以忘记密码时不需要删除数据库。操作流程为：

```text
编辑 data/config.env
→ FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=新的至少10位密码
→ 重启 FmlySys
→ 用新密码 + 原 Google Authenticator 验证码登录
→ 确认成功后把该配置重新清空
```

如果凭据文件丢失但数据库中还有旧版本密码摘要，程序会自动迁移；如果凭据文件和旧摘要都不存在，则启动会明确提示在 `data/config.env` 临时设置新密码，而不是要求删库。

### 16.4 文件一致性与 Windows

凭据写入使用同目录临时文件、flush/sync 后再 rename，尽量避免进程中断产生半文件。考虑 Windows 对已有目标文件 rename 替换的行为差异，第一次 rename 失败时只删除旧的 `admin-credentials.enc` 后重试，不触碰 `system.db` 或其它 data 文件。

Windows 开发脚本同步显示：

```text
Admin credentials: <repo>\data\admin-credentials.enc
```

并在 `data/config.env` 模板中明确说明密码行只用于首次创建或重置，成功后建议清空。

### 16.5 验证

`internal/adminauth/adminauth_test.go` 新增覆盖：

- 新管理员创建后数据库 `password_hash` 必须为空；
- 正确密码可以通过加密凭据文件完成验证，错误密码拒绝；
- `admin-credentials.enc` 原始密文中不出现明文密码、`pbkdf2-sha256` 或 JSON `password_hash` 字段；
- 修改配置密码并再次执行启动初始化逻辑，可在不删除数据库的情况下重置密码；
- 旧数据库中的 PBKDF2 `password_hash` 可自动迁移到加密文件，迁移后数据库字段被清空，旧密码仍可正常验证。

本轮没有新增 SQLite schema migration，现有 `data/system.db`、业务 Partition 和 Google Authenticator 绑定可以原地沿用。
