# FmlySys 初始技术需求文档

> 建议文件名：`doc/technical-requestment.md`
> 文档状态：Initial Draft / V0.1
> 对应业务需求：`doc/requestment.md`
> 初始讨论参考：`doc/prompt/first.md`

---

# 1. 文档目的

本文档在《家族公共事务与共同资产治理系统需求文档》的基础上，明确 FmlySys 第一阶段开发所采用的技术架构、数据模型原则、认证方式、权限边界、接口边界、数据安全、审计、附件存储、部署和备份等技术要求。

本文档重点回答：

* 系统使用什么技术实现；
* 前台、后台如何划分；
* SQLite 如何承载公共财产及事务数据；
* 公共财产余额如何计算；
* 如何保证历史修改可追溯；
* 家族成员与管理员如何认证；
* 附件和账单凭证如何保存；
* 哪些技术能力属于第一阶段；
* 哪些能力明确暂不建设；
* 系统规模较小时应避免哪些过度设计。

本文档不是完整数据库 DDL、API Swagger 或 UI 设计稿。

后续可以基于本文继续拆分：

* 数据库设计文档；
* API 文档；
* 前端交互设计；
* 部署文档；
* 测试方案。

---

# 2. 项目技术定位

FmlySys 是一个：

> **低并发、小规模、家族内部使用、强调长期数据可靠性和历史可追溯性的 Web 系统。**

系统不是面向公众运营的 SaaS。

预计长期使用人数较少，典型规模：

* 几名至几十名家族成员；
* 同时在线人数很少；
* 财务写操作频率低；
* 数据总量以文字、图片、票据、PDF 为主；
* 数据生命周期可能持续多年甚至几十年。

因此技术设计优先级为：

1. 数据正确；
2. 容易维护；
3. 容易部署；
4. 容易备份和迁移；
5. 操作可追溯；
6. 安全边界清晰；
7. 长期运行成本低；
8. 最后才是高并发性能。

第一阶段不为理论上的大规模用户量增加不必要的系统复杂度。

---

# 3. 固定技术选型

## 3.1 服务端

使用：

> **Golang**

建议采用单体应用架构。

一个 Go 服务同时承担：

* HTTP 服务；
* 前台页面；
* 后台页面；
* API；
* 登录认证；
* 权限检查；
* 财务计算；
* SQLite 访问；
* 附件管理；
* 审计日志；
* 提醒计算；
* 系统管理。

第一阶段不拆分微服务。

---

# 4. 数据库

使用：

> **SQLite**

数据库作为系统结构化数据的唯一权威持久化存储。

建议：

```text
data/
└── fmlysys.db
```

数据库中存储：

* 家族成员；
* 登录身份；
* 权限；
* 公共财产记录；
* 公共资产持有人；
* 公共消费；
* 垫付；
* 报销；
* 内部转账；
* 遗产；
* 家族事务；
* 决议；
* 提醒；
* 附件元数据；
* 操作审计；
* 系统配置。

---

# 5. SQLite 使用原则

启动数据库连接后至少启用：

```sql
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
```

写事务必须：

* 尽可能短；
* 避免在事务中执行文件 IO；
* 避免长时间持有写锁；
* 相关业务数据和审计日志必须在同一事务中提交。

FmlySys 用户量较少，因此无需为 SQLite 人为构造复杂的分布式锁。

但必须正确处理：

```text
database is locked
```

等 SQLite 并发写入情况。

---

# 6. 金额存储规则

任何人民币金额禁止使用：

```text
FLOAT
REAL
DOUBLE
```

作为权威数据。

统一使用：

> **整数分**

例如：

```text
100.00 元
```

数据库保存：

```text
10000
```

推荐字段命名：

```text
amount_cent
```

或者：

```text
amount
```

但整个项目必须保持一致。

Go 业务层使用：

```go
int64
```

表示金额。

展示层再转换为：

```text
¥100.00
```

避免浮点误差影响公共资产账目。

---

# 7. 时间存储规则

所有精确时间统一保存为：

* UTC Unix 时间；
* 或 UTC RFC3339 时间。

项目只能选择一种并全局保持一致。

页面展示时按照系统配置的家庭时区转换。

对于：

* 百日；
* 对年；
* 三年；
* 入祠；
* 祭祀；

这类以“日期”而不是精确时刻为核心的信息，应允许独立保存：

```text
date
```

不能强制转换成某一天 00:00:00 后再依赖时区计算。

---

# 8. 总体系统架构

第一阶段采用：

```text
Browser
   │
   │ HTTPS
   ▼
Reverse Proxy
   │
   ▼
FmlySys Go Server
   │
   ├── 前台 /
   ├── 后台 /admin
   ├── API
   ├── Authentication
   ├── Authorization
   ├── Business Services
   ├── Audit
   ├── Attachment Service
   │
   ├── SQLite
   │
   └── Local File Storage
```

不引入：

* Redis；
* MySQL；
* PostgreSQL；
* MongoDB；
* Elasticsearch；
* Kafka；
* RabbitMQ；
* 独立 Session Server；
* 独立文件服务；
* Kubernetes。

除非未来实际业务证明有必要。

---

# 9. 前端技术原则

第一阶段不要求 React、Vue 等大型 SPA 框架。

优先：

```text
Go html/template
+
HTML
+
CSS
+
少量 Vanilla JavaScript
```

可以对部分复杂交互采用：

```text
fetch()
JSON API
```

进行局部刷新。

原因：

* 页面数量有限；
* 用户规模很小；
* 后台管理系统交互以表格和表单为主；
* 降低构建依赖；
* 降低长期维护复杂度；
* 方便单个 Go 二进制部署。

如果未来页面复杂度显著增加，可以重新评估是否引入前端框架。

第一阶段不提前为此增加复杂度。

---

# 10. 静态资源

推荐目录：

```text
web/
├── templates/
└── static/
    ├── css/
    ├── js/
    └── img/
```

生产版本可以通过：

```go
//go:embed
```

将：

* HTML 模板；
* CSS；
* JS；
* 内置图标；

嵌入 Go 二进制。

用户上传内容不得 embed。

---

# 11. URL 总体规划

## 11.1 家族前台

入口固定：

```text
/
```

包括：

```text
/
/login
/join
/pending

/assets
/expenses
/reimbursements
/matters
/estate
/archive
/members
/timeline
/decisions
```

具体 URL 后续可以调整。

但所有普通家族成员功能统一归属于：

> `/`

体系。

---

# 12. 管理后台

后台入口固定：

```text
/admin
```

例如：

```text
/admin
/admin/login
/admin/members
/admin/join-requests
/admin/permissions
/admin/audit
/admin/settings
/admin/backup
```

