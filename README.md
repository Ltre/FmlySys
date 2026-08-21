# FmlySys

家族公共事务与共同资产治理系统。

## Step1 当前实现

- Go 单体 Web、SQLite、system.db + 独立业务数据分区；
- 公共资产：持有人虚拟账户、资产增减、公共消费、自动待报销、分次报销、内部转账、支付/转账凭证；
- 家族事务与信息共享；
- 微信网站扫码登录：未知身份申请加入 → Pending → 后台审核绑定成员；
- 成员权限控制与服务器端鉴权；
- `/admin` 独立管理员密码 + Google Authenticator TOTP；
- 关键业务修改 Audit 基础。

## 服务监听地址与端口

服务端口统一由进程环境变量 `FMLYSYS_PORT` 提供，Go 代码中不再设置任何默认端口，也不再使用旧的 `FMLYSYS_ADDR`。监听主机由 `FMLYSYS_BIND_HOST` 提供。

这两个变量属于**启动环境配置**，由对应启动脚本设置，不从 `data/config.env` 读取：

```text
FMLYSYS_BIND_HOST
FMLYSYS_PORT
```

如果没有设置 `FMLYSYS_PORT`，或端口不是 `1-65535` 的整数，FmlySys 会直接拒绝启动，避免静默回落到某个硬编码端口。

## Windows 本地开发

运行：

```bat
scripts\win-dev.start.cmd
```

Windows 开发脚本集中定义：

```text
FMLYSYS_BIND_HOST=127.0.0.1
FMLYSYS_PORT=8080
FMLYSYS_DEV_AUTH_ENABLED=1
```

因此要修改 Windows 本地开发端口，只需要修改 `scripts/win-dev.start.cmd` 中的 `FMLYSYS_PORT`，不需要再搜索 Go 代码或其他配置文件。

该脚本还会设置当前进程使用的本地代理、解析仓库根目录、初始化 `data/config.env`、执行 `go mod tidy` / `go mod download all` / `go mod verify`，最后运行：

```bat
go run ./cmd/fmlysys
```

开发脚本默认打开本地开发身份登录。正式部署必须关闭该能力。

## Linux 阿里云香港服务器

新增启动脚本：

```text
scripts/linux-alyhk.start.cmd
```

虽然文件后缀沿用项目启动脚本的 `.cmd` 命名，但它实际是 Bash 脚本。首次使用：

```bash
chmod +x scripts/linux-alyhk.start.cmd
./scripts/linux-alyhk.start.cmd
```

脚本集中定义：

```text
FMLYSYS_BIND_HOST=0.0.0.0
FMLYSYS_PORT=8080
FMLYSYS_DEV_AUTH_ENABLED=0
```

如需调整 Linux 服务器端口，只修改该脚本顶部的 `FMLYSYS_PORT`。如服务器只允许 Nginx/Caddy 本机反代，也可以把 `FMLYSYS_BIND_HOST` 改为 `127.0.0.1`。

Linux 脚本行为与 Windows 开发脚本保持同一启动流程：

1. 根据脚本所在目录定位仓库根目录；
2. 设置 `FMLYSYS_DATA_DIR`；
3. 如果不存在则创建 `data/config.env` 模板；
4. 检查 Go 是否可用；
5. 执行 `go mod tidy`；
6. 执行 `go mod download all`；
7. 执行 `go mod verify`；
8. `go run ./cmd/fmlysys`。

Linux 脚本不会写入 Windows 本地代理地址，并且默认关闭开发身份登录。

## `data/config.env` 本机持久配置

启动脚本会在首次运行时自动创建：

```text
data/config.env
```

该文件位于运行时 `data/` 中，已经被 `.gitignore` 排除，适合保存管理员初始化/重置信息和微信开发者配置，不需要每次启动前重新设置环境变量。

示例：

```text
FMLYSYS_ADMIN_USERNAME=admin
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=请换成至少10位的强密码
FMLYSYS_WECHAT_APP_ID=你的AppID
FMLYSYS_WECHAT_APP_SECRET=你的AppSecret
FMLYSYS_MASTER_KEY=
```

注意：`FMLYSYS_PORT` 和 `FMLYSYS_BIND_HOST` 不属于这里的持久业务配置，它们由启动脚本作为进程环境变量提供。

普通配置读取优先级为：

```text
环境变量 > data/config.env > 程序默认值
```

`FMLYSYS_DATA_DIR` 本身决定配置文件所在目录，因此只从环境变量/启动脚本取得；监听端口同样只接受启动环境中的 `FMLYSYS_PORT`。

