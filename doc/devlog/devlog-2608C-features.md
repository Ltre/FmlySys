# dev-2608C-step1 开发记录

日期：2026-08-24  
分支：`dev-2608C-step1`

> 历史第 1～22 节保存在 `devlog-2608C-features-01-22.md`；历史第 23～28 节本轮归档到 `devlog-2608C-features-23-28.md`。本文件继续记录当前开发。

## 29. Passkey 单入口、资金写入原子化、自动报销明细、快速记录与新增流水定位

日期：2026-08-24

### 29.1 本轮逐项核对结论

以远端 `dev-2608C-step1` 的 `29100321e791c2f0669c73677b61e17573c06e5b` 为起点重新核对用户列出的 1～8 项，而不是把上一轮尚未提交的本地设计视为完成。

核对结果：

1. `/login` 仍分成“找回已有 Passkey 身份”和“首次创建 Passkey 身份”两套表单，未满足单视图要求；
2. 消费保存已有“消费 + 凭证元数据单事务”的部分修正，但仍缺少统一的短 DB 写入上限；
3. 消费详情只展示后续 `reimbursements`，没有把自动使用经手人代管公共资产的部分显示成一条可读报销流水；
4. 报销仍是先提交 reimbursement、再另开事务保存凭证，存在半完成状态；
5. 内部转账同样仍是主记录与凭证两段提交；
6. `/quick-money-note` 与标准化入库工作流不存在；
7. 前台资产变动仍使用普通写入路径，没有与其它资金写入统一的短等待边界；
8. 成功提交后没有“新增记录 ID → 对应流水行 → 滚动、焦点和短暂高亮”的闭环。

因此本轮按同一资金工作流整体完成，不把上述任何一项标记为“此前已经完成”。

### 29.2 Passkey 登录页收敛为一个产品视图

登录页只保留一个 Passkey 区域：

```text
手机号（必填）
[ 使用Passkey身份 ]
```

第一次点击后，前端调用：

```text
POST /auth/passkey/identity/resolve
```

服务端只回答该规范化手机号是否已经存在 Passkey 登录身份。

如果存在：

```text
手机号
→ 定位原 Passkey identity
→ /auth/passkey/login/options
→ navigator.credentials.get()
→ 已有 Passkey assertion 成功
→ 恢复同一 FmlySys 登录身份
```

如果不存在：

```text
手机号
→ 页面原地展开“身份备注”
→ 备注变为必填
→ 再次点击同一个“使用Passkey身份”
→ WebAuthn create
→ 创建新的 Passkey identity
```

因此用户不再需要理解“找回”和“创建”两个技术入口。

手机号继续只承担身份定位，不成为认证因子；知道手机号仍然不能创建已有身份的 Session、不能重置已有凭据。

### 29.3 设备 B 与设备 A 的跨设备 Passkey 边界

网页无法安全地枚举“本机是否保存某个指定 Passkey”，也没有标准 Web API 允许 FmlySys 导出或复制 Passkey 私钥到另一台设备。因此没有实现“FmlySys 自己把私钥/公钥副本通过二维码、蓝牙或网络复制过去”。

正确路径保持 FIDO/WebAuthn 标准：

```text
设备 B 输入手机号
→ FmlySys 下发该 identity 已有 credential 的 assertion challenge
→ 浏览器/系统凭据面板先尝试本机或同步凭据
→ 没有本地凭据时，用户选择“使用其他设备 / 手机或平板电脑”
→ 系统显示 FIDO hybrid 跨设备二维码
→ 设备 A 扫码，并用 A 中已有 Passkey 完成签名
→ B 恢复同一登录身份
```

为避免“credential 最初创建时记录的 transport hint”把新设备限制在 `internal` 等原 transport，登录 JS 在调用 `navigator.credentials.get()` 前移除 `allowCredentials[].transports` 提示，由浏览器自行选择本机、同步或 `hybrid` 通道。

验证完成后，仍可在个人 Passkey 页面给设备 B 注册第二把属于同一 identity 的 Passkey，从此 B 可独立登录。

