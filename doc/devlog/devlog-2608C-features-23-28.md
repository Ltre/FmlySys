# dev-2608C-step1 开发记录

日期：2026-08-21  
分支：`dev-2608C-step1`

> 历史第 1～22 节已原样保存在同目录 `devlog-2608C-features-01-22.md`。本文件从第 23 节继续记录后续开发，历史 Git 提交中也保留原完整日志。

## 23. 前台 Passkey / 微信二选一认证

### 23.1 正式前台登录入口

正式前台认证收敛为两条并列路径：

```text
Passkey（WebAuthn / FIDO2）
或
微信扫码登录
```

`FMLYSYS_DEV_AUTH_ENABLED=1` 时仍可显示本地开发身份登录，但它只用于开发调试，不属于正式认证方式；正式部署必须关闭。

Passkey 使用国际通用 WebAuthn / FIDO2 标准，服务端接入 `github.com/go-webauthn/webauthn v0.12.3`。注册时要求可发现凭据（discoverable / resident credential）和用户验证（user verification），因此已经绑定 Passkey 的成员以后登录时不需要先输入姓名、账号或手机号，浏览器/系统可以直接选择属于当前 RP 的通行密钥。

Passkey 验证成功后不建立另一套用户体系，而是复用现有成员身份与 Session：

```text
Passkey 验证成功
→ 解析到内部 member_id
→ CreateMemberSession(member_id)
→ fmly_session
→ 进入现有前台
```

微信扫码登录及现有 Pending / 管理员审核流程保持不变，因此两种认证最终都落到同一个内部 `member_id` 与权限体系。

### 23.2 首次绑定、多设备与强制备注

Passkey 绑定只能发生在已经存在有效成员 Session 的页面中。也就是说，一个尚未被系统识别的新成员不能仅凭“创建一把新 Passkey”直接获得家族数据访问权。

首次使用通常为：

```text
微信扫码
→ 已绑定成员直接登录，或未知身份提交加入申请
→ 管理员审核/绑定成员
→ 成员进入“通行密钥”
→ 创建 Passkey
```

开发环境也可以先通过本地开发身份进入后绑定测试 Passkey。

一个成员允许绑定多把 Passkey。例如同一成员可以分别拥有：

```text
张三 / 138****1234 / iPhone 16
张三 / 138****1234 / Galaxy S23 Ultra
张三 / 家中 Windows 电脑
```

系统不要求 Apple、Samsung 或其它凭据管理器之间同步同一把私钥；它们可以分别为同一个 FmlySys 成员注册不同 credential。

按本轮补充要求，**绑定 Passkey 时备注必填**。服务端规则：

- 去除首尾空格后不能为空；
- 最多 160 个 Unicode 字符；
- 拒绝控制字符；
- UI 明确提示可填写“姓名 / 手机号 / 设备名称”等方便管理员辨认的信息；
- 备注只用于人工识别，不作为认证主键，不参与签名验证。

每个 `(member_id, rp_id)` 使用随机生成的 32-byte 不透明 `user_handle`。姓名、手机号或备注都不会被拿来充当 WebAuthn user handle。

### 23.3 服务端保存内容与隐私边界

FmlySys 只保存完成 WebAuthn 验证所需的公开凭据和状态，包括：

- credential ID；
- 公钥凭据 JSON；
- authenticator flags / counter 等必要状态；
- RP ID；
- 随机 user handle；
- 用户填写的识别备注；
- 创建时间、最近使用时间。

FmlySys 不接收、不保存：

- Face ID 数据；
- Touch ID / 指纹数据；
- Android 指纹或人脸模板；
- 设备锁屏 PIN / 密码；
- Passkey 私钥。

生物识别或锁屏验证发生在用户设备本地，服务端只验证标准 WebAuthn 签名。

### 23.4 数据库与 ceremony

新增增量 migration：

```text
migrations/partition/000005_passkeys.sql
```