`data/config.env` 可能包含管理员临时初始化/重置密码和微信 AppSecret，应当视为敏感文件，不要提交、发送或放进公开备份。管理员密码凭据创建成功后，可以把 `FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD` 清空；只有首次创建或忘记密码需要重置时再临时填写。

## 管理员密码不存数据库

管理员身份、TOTP 状态和后台 Session 仍保存在 `system.db`，但管理员密码及其密码摘要不再作为有效凭据保存在数据库中。

密码验证使用独立文件：

```text
data/admin-credentials.enc
```

文件内容是管理员用户名和 PBKDF2-SHA256 密码摘要组成的凭据记录，再整体使用当前 `system.key` / `FMLYSYS_MASTER_KEY` 对应的 AES-256-GCM 主密钥加密。数据库中的旧 `password_hash` 列仅为旧 schema 兼容保留，新代码只写空字符串。

升级旧数据时：

1. 如果 `data/admin-credentials.enc` 已存在，直接使用加密凭据；
2. 如果文件不存在但旧 `system.db` 仍有 `password_hash`，先把旧 hash 加密迁移到文件；
3. 只有确认加密文件成功写入后，才清空数据库中的旧 `password_hash`；
4. 新建管理员从一开始就只把密码摘要写入加密文件。

因此不需要删除现有 `system.db`，也不需要重新绑定 Google Authenticator。

## 首次创建与忘记密码重置

如果 `system.db` 还没有管理员，程序会从环境变量或 `data/config.env` 读取：

```text
FMLYSYS_ADMIN_USERNAME
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD
```

并创建管理员身份及 `data/admin-credentials.enc`。密码至少 10 个字符。

如果以后忘记管理员密码，也不需要删除数据库。直接在 `data/config.env` 临时设置一个新的：

```text
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=一个新的至少10位密码
```

重新启动后，程序会更新加密密码凭据并使现有后台 Session 失效；`system.db`、家族业务数据和既有 TOTP 绑定保持不变。确认新密码可以登录后，可再次把该配置清空。

访问 `/admin/login` 后仍然是：管理员密码 → Google Authenticator TOTP → 后台。

首次绑定页提供“密钥别名”输入框，可以填写例如：

```text
FmlySys 本机测试
FmlySys 测试服务器
FmlySys 正式环境
```

二维码会随别名更新，Google Authenticator 中会以该别名区分不同环境。别名只是 OTPAuth 的账号标签，不参与验证码计算，也不改变 TOTP Secret。

TOTP Secret 使用 AES-256-GCM 加密保存在 `system.db`；管理员密码凭据文件也使用同一主密钥加密。若未设置 `FMLYSYS_MASTER_KEY`，程序会在 `data/system.key` 自动生成本机主密钥。`system.key` 必须与 `system.db`、`admin-credentials.enc` 一并安全备份，且不得提交 Git。

## 微信扫码登录

FmlySys 本身只需要保存微信应用凭据：

```text
FMLYSYS_WECHAT_APP_ID
FMLYSYS_WECHAT_APP_SECRET
```

OAuth 回调接口由代码固定为：

```text
GET /auth/wechat/callback
```

完整回调地址无需在 FmlySys 中重复配置。程序在用户打开 `/login/wechat` 时，根据该请求实际使用的 scheme + Host 自动构造。

例如站点实际访问地址为：

```text
https://family.example.com
```

则本次微信 OAuth 使用的完整回调地址自动为：

```text
https://family.example.com/auth/wechat/callback
```

如果前面有 Nginx/Caddy/其他反向代理，应保留原始 `Host`，并正确设置 `X-Forwarded-Proto`。FmlySys 不读取 `X-Forwarded-Host`。

AppSecret 仅在服务端使用。未知微信身份扫码后只能提交加入申请，在后台审核通过并绑定到内部 `member_id` 以后才能创建正式成员 session。

## 环境变量摘要

启动脚本负责：

```text
FMLYSYS_BIND_HOST
FMLYSYS_PORT
FMLYSYS_DATA_DIR
FMLYSYS_DEV_AUTH_ENABLED
```

其他可配置项：

```text
FMLYSYS_DEV_MEMBER=Dev Admin
FMLYSYS_ADMIN_USERNAME=admin
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=
FMLYSYS_MASTER_KEY=
FMLYSYS_WECHAT_APP_ID=
FMLYSYS_WECHAT_APP_SECRET=
```

旧的 `FMLYSYS_ADDR` 不再使用。