后台负责：

* 家族成员审核；
* 权限管理；
* 系统配置；
* 后台财务管理；
* 敏感资料权限；
* 审计查看；
* 系统维护。

---

# 13. 前台与后台必须逻辑隔离

虽然前后台运行在同一个 Go 服务中，但：

```text
/
```

和：

```text
/admin
```

必须拥有独立认证中间件。

普通成员 Session：

> 不能自动获得 `/admin` 权限。

后台 Session：

> 不等同于普通成员身份。

即使同一个现实人物既是家族成员又是系统管理员，也应在逻辑上区分：

```text
Family Member Identity

Admin Identity
```

---

# 14. API 路径

普通业务 API 建议：

```text
/api/v1/*
```

例如：

```text
/api/v1/assets
/api/v1/expenses
/api/v1/reimbursements
/api/v1/matters
```

后台 API：

```text
/admin/api/v1/*
```

API 第一阶段仅供 FmlySys 自己的 Web 页面使用。

不把 API 当作公开开发者接口。

因此第一阶段：

* 不开放 API Key；
* 不建设第三方 OAuth；
* 不提供 CORS 公共调用；
* 不提供开放平台；
* 不承诺 API 长期向第三方兼容。

---

# 15. 家族成员认证

普通家族成员优先采用：

> **微信身份认证**

家族成员与微信身份必须分离。

核心业务对象：

```text
member
```

认证对象：

```text
identity
```

一个身份绑定：

```text
member_id
```

---

# 16. 微信身份数据模型

至少预留：

```text
member_id
provider
provider_user_id
openid
unionid
created_at
last_login_at
status
```

其中：

```text
provider = wechat
```

业务表禁止直接引用：

```text
openid
```

必须引用：

```text
member_id
```

---

# 17. 家族成员首次注册

未识别微信身份访问系统：

```text
微信认证
↓
取得微信身份
↓
身份尚未绑定成员
↓
创建加入申请
↓
填写姓名/家庭关系/说明
↓
等待审核
```

此时用户权限限定为：

```text
查看自己的申请状态
```

不得查看：

* 公共财产；
* 家族成员；
* 遗产；
* 家族事务；
* 档案。

---

# 18. 成员审核

管理员通过：

```text
/admin/join-requests
```

处理申请。

管理员可以：

### 方式 A

绑定到已经存在的：

```text
member
```

### 方式 B

新建：

```text
member
```

后再绑定。

批准后该微信身份才能进入系统。

---

# 19. 微信登录技术边界

微信仅承担：

> 身份认证。

微信：

* 昵称不是系统姓名；
* 头像不是家族档案头像的权威来源；
* OpenID 不是业务主键；
* 微信资料变化不应改变历史业务记录。

如果实际部署环境无法满足微信 Web 登录的回调、域名或开放平台条件，应保留认证适配层。

不得让微信登录实现侵入：

* 公共财产；
* 事务；
* 遗产；

等业务代码。

---

# 20. 开发环境身份登录

允许提供：

> Development Only

的本地测试身份入口。

例如：

```text
DEV_AUTH_ENABLED=true
```

启用后开发人员可以模拟成员。

该功能在 Production 必须：

```text
DEV_AUTH_ENABLED=false
```

生产环境不得暴露调试身份登录。

---

# 21. 管理后台认证

`/admin` 使用：

> **Google Authenticator 兼容 TOTP**

遵循：

> RFC 6238

Google Authenticator 只是客户端验证器。

服务端自己负责：

* TOTP Secret；
* 验证；
* Session；
* 登录失败限制。

---

# 22. 后台不建议只使用六位 TOTP

六位 TOTP 本质属于：

> 动态验证因子。

因此推荐后台登录使用：

```text
管理员密码
+
Google Authenticator TOTP
```

即：

```text
第一因子：管理员密码
第二因子：TOTP
```

而不是单独把六位动态码视作完整后台密码。

---

# 23. 管理员账户

建议建立独立：

```text
admin_users
```

表。

主要字段：

```text
id
username
password_hash
totp_secret_ciphertext
enabled
created_at
last_login_at
last_totp_counter
```

密码只保存：

> Argon2id Hash。

禁止保存明文密码。

---

# 24. TOTP Secret

TOTP Secret 禁止：

* 明文提交 Git；
* 输出日志；
* 返回普通 API；
* 显示给其他管理员；
* 保存在浏览器 LocalStorage。

数据库中如果保存 TOTP Secret，应使用服务端 Master Key 加密。

例如：

```text
FMLYSYS_MASTER_KEY
```

Master Key 从：

* 环境变量；
* 或服务器权限受限的 Secret 文件；

加载。

禁止进入 Git 仓库。

---

# 25. 后台初始管理员初始化

第一阶段不要建设：

> 未登录用户访问 `/admin/setup` 创建超级管理员

这种开放初始化方式。

推荐：

```text
fmlysys admin create
```

或者首次部署通过命令行初始化。

命令输出 Google Authenticator：

* Secret；
* `otpauth://` URI；
* 或终端二维码。

管理员扫码后完成初始化。

---

# 26. 后台 Session

后台认证成功后建立服务端 Session。

Cookie 建议：

```text
HttpOnly
Secure
SameSite=Lax
Path=/admin
```

后台 Cookie 与普通成员 Cookie 分离。

例如：

```text
fmly_session
fmly_admin_session
```

---

# 27. Session 技术方案

使用：

> 服务端 Session。

Session ID 使用加密安全随机数。

SQLite 只保存：

> Session Token Hash

而非原始 Session Token。

推荐 Session 表：

```text
sessions
admin_sessions
```

字段包括：

```text
id
token_hash
actor_id
created_at
expires_at
last_seen_at
ip
user_agent
```

---

# 28. CSRF

所有：

```text
POST
PUT
PATCH
DELETE
```

操作必须具备 CSRF 防护。

不得因为系统用户少就省略 CSRF。

对于 HTML Form：

> CSRF Token。

对于前端 Fetch：

> CSRF Header。

---

# 29. 登录防爆破

`/admin/login` 必须提供基本限流。

至少针对：

* IP；
* 用户名；

限制连续失败。

TOTP 允许：

```text
当前时间窗口 ±1
```

范围验证。

已经成功使用过的旧 TOTP 时间窗口，原则上不得重复接受。

---

# 30. 权限体系

第一阶段采用：

> 简单 RBAC + 必要的资源 ACL。

不构建企业级 IAM。

成员基础角色可以包含：

```text
member
asset_manager
family_manager
```

后台管理员：

```text
admin
```

独立管理。

---

# 31. 资源级权限