新增：

```text
passkey_users
passkey_credentials
passkey_ceremonies
```

其中：

- `passkey_users` 保存成员在特定 RP 下的随机 user handle；
- `passkey_credentials` 保存成员可拥有的多把公钥凭据及备注；
- `passkey_ceremonies` 保存短期注册/登录 challenge session。

注册与登录 ceremony 有效期为 5 分钟；浏览器 Cookie 保存随机原始 token，SQLite 只保存 token hash。ceremony 完成取用时即从数据库删除，并校验 ceremony 类型和 RP ID，使同一个 token 不能重复使用或跨 RP 使用。

### 23.5 前台与后台管理

前台导航新增“通行密钥”，进入：

```text
/passkeys
```

成员可以：

- 填写强制备注并绑定新 Passkey；
- 查看自己已绑定的多把 Passkey；
- 查看每把 Passkey 的备注、RP、绑定时间、最近使用时间；
- 删除自己的 Passkey。

后台导航新增：

```text
/admin/passkeys
```

管理员可查看：

- 成员 ID / 姓名；
- 用户绑定时填写的备注；
- RP 域名；
- 绑定时间；
- 最近使用时间。

因此即使一个成员拥有多部手机或多套凭据管理器，管理员仍可通过备注判断对应人员和设备。

成员执行现有智能删除时，无论最终是硬删除还是因历史业务关联而软删除，都会同步撤销该成员的 Passkey ceremony、credential 和 user handle，避免已删除成员继续使用旧 Passkey 登录；历史账务与业务事实仍按第 21 节既有规则保留。

### 23.6 RP、域名与 HTTPS

WebAuthn / Passkey 与 RP 域绑定。本轮不新增另一份固定域名配置，而是从实际请求 `Host` 派生 RP ID，并从 TLS 或受支持的 `X-Forwarded-Proto` 得到 origin。

正式环境要求 HTTPS，例如：

```text
https://fmly.miku.us
```

在该域名创建的 Passkey 对应 RP：

```text
fmly.miku.us
```

普通局域网 HTTP，例如：

```text
http://10.0.0.27:8080
```

不是标准 WebAuthn 安全上下文，因此 Passkey 接口明确拒绝该场景，前端也会隐藏 Passkey 按钮并提示使用微信扫码登录。`localhost` / 回环地址保留浏览器标准开发例外，方便本地调试。

这与此前“FmlySys 页面允许任意 IP / 域名访问”并不冲突：普通业务页面仍可通过局域网 IP 访问，只是 Passkey 这项安全能力必须遵守浏览器 WebAuthn 的 HTTPS 安全上下文要求。

### 23.7 前端标准兼容

`web/static/passkeys.js` 直接使用浏览器标准：

```text
PublicKeyCredential
navigator.credentials.create()
navigator.credentials.get()
```

优先使用浏览器提供的 JSON options 解析 API；旧实现缺少该辅助 API 时使用 Base64URL / ArrayBuffer 兼容转换。

登录页运行时判断 WebAuthn 与安全上下文：

- 支持 Passkey + HTTPS：显示“使用 Passkey 登录”；
- 不支持 Passkey：隐藏该按钮并提示使用微信；
- 非安全上下文：隐藏该按钮并提示使用微信；
- 微信仍保持独立正式入口。

因此 Passkey 实现不是 Google 登录，也不绑定 Google Play 服务。对支持标准 WebAuthn / Passkey 的国行 Apple 设备，使用的是系统/浏览器标准能力；不同厂商设备可以给同一成员分别注册自己的 credential。

### 23.8 验证与边界

本轮已完成的静态/隔离验证：