注意：是否直接弹出二维码属于操作系统/浏览器的 WebAuthn UI；网站不能用标准 API 在“不知道本机是否存在指定私钥”的前提下强制越过系统凭据选择器。当前实现采用标准兼容行为，而不是自造不安全的 Passkey 传输协议。

### 29.4 四类资金写入统一为“文件先处理、DB 单事务、4 秒写阶段”

新增统一 Store 写入层，覆盖：

```text
资产变动
公共消费
内部转账
登记报销
```

对于带凭证的三类业务：

```text
HTTP multipart 解析
→ 文件校验 / 哈希 / 磁盘暂存
→ 进入受控 DB 写阶段
→ 单一 SQLite transaction：
   业务主记录
   + 业务 audit
   + record_attachments 元数据
   + attachment audit
→ COMMIT
```

不再允许：

```text
先 COMMIT 主记录
→ 再复制文件 / 再开 attachment transaction
```

所以转账和报销不会再出现“刷新已经看到主记录，但原请求还在等待凭证保存”的半完成数据库状态。

文件 I/O 不持有 SQLite writer；进入数据库阶段后使用进程内串行 gate，并让“等待 gate + SQLite transaction”共享同一个 4 秒 context。数据库繁忙超过边界会明确失败，而不是因为 SQLite busy/writer 竞争持续几十秒 Pending。

无附件时，前端资金表单继续使用 urlencoded，能进入现有普通请求 deadline；旧缓存或直接 multipart 调用仍由已有兼容层处理。

### 29.5 消费自动报销作为“可见流水”，但不重复记账

消费创建时仍按经手人的实时代管余额计算：

```text
public_paid_amount_cent = min(消费金额, 经手人可用代管余额)
reimbursable_amount_cent = 消费金额 - public_paid_amount_cent
```

因此经手人公共资产足够时：

```text
消费金额 300
public_paid = 300
待后续报销 = 0
```

这一部分在资金意义上就是“消费发生时自动报销”。

消费详情页的“报销状态与流水”现在统一显示：

```text
自动报销流水
+
后续人工登记的 reimbursements
```

自动报销行显示发生时间、付款持有人、收款成员、金额，并沿用原消费凭证。

它是由 `public_expenses.public_paid_amount_cent` 派生的显示流水，不额外 INSERT 一条 `reimbursements`，避免：

```text
public_paid 已扣一次
+
新增 reimbursement 再扣一次
```

前台和后台都通过同一个 `/assets/expenses/{id}/edit` 详情模板查看，所以两侧口径一致。

### 29.6 “快速记录（稍后整理数据）”工作流

首页“公共资产”卡片的 quick actions 最后一项新增：

```text
快速记录（稍后整理数据）
```

进入：

```text
/quick-money-note
```

页面上方快速表单包含：

- 醒目的 checkbox 分类：公共消费、内部转账、登记报销、资产变动登记；
- 文件：票据、照片、截图、文档、音视频等现有 evidence 类型；
- 简短摘要 textarea。

四个 checkbox 在交互上强制单选；服务端再次强校验“必须且只能有一个分类”，不依赖 JS 保证数据一致性。

快速记录存入新增的：

```text
quick_money_notes
```

此时不会改变任何公共资产余额。

页面下方展示本人快速记录列表；draft 状态提供：

```text
进行数据入库
```

进入用户要求的路径：

```text
/quick-money-note-to-standarized?id=<note_id>
```

### 29.7 根据分类加载正式表单并沿用文件/摘要

标准化页面按分类分别显示：

- 公共消费：事项、分类、金额、时间、渠道、商户、事务、描述；
- 内部转账：方向、对方、金额、渠道、时间、事务、用途；
- 登记报销：待报销消费、金额、渠道、时间、备注；
- 资产变动：资产新增 / 资产减少、金额、时间、说明。

原快速摘要预填到标题/描述/用途/备注等适合的位置。

快速记录的附件不复制第二份文件；正式入库与 quick note 状态更新在同一个事务中，将 `record_attachments` 的：

