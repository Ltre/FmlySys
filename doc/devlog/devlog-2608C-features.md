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