针对：

* 医疗；
* 身份证件；
* 遗嘱；
* 敏感档案；

允许设置：

```text
family
selected_members
admin_only
```

必要时建立：

```text
resource_acl
```

例如：

```text
resource_type
resource_id
member_id
permission
```

第一阶段不构造复杂 ABAC 表达式语言。

---

# 32. 数据库迁移

必须建立数据库 Migration。

例如：

```text
migrations/
├── 000001_init.sql
├── 000002_asset_ledger.sql
└── ...
```

数据库记录：

```text
schema_migrations
```

每次启动：

1. 检查数据库版本；
2. 执行未运行 Migration；
3. Migration 失败则终止启动；
4. 不允许带着半升级数据库继续工作。

生产数据库禁止依赖人工逐条执行 SQL 才能升级。

---

# 33. 数据库访问层

推荐使用：

```text
database/sql
```

加明确 SQL。

可以选择：

```text
sqlc
```

生成类型安全访问代码。

第一阶段不要求使用 GORM 等大型 ORM。

核心账务 SQL 应保持：

* 可阅读；
* 可审核；
* 可测试；
* 可明确知道数据如何计算。

---

# 34. SQLite Driver

优先考虑 Pure Go SQLite Driver，例如：

```text
modernc.org/sqlite
```

避免部署时强制依赖 CGO。

如果后期确定运行平台固定，也可以评估其他 SQLite Driver。

Driver 属于实现细节，但 SQLite 数据文件格式不得依赖私有实现。

---

# 35. 公共财产核心数据原则

系统不建立真实意义上的：

> 公共银行账户。

技术层必须完全遵守：

> **公共财产总账户是虚拟账本。**

公共财产实际可能分别由：

```text
A
B
C
```

等成员代管。

---

# 36. 持有人虚拟账户

数据库保存：

```text
holder
```

或等价关系。

它表达：

> 当前由某成员代管的公共资产。

但不得设计：

```text
holder_bank_accounts
holder_wechat_wallet
holder_alipay_balance
```

等功能。

---

# 37. 具体金融渠道只属于单笔业务凭证

例如发生：

```text
A → B
```

内部转账时，可以记录：

```text
支付渠道：微信
```

发生消费时可以记录：

```text
支付渠道：支付宝
```

它只说明：

> 这一次钱怎么支付。

不能据此建立：

```text
A 的微信公共资金余额
A 的银行卡公共资金余额
```

这类数据。

---

# 38. 财务数据采用“业务事件”为权威数据

数据库的权威事实是：

* 公共财产新增；
* 公共消费；
* 公共资产持有人内部转移；
* 垫付；
* 报销；
* 公共资产划出；
* 账务调整。

而不是：

```text
当前余额 = xxxx
```

---

# 39. 不保存死余额

不得把以下字段作为权威业务数据：

```text
before_balance
after_balance
current_balance
```

例如消费记录：

```text
金额：338 元
```

其：

```text
消费前余额
消费后余额
```

必须在查询时通过有效业务记录计算。

---

# 40. 缓存余额

未来为了性能允许增加：

```text
balance_cache
```

或者快照表。

但是必须满足：

> 缓存可以删除后重新生成。

缓存永远不能成为账务唯一来源。

以当前用户规模，V1 原则上没有必要实现余额缓存。

---

# 41. 财务事件类型

V1 至少支持：

```text
INITIAL_ASSET
ASSET_IN
EXPENSE
HOLDER_TRANSFER
REIMBURSEMENT
ASSET_OUT
ADJUSTMENT
```

业务语义分别为：

### INITIAL_ASSET

建立公共财产初始值。

### ASSET_IN

后续公共财产增加。

### EXPENSE

公共财产实际发生用途性消费。

### HOLDER_TRANSFER

两个公共资产持有人之间转移代管资金。

### REIMBURSEMENT

持有人向垫付成员支付报销款。

### ASSET_OUT

公共财产正式分配或退出公共财产体系。

### ADJUSTMENT

处理确有依据的历史账差等特殊情况。

---

# 42. 公共消费两种付款情形

## 情形 A：持有人直接使用其代管公共资产付款

例如：

```text
A 持有：20,000
消费：1,000
```

结果：

```text
公共财产净额 -1,000
A 持有金额 -1,000
```

不产生待报销。

---

# 43. 个人垫付消费

例如 C 使用个人资金：

```text
1,000
```

支付公共消费。

结果：

```text
公共财产净额 -1,000
持有人余额暂时不变
待报销 C +1,000
```

---

# 44. 报销

之后 A 使用其代管公共资产：

```text
A → C 1,000
```

报销。

结果：

```text
A 持有金额 -1,000
待报销 C -1,000
公共财产净额不变
```

不得第二次将：

```text
公共财产净额 -1,000
```

---

# 45. 内部转账

例如：

```text
A → B 10,000
```

用于 B 后续处理公共事务。

结果：

```text
A -10,000
B +10,000
公共财产净额不变
```

这类记录必须独立于：

> 公共消费。

---

# 46. 账务恒等关系

在当前业务模型下应能够验证：

```text
所有持有人虚拟余额合计
-
所有待报销金额
=
公共财产净额
```

如果不能成立：

> 系统应报告账务不一致。

禁止简单修改某个：

```text
current_balance
```

把错误“修平”。

必须找出：

> 哪一条业务事件造成不一致。

---

# 47. 公共财产净额

公共财产总账原则上根据有效事件动态计算：

```text
初始公共财产
+ 公共财产新增
- 公共消费
- 公共财产划出
± 合法账务调整
```

以下事件不得改变公共财产净额：

```text
HOLDER_TRANSFER
REIMBURSEMENT
```

---

# 48. 持有人余额计算

某成员当前代管金额由：

```text
划入其名下的公共资产
+ 其他持有人转入
- 转给其他持有人
- 使用代管公共资产直接消费
- 从代管资产支付报销
- 从其代管资产完成资产划出
± 合法调整
```

计算。

不得单独维护一个无法重算的：

```text
member.balance
```

作为权威金额。

---

# 49. 退款

消费退款原则上：

> 作为与原消费有关联的资金回流事件。

不能直接修改原消费为：

```text
0
```

除非原消费本身就是录入错误。

例如：

```text
原消费 ¥500
退款 ¥200
```

历史应该保留：

```text
消费 -500
退款 +200
```

而不是：

```text
消费 -300
```

这能准确表示真实发生过的资金行为。

---

# 50. 财务记录修改

用户发现：

```text
328
```

录错为：

```text
338
```

时：

> 可以直接修改原业务记录。

不要求：

```text
作废旧单 + 新建新单
```