- `000005_passkeys.sql` 已检查表结构、唯一约束、外键及成员删除级联关系；
- `web/static/passkeys.js` 已通过 `node --check` JavaScript 语法检查；
- 新增 RP/origin 测试覆盖 HTTPS 域名、反向代理 HTTPS、localhost HTTP 和局域网 HTTP 拒绝；
- 新增 Store 测试覆盖备注归一化、随机 32-byte user handle 稳定性、credential 保存读取、ceremony 一次性消费和成员自删 Passkey；
- Passkey 删除已接入既有成员删除认证状态清理；
- 登录完成继续复用现有成员 Session 与权限模型，没有放宽任何业务权限。

当前执行环境无法从 `github.com` / Go Module 源完整拉取依赖，因此没有声称已经运行全仓 `go test ./...`；真实国行 iPhone、Galaxy、微信扫码以及反向代理 HTTPS 的端到端验证仍应在实际部署环境完成。

本 migration 为增量升级，不要求删除或重建现有 `data`。

## 24. Passkey 独立登录身份、手机号定位与跨设备找回

日期：2026-08-22  

> 本节修正并取代第 23 节中“必须先有成员 Session 才能绑定 Passkey”“Passkey 直接绑定 member_id”的身份模型。第 23 节保留为历史开发记录，不回写删除。

### 24.1 需求重新确认

本轮把 Passkey 的业务语义固定为：

```text
Passkey 本身可以在未登录状态下创建一个全新的 FmlySys 登录身份；
Passkey 是该身份认证的充分必要依据；
手机号只是用户熟悉的“身份定位标记”，不能单独认证、重置或接管身份。
```

因此不再采用：

```text
微信登录 → 已有 member_id → 再给 member_id 绑定 Passkey
```

而采用：

```text
Passkey 登录身份 → 可独立存在 → 管理员再关联到 member_id
```

微信扫码仍是与 Passkey 并列的另一条正式登录路径，两者互不构成前置条件。

### 24.2 为什么保留手机号，但不把手机号当 Passkey 主键

用户在设备 A 首次创建身份时填写手机号与识别备注，例如：

```text
手机号：13800138000
备注：张三 / 138****1234 / iPhone 16
```

系统会把中国大陆 11 位手机号统一规范为 `+86...`，同时接受 `+86`、空格、短横线和括号等常见输入形式。手机号在一个数据分区内唯一，用于回答“我要找回哪个 FmlySys 登录身份”。

但底层 WebAuthn `userHandle` 仍然使用随机生成的 32-byte 不透明值，不把手机号、姓名或备注直接写入认证主键。这样手机号泄露、换号或被别人知道，都不会直接取得登录权限。

### 24.3 设备 A 首次创建全新 Passkey 登录身份

登录页新增独立的“首次创建 Passkey 身份”流程：

```text
未登录
→ 输入手机号 + 身份备注
→ 服务端确认手机号尚未创建过 Passkey 身份
→ 生成随机 userHandle
→ navigator.credentials.create()
→ WebAuthn 注册验证完成
→ 原子创建 passkey_login_identity + 首把 credential
→ 创建 fmly_passkey_identity Session
→ 进入 /passkey/account
```

注册完成前不会提前持久化一个空身份，避免用户取消系统 Passkey 面板后留下没有凭据的孤儿账号。

### 24.4 设备 B 找回设备 A 创建的同一身份

设备 B 的用户只需要记住自己最熟悉的手机号：

```text
设备 B
→ 输入设备 A 创建身份时的手机号
→ FmlySys 定位到原 Passkey 登录身份
→ 服务端只下发该身份已有 credential 的 WebAuthn assertion
→ navigator.credentials.get()
```

此时分两种情况：

1. Passkey 已经由 iCloud、Google Password Manager、Samsung Pass 或其它凭据提供器同步到 B：直接用 B 上已有凭据验证；
2. B 没有本地凭据，但 A 仍持有：浏览器/系统可选择“使用其他设备”，显示标准 FIDO 跨设备二维码，由设备 A 扫码并用 A 上的 Passkey 完成验证。

只有 assertion 验证成功后，FmlySys 才创建 `fmly_passkey_identity` 登录 Session。**仅输入正确手机号不会产生任何登录态。**

