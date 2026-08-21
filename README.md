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

### 首次创建后台管理员

在启动脚本前设置：

```bat
set FMLYSYS_ADMIN_USERNAME=admin
set FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=请换成至少10位的强密码
scripts\dev-windows.cmd
```

第一次成功创建管理员以后，可以移除 `FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD`。访问 `/admin/login`，密码验证通过后首次绑定 Google Authenticator，之后每次登录均需要密码 + TOTP。

TOTP 密钥使用 AES-256-GCM 加密保存。若未设置 `FMLYSYS_MASTER_KEY`，程序会在 `data/system.key` 自动生成本机主密钥；该文件必须与数据库一并安全备份，且不得提交 Git。

### 微信扫码登录

网站应用需要配置：

```text
FMLYSYS_WECHAT_APP_ID
FMLYSYS_WECHAT_APP_SECRET
FMLYSYS_WECHAT_REDIRECT_URL
```

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