---

# 51. 修改必须写审计日志

例如：

```text
entity_type: expense
entity_id: 123

field: amount
before: 32800
after: 33800

actor: member_5
time: ...
```

当前业务表保存：

> 最新有效值。

审计表保存：

> 历史变化过程。

---

# 52. 删除规则

普通录错：

* 金额错误；
* 经手人错误；
* 用途错误；
* 日期错误；

使用：

> 修改。

如果整条事件根本没有发生：

> 撤销。

数据库采用：

```text
status = revoked
```

或者：

```text
revoked_at
```

进行逻辑撤销。

财务记录不得通过正常 Web 功能：

```sql
DELETE FROM ...
```

物理删除。

---

# 53. 被撤销事件

被撤销事件：

* 不参与余额计算；
* 页面默认不参与正常流水；
* 仍可在历史中查询；
* 保留原始数据；
* 保留撤销人；
* 保留撤销时间；
* 保留撤销原因。

---

# 54. 审计日志

建立统一：

```text
audit_logs
```

至少记录：

```text
id
actor_type
actor_id
action
entity_type
entity_id
before_json
after_json
reason
request_id
ip
user_agent
created_at
```

---

# 55. actor_type

至少：

```text
member
admin
system
```

这样可以区分：

> 普通成员修改公共消费

与：

> 后台管理员执行系统管理

以及：

> 系统自动执行某项动作。

---

# 56. 审计写入事务

任何重要数据修改：

```text
BEGIN

修改业务数据
写 audit_logs

COMMIT
```

必须属于同一个数据库事务。

不允许：

```text
业务修改成功
↓
审计写入失败
```

后仍然返回成功。

---

# 57. 审计日志不可正常修改

应用层不得提供：

```text
UPDATE audit_logs
DELETE audit_logs
```

接口。

可以考虑使用 SQLite Trigger 阻止普通应用 SQL：

```text
UPDATE
DELETE
```

审计表。

系统数据库文件拥有者仍然可以直接修改 SQLite 文件。

因此 FmlySys 的审计目标是：

> 防止应用内无痕篡改。

不是：

> 对抗拥有服务器和数据库文件控制权的恶意系统管理员。

这是明确安全边界。

---

# 58. 乐观并发控制

由于公共消费允许多个成员编辑，所有重要可编辑业务表建议加入：

```text
version INTEGER NOT NULL
```

每次修改：

```text
version + 1
```

客户端提交修改时携带旧：

```text
version
```

若数据库版本已变化：

```text
UPDATE ... WHERE id=? AND version=?
```

更新不到记录，则返回：

> 数据已经被其他成员修改。

避免后保存的人无意覆盖先保存者的数据。

---

# 59. 初始核心数据库实体

V1 建议至少包含：

```text
schema_migrations

members
member_identities
join_requests

roles
member_roles
resource_acl

admin_users
sessions
admin_sessions

public_asset_sources
public_expenses
holder_transfers
reimbursements
asset_outflows
asset_adjustments

estate_cases
estate_items
estate_distributions

matters
matter_members
matter_tasks

decisions
decision_votes

archives
attachments

reminders

audit_logs
```

实际表名可以调整。

---

# 60. 公共消费表

建议包含：

```text
id
title
category
amount_cent
occurred_at

handler_member_id
payer_member_id

funding_type
holder_member_id

payment_channel
merchant
description

matter_id

reimbursable_amount_cent

status
version

created_by
created_at
updated_by
updated_at

revoked_by
revoked_at
revoke_reason
```

---

# 61. funding_type

至少区分：

```text
PUBLIC_HELD_ASSET
PERSONAL_ADVANCE
```

### PUBLIC_HELD_ASSET

付款直接来自某成员代管的公共财产。

### PERSONAL_ADVANCE

付款来自成员个人资产，需要形成报销关系。

必须避免仅通过：

```text
payer_member_id
```

猜测资金性质。

---

# 62. 报销表

一笔消费允许：

> 多次报销。

因此不能只在：

```text
public_expenses
```

保存：

```text
reimbursed = true/false
```

应该独立建立：

```text
reimbursements
```

例如：

```text
expense_id
payer_holder_member_id
receiver_member_id
amount_cent
payment_channel
occurred_at
note
status
```

这样支持：

```text
应报销 2,000

第一次 500
第二次 1,000
第三次 500
```

---

# 63. 报销状态属于派生状态

可以展示：

```text
未报销
部分报销
已全额报销
```

但真正权威数据是：

```text
应报销金额
+
有效 reimbursement 记录
```

即：

```text
remaining =
reimbursable_amount
-
SUM(reimbursement)
```

因此状态可以实时计算。

---

# 64. 内部转账表

建议：

```text
holder_transfers
```

包括：

```text
id
from_member_id
to_member_id
amount_cent
purpose
payment_channel
occurred_at
matter_id
status
version
created_by
created_at
updated_at
```

约束：

```text
from_member_id != to_member_id
amount_cent > 0
```

---

# 65. 遗产模块

遗产独立于：

> 公共财产。

主要实体：

```text
estate_cases
estate_items
estate_distributions
```

---

# 66. estate_cases

表示：

> 一次完整遗产事务。

例如：

```text
父亲遗产处理
```

字段可包含：

```text
name
description
status
opened_at
closed_at
```

---

# 67. estate_items

表示：

* 银行存款；
* 社保款；
* 房产；
* 宅基地权益；
* 现金；
* 其他财物。

金额不是所有遗产项目必填。

例如房产：

```text
amount_cent = NULL
estimated_value_cent = ...
```

允许：

> 非货币资产。

---

# 68. 遗产分配

`estate_distributions` 记录：

```text
estate_item_id
member_id
amount/value
status
completed_at
```

部分遗产正式成为公共财产时，应建立：

> 遗产分配记录

以及：

> 公共财产 ASSET_IN 记录。

两者使用关联 ID 串联。

不得只在遗产记录里写一句：

```text
其中 4 万作为公共财产
```

而公共财产账本没有对应资金来源。

---

# 69. 通用事务模块

家族事务采用：

```text
matters
```

统一建模。

不要分别开发：

```text
funeral_events
house_repairs
ritual_events
```

等完全独立体系。

---

# 70. Matter 层级

支持：

```text
parent_id
```

形成：

```text
父亲身后事务
├── 圆七
├── 百日
├── 对年
├── 三年
└── 入祠
```

同样支持：

```text
祖屋翻修
├── 屋顶
├── 水电
└── 外墙
```

---

# 71. Matter 与财务双向关联

公共消费：

```text
matter_id
```

允许关联某项事务。

查询事务时：