### 24.5 让设备 B 以后可以独立登录

设备 B 通过设备 A 的已有 Passkey 完成找回后，会进入 `/passkey/account?recovered=1`。页面明确提示可立即“给当前设备新增 Passkey”。

新增流程仍然绑定到同一个 Passkey 登录身份：

```text
Identity P123
├── Credential A：设备 A
└── Credential B：设备 B
```

以后 B 输入同一手机号即可直接使用 Credential B 登录，不必每次借设备 A。

为降低 Session 被盗后被恶意增加新认证凭据的风险，只有“刚刚通过 Passkey 验证”的身份 Session 才能新增 Passkey；当前实现 freshness 为 10 分钟。超过后要求重新使用已有 Passkey 登录，再执行新增设备操作。

### 24.6 新数据模型

新增增量 migration：

```text
migrations/partition/000006_passkey_login_identities.sql
```

新增：

```text
passkey_login_identities
passkey_login_users
passkey_login_credentials
passkey_login_ceremonies
passkey_login_sessions
```

职责：

- `passkey_login_identities`：FmlySys 独立登录主体，保存唯一手机号定位标记、身份备注及可选 `member_id`；
- `passkey_login_users`：按 RP 保存随机 WebAuthn user handle；
- `passkey_login_credentials`：同一身份的一把或多把 Passkey 公钥凭据；
- `passkey_login_ceremonies`：5 分钟一次性 create/login/add ceremony；
- `passkey_login_sessions`：Passkey 验证后建立的登录 Session，并记录最近一次强认证时间。

旧 `000005_passkeys.sql` 保留用于历史 migration 兼容，但新的公开登录流程由 `WithPasskeyIdentities` 外层路由接管；旧的“成员登录后绑定 Passkey”POST 接口被显式返回 `410 Gone`，避免继续生成已经不参与新登录模型的旧式凭据。

### 24.7 管理员识别与家族成员关联

新建 Passkey 身份可以先独立存在，不自动获得家族数据权限。`/admin/passkeys` 改为展示：

- 身份 ID；
- 用户填写的手机号；
- 身份备注；
- 该身份下所有 Passkey 的设备备注、RP、创建时间、最近使用时间；
- 当前关联的家族成员。

管理员可根据“姓名 / 手机号 / 设备”等备注辨认用户，再把该 Passkey 登录身份关联到一个已有 `member_id`。关联完成后，该用户下一次以 Passkey 登录（或刷新仍有效的 Passkey 身份页）即可建立正常 `fmly_session`，进入现有权限体系。

手机号不参与管理员关联后的授权判断；真正认证仍由 Passkey assertion 完成。

### 24.8 登出与安全边界

正常 `/logout` 现在由外层 Passkey 身份路由接管，同时删除：

```text
fmly_session
fmly_passkey_identity
```

避免用户表面退出家族系统后，仍可凭残留的 Passkey 身份 Session 立即恢复成员 Session。

明确不实现：

```text
输入手机号
→ 短信验证码
→ 重置/新增 Passkey
```

因为一旦允许手机号本身恢复认证凭据，Passkey 就不再是该登录身份的必要依据。当前如果所有 Passkey 都丢失，只能另行设计管理员人工恢复流程；本轮不把手机号升级为认证因子。

### 24.9 验证

本轮在可用执行环境中完成：

- 新增 Go 文件全部经过 `gofmt`；
- 新增 Go 文件使用标准库 `go/parser` 做语法解析检查；
- `web/static/passkeys.js` 通过 `node --check`；
- 新增 Store 测试覆盖手机号规范化、身份创建、手机号定位同一身份、重复手机号拒绝、身份 Session、管理员绑定成员、ceremony 一次性消费；
- 旧 RP / HTTPS 测试继续保留。