```text
entity_type / entity_id
```

原子迁移到最终：

```text
expense
transfer
reimbursement
asset_event
```

所以最终流水与快速记录使用的是同一份已保存文件，不产生重复磁盘副本。

### 29.8 成功提交后定位并高亮新增流水

四类正式资金 POST 成功后服务端都返回响应头：

```text
X-Fmly-Record-Key: <kind>:<id>
```

例如：

```text
expense:35
transfer:18
reimbursement:9
asset_event:27
```

现有增强表单响应层会保留该自定义 header。

全局 `record-focus.js` 包装浏览器 `fetch`，在导航前把 key 保存到 `sessionStorage`。目标页面加载后通过受现有 member/admin 权限保护的：

```text
GET /api/money-record/{kind}/{id}
```

获取该条记录的可见定位信息，再定位到对应流水行。

定位成功后：

1. `scrollIntoView({block: "center"})`；
2. 给新增行设置可聚焦状态并调用 `focus()`；
3. 临时增加高亮边框、背景与阴影；
4. 约 4.2 秒后移除高亮；
5. 删除 sessionStorage 中的待定位 key。

该机制同时覆盖：

```text
前台 / 后台
×
资产变动 / 公共消费 / 内部转账 / 登记报销
```

从快速记录标准化入库后也使用相同 record key，因此同样跳到 `/assets` 对应的正式新流水。

### 29.9 migration

新增：

```text
migrations/partition/000007_quick_money_notes.sql
```

表：

```text
quick_money_notes
```

字段保存分类、摘要、draft/standardized 状态、最终实体类型与 ID、创建成员、创建/标准化时间。

附件继续复用既有 `record_attachments`，不新增另一套附件表。

### 29.10 验证与剩余环境边界

提交前执行/检查：

- 所有新增、修改 Go 文件执行 `gofmt`；
- 使用 Go 标准库 parser 对本轮 Go 文件做语法解析；
- `web/static/passkeys.js`、`record-focus.js`、`quick-money.js` 使用 `node --check`；
- 新 migration 使用 SQLite 实际执行建表并检查约束；
- 登录模板、快速记录模板、消费详情模板与共享 nav 模板做模板语法解析检查；
- 重新检查启动 middleware 顺序，确认 V3 money routes 位于旧 asset workflow route 外层并实际接管相同 POST URL；
- 不创建 PR。

实际 FIDO 跨设备二维码是否出现、国行三星对 hybrid transport 的系统实现，以及真实 SQLite 高并发/磁盘 I/O 延迟，仍需要在部署机器和真实终端做 E2E。这里不把静态检查等同于真实设备验证。

## 30. 修复 V3 资金工作流启动编译失败

日期：2026-08-24

### 30.1 现象

Windows 本地执行 `win-dev.start.cmd` 时，Go 编译阶段直接失败：

```text
internal\httpserver\money_workflow_v3.go:11:2: "github.com/Ltre/FmlySys/internal/store" imported and not used
```

服务因此完全没有启动。

### 30.2 根因与修复

第 29 节新增 `money_workflow_v3.go` 时曾在设计阶段直接引用 `store` 包，最终实现改为全部通过 `s.Store` 方法访问 Store，但 import 列表中残留了：

```go
"github.com/Ltre/FmlySys/internal/store"
```

Go 编译器禁止未使用 import，因此虽然 `go/parser` 语法解析能通过，真实编译必然失败。

本轮删除该残留 import，不修改 V3 资金工作流行为、路由、事务或数据模型。

### 30.3 验证边界修正

这次问题说明仅做 parser 语法检查不足以作为 Go 代码提交的编译级验证。后续应优先执行真实 `go test ./...` 或至少 `go build ./cmd/fmlysys`；只有依赖/网络环境确实阻断真实编译时，才退化为 parser 检查并明确标注限制。

当前执行环境仍无法解析 `github.com`，无法在此处重新拉取仓库并运行完整 Go build；但本次失败点已经由用户本地编译器精确定位，修复仅删除确认未使用的 import。