> 自动统计该事务关联的公共消费。

查看公共消费时：

> 可以进入对应事务。

不得为事务另建一套独立：

> “实际支出金额”

权威数据。

事务实际支出应根据关联公共消费计算。

---

# 72. 预计费用与实际费用

事务可以保存：

```text
estimated_cost_cent
```

作为计划值。

但：

```text
actual_cost
```

应由有效公共消费动态计算。

---

# 73. 家族决议

V1 可以建立基础：

```text
decisions
decision_votes
```

保存：

* 提案；
* 说明；
* 金额；
* 发起人；
* 开始/结束时间；
* 当前状态；
* 成员意见。

第一阶段只记录：

> 家族内部意见与决定。

不实现复杂公司治理表决模型。

---

# 74. 决议功能法律边界

系统中的：

```text
同意
不同意
弃权
```

表示：

> 系统内记录的家族意见。

不得由程序宣称：

> 自动产生法律上的遗产处分、产权变更或合同效力。

---

# 75. 附件存储

文件本体第一阶段不保存到 SQLite BLOB。

推荐：

```text
data/
├── fmlysys.db
└── uploads/
```

数据库只保存：

> 附件元数据。

---

# 76. 附件元数据

至少：

```text
id
storage_name
original_name
mime_type
size
sha256

owner_type
owner_id

uploaded_by
created_at

status
deleted_at
```

---

# 77. 上传文件名

禁止直接使用用户原始文件名作为磁盘文件名。

例如：

```text
微信图片_20260821.jpg
```

数据库保留原名称。

磁盘采用随机 ID：

```text
0198e...
```

或 UUID。

防止：

* 文件覆盖；
* 路径穿越；
* 特殊字符问题。

---

# 78. 附件访问

禁止：

```text
https://host/uploads/xxx.jpg
```

直接作为公开静态文件目录。

必须通过：

```text
/files/{id}
```

或受保护 API 访问。

服务器读取附件前必须：

> 检查当前成员是否拥有查看该业务对象的权限。

尤其适用于：

* 医疗资料；
* 遗嘱；
* 身份证件；
* 遗产文件。

---

# 79. 文件类型安全

上传后：

* 检查文件大小；
* 检查 MIME；
* 服务器自行重新生成存储文件名；
* 不在上传目录执行任何程序；
* 不信任用户上传的扩展名。

HTML/SVG 等可执行内容默认：

> 以附件下载形式返回。

避免直接在同源页面执行用户上传脚本。

---

# 80. 附件 Hash

上传时建议计算：

```text
SHA-256
```

用于：

* 文件完整性检查；
* 备份验证；
* 发现完全重复文件。

第一阶段不要求自动去重。

---

# 81. 附件删除

财务凭证、遗产证明等重要附件默认采用：

> 逻辑删除。

普通用户界面不立即物理删除文件。

删除动作进入：

> audit_logs。

第一阶段不需要自动定时清理已删除重要附件。

---

# 82. 家族档案

`archives` 用于保存：

* 家族资料；
* 历史照片；
* 祖屋文件；
* 墓地资料；
* 祠堂资料；
* 重要证件；
* 办事经验。

档案本身保存：

```text
metadata + description + ACL
```

文件通过：

```text
attachments
```

关联。

---

# 83. 医疗功能边界

医疗功能后期可以复用：

```text
archive
matter
reminder
attachment
ACL
```

基础能力。

第一阶段不单独开发复杂医疗系统。

系统不：

* 判断疾病；
* 推荐药物；
* 替代医院病历；
* 进行医疗诊断。

---

# 84. 提醒系统

提醒数据可以来自：

```text
reminders
```

关联：

* Matter；
* Task；
* Archive；
* 其他业务。

V1 至少支持：

> 系统内提醒。

不要求第一阶段完成：

* 微信模板消息；
* 短信；
* 邮件；
* Push；
* Telegram。

---

# 85. 日历日期

对于：

* 百日；
* 对年；
* 三年；
* 入祠；

允许同时保存：

```text
公历日期
农历说明
```

第一阶段不要求系统自动完成所有传统节日和地方习俗日期计算。

允许管理员人工确定日期。

---

# 86. 首页聚合

`/` 登录后的首页至少聚合：

### 公共财产

```text
公共财产净额
待报销总额
主要持有人当前余额
```

### 最近事务

```text
即将到期
进行中
待处理
```

### 最近活动

```text
新增消费
完成报销
内部转账
事务更新
```

### 公告

```text
重要家族信息
```

首页原则：

> 只做已有数据的聚合。

不能另外维护一份独立首页数据源。

---

# 87. 后台功能范围

`/admin` V1 至少包含：

## 成员

* 查看成员；
* 创建成员；
* 禁用成员；
* 审核加入申请；
* 绑定微信身份。

## 权限

* 设置角色；
* 设置敏感资源权限。

## 公共财产

* 查看全账；
* 必要的管理操作；
* 查看不一致告警。

## 审计

* 查看操作历史；
* 按用户、对象、时间过滤。

## 系统

* 系统配置；
* 备份；
* 数据库状态。

---

# 88. 普通成员编辑公共消费

依据业务需求：

> 公共消费记录可以允许家族成员编辑。

技术上必须同时满足：

1. 当前成员拥有编辑权限；
2. 使用乐观锁避免覆盖；
3. 修改与 Audit 同事务；
4. 修改后相关余额立即重新计算；
5. 不更新任何历史 `before_balance` / `after_balance`。

---

# 89. 财务一致性检查

建议建立服务：

```text
LedgerConsistencyChecker
```

至少检查：

```text
sum(holder balances)
-
sum(pending reimbursements)
==
public asset net
```

同时检查：

* 报销不能超过允许报销金额；
* 内部转账转出人不能等于转入人；
* 金额必须大于 0；
* 被撤销记录不能参与统计；
* 不存在已删除成员导致的孤立账目；
* 外键关系有效。

管理员后台显示：

```text
账务正常
```

或者：

```text
发现 ¥xxx 差异
```

---

# 90. 不允许自动“修平”

如果发现账务不一致：

> 系统不得偷偷创建 adjustment。

只能报告问题。

管理员调查后明确：

* 原因；
* 金额；
* 依据；

才能手工添加：

```text
ADJUSTMENT
```

并产生完整 Audit。

---

# 91. 配置管理

非敏感配置可以使用：

```text
config.yaml
```

或者环境变量。

敏感数据必须：

> 与普通配置分离。

例如：

```text
WECHAT_APP_SECRET
FMLYSYS_MASTER_KEY
```

不得提交 Git。

---

# 92. 推荐目录结构

初始可按以下结构组织：