当前执行环境无法解析 `github.com`，Go Module 缓存也没有 `go-webauthn` / SQLite 依赖，因此无法真实执行全仓 `go test ./...`。提交前不把未执行的测试描述为已通过；实际部署后仍需在 HTTPS 域名上做设备 A → 设备 B 的 WebAuthn/FIDO 跨设备端到端验证。

## 25. 资产表单、消费提交、凭证预览与报销流水修正

日期：2026-08-23  

### 25.1 资产金额误报为空

根因在前端统一异步表单增强：原实现无论表单有没有附件，都使用 `FormData` 发送，因此普通资产登记也变成 `multipart/form-data`。而资产变动处理器只调用 `ParseForm()`，没有解析 multipart 字段，最终 `amount` 为空并触发“金额不能为空”。前台“登记我的资产变动”和后台“登记公共资产”因此同时复现。

本轮对资产工作流的请求编码做分流：

```text
没有实际选择附件
→ application/x-www-form-urlencoded

选择了附件
→ multipart/form-data
```

同时服务端新增统一的 `parseWorkflowRequest`，兼容 urlencoded 与 multipart，因此旧页面缓存或其它客户端即使仍用 multipart 提交无附件表单，也能正确读到金额等字段。

### 25.2 新增公共消费长时间 Pending 与半完成状态

旧消费保存流程为：

```text
CreateExpenseAuto()
→ 提交消费记录事务
→ SaveEvidenceFiles()
→ 另开事务保存凭证
```

这意味着消费主记录可能已经提交，而凭证文件/附件元数据仍未处理完；用户刷新后能看到消费记录，却无法确定整个接口是否完整成功。并且统一使用 multipart 后，即使没有附件，`WithRequestDeadline` 也会把该请求当成大文件上传而跳过 15 秒数据库请求超时保护。

本轮改为：

```text
无附件
→ urlencoded 请求，恢复正常 request deadline

有附件
→ 先完成文件校验与磁盘暂存（不占用 SQLite 写事务）
→ 单一 SQLite 事务写入消费主记录 + 消费审计 + 凭证元数据 + 凭证审计
→ 事务成功后才视为消费保存完成
```

因此消费记录不会再在数据库单元尚未完整提交时提前表现为“已经保存”。文件 I/O 放在写事务之前，避免上传/哈希过程中长期占用 SQLite writer。单文件仍保持 10MB、一次最多 20 个凭证的既有约束。

### 25.3 凭证文件改为可预览优先

`GET /evidence/{id}` 不再对所有凭证强制 `Content-Disposition: attachment`。

以下类型改为浏览器内联预览：

- 图片：JPG/JPEG/PNG/GIF/WebP/BMP/HEIC/HEIF；
- 视频：MP4/WebM/MOV/M4V；
- 音频：MP3/M4A/AAC/WAV/OGG/FLAC；
- PDF；
- TXT（显式使用 `text/plain; charset=utf-8`）。

Word/Excel/PPT 等不适合浏览器直接预览的类型仍保持下载。消费、内部转账、报销流水中的凭证链接默认新标签页打开；用户仍可使用浏览器右键“另存为/下载链接”下载可预览文件。凭证上传允许列表同步增加常见视频、音频扩展名。

### 25.4 后台消费流水增加快捷报销

后台“登记报销”区域补充稳定定位点，并对消费流水中 `PendingCent > 0` 的记录在“报销”单元格增加“报销”按钮。

点击后：

```text
自动把该 expense_id 写入“选择待报销消费”
→ 页面滚动到登记报销区域
→ 高亮该区域
→ 焦点落到待报销消费 select
```

复用前台已经存在的 `enhanceReimbursementJumps()` 交互，前后台行为保持一致。

### 25.5 资产变动流水增加“消费报销”并统一人类可读类型

“消费报销”是消费/报销业务事实对持有人代管余额造成的减少，不开放为“登记公共资产”的手工事件类型。本轮后台流水改用完整的余额变动视图：

