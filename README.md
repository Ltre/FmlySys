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

## Windows 本地开发

```bat
scripts\dev-windows.cmd
```

脚本只监听 `127.0.0.1:8080`，并设置当前进程临时代理。开发脚本默认打开：

```text
FMLYSYS_DEV_AUTH_ENABLED=1
```

因此本地未配置微信开放平台时仍可通过“本地开发身份登录”测试。正式部署必须关闭该变量。

### `data/config.env` 本机持久配置

Windows 启动脚本会在首次运行时自动创建：

```text
data/config.env
```

该文件位于运行时 `data/` 中，已经被 `.gitignore` 排除，适合保存本机测试环境的管理员初始化信息和微信开发者配置，不需要每次启动前重新执行 `set`。

示例：

```text
FMLYSYS_ADMIN_USERNAME=admin
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=请换成至少10位的强密码
FMLYSYS_WECHAT_APP_ID=你的AppID
FMLYSYS_WECHAT_APP_SECRET=你的AppSecret
FMLYSYS_WECHAT_REDIRECT_URL=https://你的已审核域名/auth/wechat/callback
FMLYSYS_MASTER_KEY=
```

读取优先级固定为：

```text
环境变量 > data/config.env > 程序默认值
```

因此部署环境仍然可以使用环境变量覆盖本机配置。`FMLYSYS_DATA_DIR` 本身决定配置文件所在目录，所以它只从环境变量/启动脚本取得，不从 `data/config.env` 自己读取。

`data/config.env` 可能包含管理员初始密码和微信 AppSecret，应当视为敏感文件，不要提交、发送或放进公开备份。

### 首次创建后台管理员

如果 `system.db` 还没有管理员，程序会从环境变量或 `data/config.env` 读取：

```text
FMLYSYS_ADMIN_USERNAME
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD
```

并在启动时创建第一个管理员。密码至少 10 个字符。

这个密码只用于**首次创建管理员**：管理员一旦写入 `system.db`，以后启动不需要再次提供 bootstrap password，也不会因为修改 `data/config.env` 而自动重置已有管理员密码。

访问 `/admin/login`，密码验证通过后首次绑定 Google Authenticator，之后每次登录均需要密码 + TOTP。

首次绑定页提供“密钥别名”输入框。可以使用例如：

```text
FmlySys 本机测试
FmlySys 测试服务器
FmlySys 正式环境
```

二维码会随别名更新，Google Authenticator 中会以该别名区分不同环境。别名只是 OTPAuth 的账号标签，不参与验证码计算，也不改变 TOTP Secret。

TOTP 密钥使用 AES-256-GCM 加密保存。若未设置 `FMLYSYS_MASTER_KEY`，程序会在 `data/system.key` 自动生成本机主密钥；该文件必须与数据库一并安全备份，且不得提交 Git。

### 微信扫码登录

网站应用需要配置：

```text
FMLYSYS_WECHAT_APP_ID
FMLYSYS_WECHAT_APP_SECRET
FMLYSYS_WECHAT_REDIRECT_URL
```

以上三项既可以写入 `data/config.env`，也可以通过环境变量提供。

`FMLYSYS_WECHAT_REDIRECT_URL` 应指向：

```text
https://你的已审核域名/auth/wechat/callback
```

AppSecret 仅在服务端使用。未知微信身份扫码后只能提交加入申请，在后台审核通过并绑定到内部 `member_id` 以后才能创建正式成员 session。

## 其他环境变量

```text
FMLYSYS_ADDR=127.0.0.1:8080
FMLYSYS_DATA_DIR=data
FMLYSYS_DEV_MEMBER=Dev Admin
FMLYSYS_DEV_AUTH_ENABLED=0
FMLYSYS_ADMIN_USERNAME=admin
FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=
FMLYSYS_MASTER_KEY=
FMLYSYS_WECHAT_APP_ID=
FMLYSYS_WECHAT_APP_SECRET=
FMLYSYS_WECHAT_REDIRECT_URL=
```