```text
FmlySys/
├── cmd/
│   └── fmlysys/
│       └── main.go
│
├── internal/
│   ├── config/
│   ├── db/
│   ├── auth/
│   ├── admin/
│   ├── member/
│   ├── asset/
│   ├── estate/
│   ├── matter/
│   ├── decision/
│   ├── archive/
│   ├── attachment/
│   ├── reminder/
│   ├── audit/
│   └── httpserver/
│
├── migrations/
│
├── web/
│   ├── templates/
│   └── static/
│
├── data/
│   ├── fmlysys.db
│   └── uploads/
│
├── doc/
│   ├── prompt/
│   │   └── first.md
│   ├── requestment.md
│   └── technical-requestment.md
│
├── go.mod
├── go.sum
└── README.md
```

实际命名可以调整。

但业务模块之间应保持边界。

---

# 93. Go 内部依赖原则

例如：

```text
HTTP Handler
    ↓
Service
    ↓
Repository / DB
```

Handler 不应直接写大量 SQL。

例如：

```text
ExpenseHandler
→ ExpenseService
→ ExpenseRepository
```

账务影响统一经过：

```text
AssetService
```

或者对应 Ledger Service。

避免：

> 不同 Handler 各自实现一套余额增减逻辑。

---

# 94. 金额计算必须集中

例如以下页面：

* 首页；
* 公共财产；
* 成员持有金额；
* 消费详情；
* 报销详情；

都不能自己实现不同版本的余额算法。

统一调用：

```text
LedgerService
```

输出：

```text
GetPublicAssetBalance()
GetHolderBalance(memberID)
GetExpenseBeforeBalance(expenseID)
GetExpenseAfterBalance(expenseID)
GetPendingReimbursement(memberID)
```

确保整个系统只有一套财务语义。

---

# 95. 数据排序与历史余额

历史财务计算不能简单依赖：

```text
created_at
```

因为用户可能今天补录：

> 上个月发生的一笔消费。

财务事件至少区分：

```text
occurred_at
created_at
```

计算历史账时主要依据：

```text
occurred_at
```

若两个事件发生时间完全相同，应建立稳定排序，例如：

```text
occurred_at ASC,
id ASC
```

保证结果可重复计算。

---

# 96. 补录历史数据

系统必须允许：

> 今天录入过去发生的业务。

例如：

```text
录入时间：8月21日
实际消费日期：8月1日
```

加入后：

> 8月1日之后所有历史余额显示自动重新计算。

这也是不能保存死余额的重要原因。

---

# 97. 日志

应用日志用于：

* 服务启动；
* HTTP Error；
* 数据库错误；
* 微信认证错误；
* 管理登录；
* 文件错误；
* 系统任务错误。

应用日志与：

```text
audit_logs
```

不是同一个概念。

### Application Log

解决：

> 系统发生了什么技术问题。

### Audit Log

解决：

> 谁修改了什么业务数据。

两者必须分开。

---

# 98. 日志隐私

应用日志禁止输出：

* 管理员密码；
* TOTP Secret；
* Session Cookie；
* 微信 App Secret；
* OAuth Access Token；
* 敏感文件正文；
* 完整身份证号码等敏感资料。

---

# 99. Request ID

每个 HTTP 请求建议产生：

```text
request_id
```

Audit 和 Application Log 都可以带：

```text
request_id
```

方便出现异常时串联：

```text
用户操作
→ HTTP 请求
→ 数据修改
→ Audit
→ Error Log
```

---

# 100. HTTPS

如果系统通过互联网访问：

> 强制 HTTPS。

Go 服务推荐监听：

```text
127.0.0.1
```

由：

* Caddy；
* Nginx；

等反向代理处理 TLS。

---

# 101. 系统“非公开”含义

本项目所谓：

> 基本没有对外开放系统的需求

表示：

* 不面向社会注册；
* 不提供公共内容社区；
* 不提供第三方 API；
* 不追求搜索引擎收录；
* 只有家族成员和管理员使用。

它不意味着：

> 可以忽略 Web 安全。

只要系统能够从互联网访问：

* 登录；
* CSRF；
* Session；
* HTTPS；
* 上传安全；
* 权限；

仍必须完整实现。

---

# 102. 可进一步限制后台

若部署条件允许，`/admin` 可以在反向代理层额外增加：

* VPN；
* 固定 IP；
* IP 白名单；
* 私有网络访问。

但这是：

> 额外保护层。

不能替代：

> 管理员密码 + TOTP。

---

# 103. 安全 Header

建议至少：

```text
X-Content-Type-Options: nosniff
Referrer-Policy
Content-Security-Policy
```

并根据 HTTPS 部署情况配置：

```text
Strict-Transport-Security
```

---

# 104. SQL Injection

所有 SQL 必须使用：

> 参数绑定。

禁止：

```go
"SELECT ... WHERE id=" + id
```

方式拼接用户输入。

---

# 105. XSS

所有服务端模板默认通过：

```go
html/template
```

转义。

用户填写：

* 事务说明；
* 备注；
* 公告；

第一阶段默认：

> 纯文本。

不要在 V1 直接开放任意 HTML。

如未来加入 Markdown，应统一进行 HTML Sanitization。

---

# 106. 备份要求

这个系统的数据长期价值高于系统代码本身。

因此备份必须作为：

> V1 基础能力。

至少需要备份：

```text
SQLite Database
+
uploads/
```

二者必须匹配。

---

# 107. 推荐备份方式

提供 CLI：

```text
fmlysys backup
```

输出：

```text
backup/
└── fmlysys-20260821-xxxx.tar.gz
```

包含：

```text
database snapshot
uploads
manifest.json
```

Manifest 至少记录：

```text
系统版本
数据库 schema 版本
备份时间
文件数量
数据库 SHA-256
```

---

# 108. SQLite 一致性备份

运行中的 SQLite 不应简单：

> 无脑复制 db 文件。

应使用：

* SQLite Backup API；
* `VACUUM INTO`；
* 或其他一致性快照方式。

然后再与附件目录组成备份。

---

# 109. 恢复

必须提供明确恢复流程。

例如：

```text
停止 FmlySys
↓
备份当前数据
↓
恢复数据库
↓
恢复 uploads
↓
执行 integrity check
↓
启动
```

后续可以建设：

```text
fmlysys restore
```

但 restore 属于高风险操作，不能通过一个无二次确认的普通 Web 按钮直接执行。

---

# 110. SQLite 完整性检查

管理员后台可以显示最近检查结果。

维护时至少支持：

```sql
PRAGMA integrity_check;
```

备份恢复完成后必须能够执行完整性检查。

---

# 111. 数据导出