- `INITIAL_ASSET` → `初始资产`；
- `ASSET_IN` → `资产新增`；
- `ASSET_OUT` → `资产减少`，金额按负数展示；
- `ADJUSTMENT` → `财务调整`；
- 消费发生时自动使用代管资金 → `消费报销`，负数；
- 后续登记报销 → `消费报销`，负数。

消费报销流水从 `public_expenses.public_paid_amount_cent` 与 `reimbursements` 派生，不向 `asset_events` 重复写入数据，因此不会重复扣减余额。前台原本已经以相同语义合成消费报销行，本轮将后台也统一到这一口径。

### 25.6 验证与边界

本轮完成：

- 新增 Go 文件 `gofmt`；
- 使用标准库 `go/parser` 对新增 Go 文件及启动入口做语法解析；
- `web/static/asset-workflow.js` 通过 `node --check`；
- 修改后的共享导航模板通过 `html/template` 解析；
- 新增测试覆盖 urlencoded/multipart 金额解析、媒体凭证允许列表、内联预览类型判断、资产流水人类可读类型及消费报销负数语义。

当前执行环境无法解析 `github.com` 并补齐 Go Module 依赖，因此没有声称已运行全仓 `go test ./...`。本轮不需要新增数据库 migration。

## 26. 后台报销流水 SQL 修复与 Passkey 登录入口收敛

日期：2026-08-23  

### 26.1 后台 `payer_holder_meb_id` SQL 错误

问题由第 25 节新增“消费报销”合成流水时引入。`reimbursements` 表真实字段为：

```text
payer_holder_member_id
```

但 `AssetMovementsDetailed()` 的后续报销查询误写为：

```text
payer_holder_meb_id
```

因此 `/admin` 读取完整资产变动流水时 SQLite 直接返回 `no such column`。本轮修正为真实字段名；既有回归测试中使用真实 schema 字段，后续只要测试真正执行即可覆盖该类拼写错误。

### 26.2 Passkey Session 正式作为前台登录态

此前虽然创建/验证 Passkey 后已经生成 `fmly_passkey_identity` Session，但首页 `/` 仍只经过旧 `member()` 中间件，只认可 `fmly_session`。因此出现：

```text
Passkey 已成功创建或验证
→ 技术上已有 Passkey identity Session
→ 前台首页仍判断“未登录”
```

这与既定语义“Passkey 本身是该登录身份的充分必要认证依据”不一致。

本轮新增 Passkey-aware 前门：

```text
GET /
→ 优先复用有效 fmly_session
→ 否则检查 fmly_passkey_identity
```

若 Passkey 身份已经关联 `member_id`：

```text
Passkey identity
→ 读取对应 active member + member permissions
→ 自动补建 fmly_session
→ 直接渲染正常首页总览
```

若尚未关联家族成员：

```text
Passkey identity
→ 仍视为“已登录”
→ 进入首页总览壳
→ 明确显示“Passkey 登录身份已建立、等待管理员关联家族成员”
→ 不展示公共资产 / 家族事务 / 共享资料
→ 不自动授予任何业务权限
```

由此将“认证成功”和“获得家族业务权限”分开：Passkey 足以建立登录态，但家族数据授权仍必须落到已关联成员及其权限。

### 26.3 创建 / 找回 Passkey 后回首页

Passkey 创建完成和已有 Passkey 验证找回完成后，不再把用户强制送到：

```text
/passkey/account?created=1
/passkey/account?recovered=1
```

成功响应统一改为进入：

```text
/
```

`/passkey/account` 回归为“用户个人信息 / Passkey 管理页”，不再承担登录后的默认落地页。

### 26.4 前台 Header 的个人信息入口

前台 Header 原先单独展示“通行密钥”主菜单，同时用户名只是不可点击文本。本轮调整为：