后续应允许导出：

* 公共资产流水；
* 消费记录；
* 报销；
* 遗产；
* 家族事务。

第一阶段优先：

```text
CSV
JSON
```

PDF 报告不是核心技术要求。

---

# 112. 数据导入

V1 不提供通用：

> 任意 CSV 自动导入所有业务数据。

初始历史数据如果需要批量迁移，应通过：

* 专用迁移脚本；
* 或管理员导入工具；

按具体数据格式开发。

避免建设一个复杂、容易导入错误的万能导入器。

---

# 113. 性能目标

鉴于使用人数少，第一阶段基本目标：

普通页面：

```text
数据库本地查询一般 < 100 ms
```

普通请求：

```text
目标 < 500 ms
```

附件上传/下载除外。

不为了：

```text
10,000 QPS
```

设计系统。

---

# 114. 数据规模预期

SQLite 应能够轻松覆盖：

* 数十名成员；
* 数万条财务记录；
* 数万条事务记录；
* 多年操作历史。

真正可能快速增长的是：

> 附件。

因此附件不放 SQLite BLOB。

---

# 115. 分页

以下页面必须分页：

* 财务流水；
* 公共消费；
* Audit；
* 档案；
* 家族事务；
* 附件列表。

默认：

```text
20～50 条/页
```

避免长期运行后一次加载全部历史。

---

# 116. 搜索

V1 可以使用 SQLite：

```text
LIKE
```

完成基础搜索。

例如：

* 消费用途；
* 事务名称；
* 成员；
* 档案标题。

第一阶段不引入 Elasticsearch。

如果未来数据显著增加，可以评估：

> SQLite FTS5。

---

# 117. 定时任务

V1 如需要：

* 提醒生成；
* Session 清理；
* 临时文件清理；

可在 Go 单体应用内运行轻量 Scheduler。

不引入：

* Celery；
* Kafka；
* 独立 Worker 集群。

重要业务状态不能只存在内存 Scheduler 中。

---

# 118. 系统启动

启动过程建议：

```text
读取配置
↓
检查 Data Directory
↓
打开 SQLite
↓
设置 PRAGMA
↓
执行 Migration
↓
验证管理员配置
↓
加载模板
↓
启动 HTTP Server
```

关键步骤失败：

> 服务直接退出。

不得在数据库不可正常使用时仍启动一个半残系统。

---

# 119. Graceful Shutdown

Go 服务收到：

```text
SIGTERM
SIGINT
```

时：

* 停止接受新请求；
* 等待进行中请求；
* 结束后台任务；
* Flush Log；
* 正常关闭数据库。

---

# 120. 运行形式

项目目标：

> 尽可能以一个 Go 二进制完成部署。

例如：

```text
fmlysys
data/
config/
```

即可运行。

不要求 Docker。

但可以提供 Docker 作为可选部署方式。

---

# 121. Docker 边界

如果提供 Docker：

SQLite 和附件目录必须挂载持久卷：

```text
/data
```

不得把数据库保存在：

> 容器临时文件系统。

---

# 122. 不需要高可用集群

第一阶段不支持：

```text
两个 FmlySys 实例同时读写同一个 SQLite
```

标准部署为：

> 单实例。

如果未来需要多实例：

> 再评估数据库迁移到 PostgreSQL。

当前不提前建设。

---

# 123. 测试要求

## 单元测试

重点覆盖：

* 金额计算；
* 持有人余额；
* 垫付；
* 部分报销；
* 全额报销；
* 内部转账；
* 修改旧消费后的余额重算；
* 撤销；
* 退款；
* 财务一致性。

---

# 124. 财务核心测试案例

必须至少覆盖：

### Case 1

```text
初始：
A 30,000
B 20,000

总额：
50,000
```

### Case 2

```text
A → B 10,000

A = 20,000
B = 30,000
总额 = 50,000
```

### Case 3

```text
C 垫付 2,000

A+B = 50,000
待报销 = 2,000
净额 = 48,000
```

### Case 4

```text
B 报销 C 1,000

A+B = 49,000
待报销 = 1,000
净额 = 48,000
```

### Case 5

```text
B 再报销 C 1,000

A+B = 48,000
待报销 = 0
净额 = 48,000
```

---

# 125. 修改历史测试

例如：

```text
消费原金额 328
↓
修改为 338
```

验证：

* 当前消费金额 = 338；
* Audit 存在 328 → 338；
* 该消费之前余额不变；
* 该消费之后余额重新计算；
* 后续所有流水余额重新计算；
* 不存在旧 after_balance 残留。

---

# 126. 撤销测试

新增：

```text
消费 ¥500
```

然后撤销。

验证：

* 流水历史仍能看到；
* Audit 可看到撤销；
* 当前余额恢复；
* 被撤销消费不参与计算；
* 数据库原记录仍存在。

---

# 127. 权限测试

至少覆盖：

* 未登录不能看家族数据；
* 待审核用户不能进入系统；
* 普通成员不能进入 `/admin`；
* 无权限成员不能读取敏感附件；
* 禁用成员 Session 失效；
* 后台 TOTP 错误不能登录；
* CSRF 缺失不能修改数据。

---

# 128. SQLite 集成测试

集成测试建议直接创建：

> 临时 SQLite 数据库。

真实执行：

* Migration；
* Insert；
* Update；
* Transaction；
* Foreign Key；
* Recalculation。

避免核心财务逻辑只 Mock 数据库。

---

# 129. V1 第一阶段实施顺序

建议按以下顺序开发。

## Phase 1：技术骨架

* Go 项目；
* 配置；
* SQLite；
* Migration；
* HTTP Router；
* Template；
* Static；
* Audit 基础；
* Session 基础。

## Phase 2：后台

* Admin；
* Password；
* Google Authenticator；
* 成员管理；
* 权限。

## Phase 3：普通成员身份

* 微信认证抽象；
* 微信登录；
* Join Request；
* 审核。

## Phase 4：公共财产核心

* 初始资产；
* 持有人；
* 公共消费；
* 垫付；
* 报销；
* 内部转账；
* 资产划出；
* 调整；
* 动态余额；
* 一致性检查。

## Phase 5：附件

* 图片；
* 票据；
* PDF；
* 权限读取。

## Phase 6：事务

* Matter；
* 子事务；
* 负责人；
* 参与人；
* 财务关联。

## Phase 7：遗产

* 遗产事项；
* 遗产清单；
* 分配；
* 划入公共财产。

## Phase 8：长期能力

* 首页聚合；
* 时间轴；
* 提醒；
* 档案；
* 决议。

---

# 130. V1 明确不做

第一阶段不开发：

* 银行账户连接；
* 微信余额读取；
* 支付宝余额读取；
* 自动获取银行卡账单；
* 自动同步私人金融账户；
* 复式企业会计；
* 发票税务系统；
* 自动遗产法律判定；
* AI 自动决定家族事务；
* 医疗诊断；
* 完整医院病历系统；
* 复杂家谱引擎；
* OCR 大规模票据自动记账；
* Elasticsearch；
* Redis；
* 消息队列；
* 微服务；
* Kubernetes；
* 多区域部署；
* 多实例 SQLite 集群；
* 对外开放 API；
* 对公众开放注册。

---

# 131. 不记录公共财产所在具体私人账户

这是项目长期固定边界之一。

系统记录：

```text
A 当前代管 20,000
```

但不记录：

```text
A 微信 3,000
A 支付宝 2,000
A 中国银行 15,000
```

支付渠道只针对：

> 单笔具体交易。

这一原则不得在实现过程中因为“方便统计”被悄悄改变。

---

# 132. 不将系统变成个人资产管理器

FmlySys 管理：

> 家族公共财产。

不管理：

* 成员个人存款；
* 收入；
* 信用卡；
* 证券；
* 个人负债；
* 私人消费。

系统必须保持：

> 公共资产透明

与：

> 个人财务隐私

之间的边界。

---

# 133. 不将虚拟账户误实现为金融账户

所谓：

```text
A 公共资产虚拟账户
```

仅代表：

> A 当前代管公共资产的账面额度。

不存在：

* 开户；
* 银行账号；
* 支付能力；
* 实际转账接口；
* 自动扣款。

真实支付仍然在系统外发生。

系统只负责：

> 记录真实世界已经发生的事实。

---

# 134. 系统不发起资金转账

例如内部记录：

```text
A → B ¥10,000
```

FmlySys 不调用：

* 微信支付；
* 支付宝；
* 银行；

实际划款。

流程是：

```text
现实完成转账
↓
在 FmlySys 登记
↓
上传支付凭证
```

---

# 135. 系统不作为法律确权系统

FmlySys 中：

* 遗产分配记录；
* 家族决议；
* 宅基地资料；

属于：

> 家族内部信息记录。

系统不自动改变现实法律权属。

---

# 136. 系统不作为不可篡改区块链

Audit 的目标：

> 使正常应用操作不能无痕修改。

系统不承诺：

> 拥有服务器 Root 权限的人也无法篡改数据。

因此不引入：

* 区块链；
* 分布式共识；
* 公证链；

等技术。

如未来确有取证需求，再单独建设：

> 外部签名备份或 Hash Chain。

---

# 137. 数据长期可迁移原则

数据库使用 SQLite。

附件使用普通文件。

配置使用通用文本格式。

必须避免把核心数据绑定到：

> 某个云厂商私有服务。

长期目标：

```text
SQLite DB
+
uploads
+
配置
```

即可完整迁移 FmlySys。

---

# 138. 代码长期可维护原则

业务核心应保持清晰。

特别是：

```text
Public Asset
Holder
Expense
Advance
Reimbursement
Transfer
Matter
Estate
Audit
```

这些术语必须在：

* 数据库；
* Go；
* API；
* 页面；

中尽可能保持一致。

避免同一个概念在不同模块被分别称为：

```text
account
wallet
fund
money_holder
keeper
```

导致长期语义混乱。

---

# 139. 推荐核心领域名称

统一建议：

```text
Member
PublicAsset
AssetHolder
Expense
Reimbursement
HolderTransfer
Estate
Matter
Decision
Archive
Attachment
AuditLog
```

中文界面可分别显示：

```text
家族成员
公共财产
公共资产持有人
公共消费
报销
公共资产内部转账
遗产事务
家族事务
家族决议
家族档案
附件
操作记录
```

---

# 140. 技术设计最终原则

FmlySys 第一阶段技术实现应始终坚持：

### 1. 单体优先

一个 Go 服务足以完成当前需求。

### 2. SQLite 优先

当前规模不存在引入大型数据库的必要。

### 3. 数据事实优先

保存真实业务事件，而不是保存无法追溯来源的余额数字。

### 4. 余额可重算

公共财产余额、成员持有余额和历史余额均能够从有效业务记录重新计算。

### 5. 修改可追溯

允许修正错误，但不能无痕修改。

### 6. 删除可追溯

重要业务记录采用逻辑撤销，不通过普通页面物理删除。

### 7. 公私边界明确

记录成员代管公共资产总额，但不记录该成员私人金融账户的具体资金分布。

### 8. 财务与事务关联

系统不仅知道：

> 花了多少钱，

还应知道：

> 为什么花。

### 9. 身份与业务解耦

微信、Google Authenticator 都属于认证机制，不得成为业务数据模型的一部分。

### 10. 后台独立认证

`/admin` 不依赖普通家族成员登录态。

### 11. 附件必须受权限控制

不得因为文件存在服务器就可以通过 URL 任意访问。

### 12. 备份属于核心功能

FmlySys 的长期价值主要存在于数据库和家族档案，而不是代码本身。

### 13. 不过度设计

没有实际需求时，不增加 Redis、消息队列、微服务、集群和复杂云服务。

### 14. 为多年使用设计

今天只有几条遗产和消费记录，也必须保证十年后仍然可以解释：

> 这笔钱从哪里来；
> 当时是谁代管；
> 为什么支出；
> 谁先垫付；
> 谁完成报销；
> 哪条记录后来修改过；
> 为什么修改；
> 当时对应什么家族事务。

---

# 141. 第一版完成判定

当以下流程能够完整闭环时，可以认为 FmlySys 已具备第一版核心技术基础：

```text
管理员初始化
↓
Google Authenticator 登录 /admin
↓
建立家族成员
↓
成员通过微信身份申请加入
↓
管理员批准
↓
成员登录 /
↓
录入初始公共资产
↓
指定 A/B 为资产持有人
↓
A → B 公共资产内部转账
↓
成员登记公共消费
↓
出现个人垫付
↓
持有人分次完成报销
↓
所有余额自动正确计算
↓
修改历史消费金额
↓
历史余额自动重新计算
↓
Audit 能完整还原修改过程
↓
消费关联家族事务
↓
上传账单/照片/PDF
↓
无权限成员无法读取敏感附件
↓
完整备份 SQLite + 附件
↓
可从备份恢复系统
```

以上链路是第一阶段最重要的验收基线。

只有在这套基础稳定之后，再继续扩展：

* 家族长期档案；
* 复杂决议；
* 医疗备忘；
* 家谱；
* 更多提醒方式；
* OCR；
* 其他高级功能。