- 移除 Header 主导航中的“通行密钥”入口；
- 右上角用户名改为“用户个人信息”链接，统一进入 `/account`；
- 当前存在 Passkey identity Session 时，`/account` 进入 `/passkey/account`；
- 普通成员会话没有 Passkey identity 时，`/account` 进入现有 Passkey 说明页。

尚未关联成员的 Passkey 身份首页也提供相同的右上角个人信息入口。

### 26.5 验证与边界

本轮完成：

- 修正 `AssetMovementsDetailed()` 的后续报销 JOIN 字段拼写；
- 新增 Passkey 成功响应重写测试，验证成功响应跳转 `/` 且 `Set-Cookie` 不丢失；
- 新增失败响应保持原状态码/正文的回归测试；
- 新增 Go 文件及启动入口执行 `gofmt`；
- 新增独立的未关联 Passkey 首页模板，并检查模板字段与现有数据结构一致；
- 未新增数据库 migration。

当前执行环境仍无法解析 `github.com`，因此没有声称已完整运行 `go test ./...`。实际部署后重点验证：后台 `/admin` 正常加载、Passkey 创建后进入首页、已关联 Passkey 身份可直接看到正常首页、未关联身份进入受限首页、右上角用户名可进入个人 Passkey 页面。

## 27. 修复 Passkey 前门导致的 `/login` 无限重定向

日期：2026-08-23  

### 27.1 现象

第 26 节上线后，访问首页会跳到 `/login`，而 `/login` 本身继续重定向到 `/login`，浏览器最终报：

```text
ERR_TOO_MANY_REDIRECTS
```

该问题与 Passkey Cookie 内容本身无关，也不需要把“删除 Cookie”作为修复方案。

### 27.2 根因

第 26 节在外层 `http.ServeMux` 注册了：

```go
mux.HandleFunc("GET /", s.passkeyAwareDashboard)
```

项目使用 Go 1.23。新版 `net/http.ServeMux` 中，路径模式 `/` 是子树模式；增加 `GET` 方法限定后，`GET /` 仍会匹配所有未被同一 mux 中更具体模式截获的 GET 子路径，而不是只匹配网站根路径。

因此外层路由实际上形成：

```text
GET /
GET /login
GET /static/app.css
GET /assets
GET /admin
...
        ↓
全部进入 passkeyAwareDashboard
```

当用户没有有效前台 Session 时，`passkeyAwareDashboard` 会重定向到 `/login`；但新请求 `GET /login` 又再次被同一个首页处理器截获，于是形成：

```text
/login
→ passkeyAwareDashboard
→ /login
→ passkeyAwareDashboard
→ /login
→ ...
```

### 27.3 修复方式

首页前门改为 Go 1.22+ ServeMux 支持的精确根路径模式：

```go
mux.HandleFunc("GET /{$}", s.passkeyAwareDashboard)
```

`{$}` 只匹配路径结尾，因此 `GET /{$}` 只处理真正的 `/`。

其余路径继续交给下层现有 Router：

```text
/login          → 原 loginPage
/static/...     → 原静态文件处理器
/assets         → 原成员权限路由
/admin          → 原后台路由
/account        → Passkey 前门显式个人信息入口
```

这样保留第 26 节“Passkey identity Session 可直接构成前台登录态”的设计，同时消除外层 mux 对整站 GET 请求的误拦截。

### 27.4 回归测试

在 `passkey_frontdoor_fixes_test.go` 新增路由范围测试：构造一个不依赖 Store 的下层 handler，并验证：

```text
GET /login
GET /static/app.css
GET /assets
GET /admin
```

都必须透传到下层 handler。

该测试针对本次真实根因；如果以后误把 `GET /{$}` 改回 `GET /`，上述请求会被 `passkeyAwareDashboard` 抢占，测试会直接失败。

### 27.5 验证与边界

本轮完成：

- `go.mod` 已确认项目使用 Go 1.23，支持 `{$}` 精确根路径模式；
- 修改后的 Go 文件和测试已执行 `gofmt`；
- 新增非根 GET 透传回归测试；
- 不修改 Passkey Session、成员 Session 或 Cookie 数据结构；
- 不新增 migration。

当前执行环境仍无法从 GitHub 拉取缺失 Go Module，因此不声称已执行完整 `go test ./...`。本次修复只收窄外层首页路由的匹配范围，不改变已有认证和授权语义。

## 28. 后台异步普通表单 multipart 兼容修复

日期：2026-08-23  

### 28.1 现象与共同根因

后台出现两类看似无关、实际同源的问题：

- `/admin/passkeys/{id}/bind` 明明选择了成员，仍返回“成员 ID 无效”；
- `/admin/members` 明明填写了成员姓名，仍返回“成员姓名不能为空”。

浏览器抓包确认两者都带 `X-Fmly-Async: 1`，并由全局异步表单增强发送为 `multipart/form-data`。对应旧 handler 使用 `r.ParseForm()` 读取普通字段；Go 的 `ParseForm()` 不负责解析 multipart body，因此 `member_id`、`name`、`relation`、`permissions` 等字段在 handler 看来为空。

这不是 Passkey 关联逻辑或成员数据库校验本身的问题，而是统一表单传输格式与旧 handler 解析方式不一致。相同风险还覆盖成员权限、加入申请审核等由全局异步表单增强接管的普通 POST。

### 28.2 统一兼容层

新增 `WithAsyncMultipartFormCompatibility`，放在统一请求链中，而不是分别给每个业务 handler 打补丁。

处理条件收敛为：

```text
X-Fmly-Async: 1
+ multipart/form-data
+ Content-Length > 0 且 <= 1 MiB
```

兼容层先解析小型 multipart。若请求只有普通字段、没有文件，则把全部字段重新编码为：

```text
application/x-www-form-urlencoded
```

并重置 `Form/PostForm/MultipartForm`，让下游原有 `ParseForm()` 按其既有设计重新解析。多值字段不会丢失，例如成员权限的多个 `permissions` 会按原顺序保留。

因此用户抓包中的：

```text
member_id=1
```

和：

```text
name=...
relation=...
permissions=assets.view
permissions=assets.self_change
...
```

都会被旧 handler 正常读取。

### 28.3 文件上传不被误改写

如果小型 multipart 中实际存在文件 part，兼容层保持 multipart，不转换为 urlencoded；已经解析得到的 `MultipartForm` 继续交给下游原上传处理器使用。

超过 1 MiB 的 multipart 不在此兼容层预解析，继续由消费凭证、共享附件等各自原有上传流程、大小限制和文件校验负责。

这样避免为了修普通后台表单而绕过或削弱已有上传限制。

### 28.4 请求 deadline 恢复

兼容层挂载在 `WithRequestDeadline` 外层、`WithEnhancedFormResponses` 内层。字段型 multipart 被规范化成 urlencoded 后再进入 request-deadline middleware，因此不再因为浏览器使用 FormData 就被错误当成“大文件上传”而跳过 15 秒普通数据库请求超时保护。

真正包含文件的请求仍保持 multipart，因此继续按原规则排除上传耗时，不受普通数据库 deadline 误伤。

### 28.5 验证与边界

本轮完成：

- 新增兼容层和回归测试均执行 `gofmt`；
- 用仅依赖 Go 标准库的隔离测试真实执行 `go test`，覆盖：字段型 multipart 转为 urlencoded；`member_id`、中文 `name`、重复 `permissions` 均可由 `ParseForm()` 读出；带文件的 multipart 保持原格式并能继续读取文件；
- 启动入口接入兼容层，顺序为增强响应 → multipart 兼容 → request deadline → 业务路由；
- 不修改 Passkey/成员数据模型，不新增 migration。

当前执行环境仍无法从 GitHub 拉取项目缺失依赖，因此不声称已执行全仓 `go test ./...`；但本轮新增兼容层及其标准库隔离测试已实际运行通过。