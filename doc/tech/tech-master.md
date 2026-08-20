# FmlySys 初始技术需求文档

> 文件：`doc/tech/tech-master.md`
> 文档状态：Initial Draft / V0.2
> 对应业务需求：`doc/requestment.md`
> 初始讨论参考：`doc/prompt/first.md`

---

# 1. 文档目的

本文档在《家族公共事务与共同资产治理系统需求文档》的基础上，明确 FmlySys 第一阶段开发所采用的技术架构、数据模型原则、认证方式、权限边界、接口边界、数据安全、审计、附件存储、系统数据分区、部署和备份等技术要求。

本文档重点回答：

* 系统使用什么技术实现；
* 前台、后台如何划分；
* SQLite 如何承载公共财产及事务数据；
* 系统数据分区如何组织；
* 公共财产余额如何计算；
* 如何保证历史修改可追溯；
* 家族成员与管理员如何认证；
* 附件和账单凭证如何保存；
* 如何备份本系统数据；
* 如何一键将数据备份至 Google Drive；
* 如何从外部备份文件导入数据且不覆盖当前系统数据；
* 哪些技术能力属于第一阶段；
* 哪些能力明确暂不建设；
* 系统规模较小时应避免哪些过度设计。

本文档不是完整数据库 DDL、API Swagger 或 UI 设计稿。

后续可以基于本文继续拆分：

* 数据库设计文档；
* API 文档；
* 前端交互设计；
* 部署文档；
* 备份与恢复文档；
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
* 数据分区管理；
* 附件管理；
* 审计日志；
* 提醒计算；
* 备份与导入；
* Google Drive 备份；
* 系统管理。

第一阶段不拆分微服务。

---

# 4. 数据存储与系统数据分区

使用：

> **SQLite**

SQLite 作为系统结构化数据的唯一权威持久化存储。

但 FmlySys 不应永久假定整个实例只有唯一一份业务数据库。

因为系统需要支持：

> 从外部备份文件导入历史系统数据，但不覆盖现有系统数据，而是建立新的独立系统数据分区。

因此从初始架构开始引入：

> **System Data Partition / 系统数据分区**

概念。

---

## 4.1 系统级数据与业务数据分离

推荐采用：

```text
data/
├── system.db
│
├── partitions/
│   ├── p_default/
│   │   ├── fmlysys.db
│   │   └── uploads/
│   │
│   ├── p_xxxxx/
│   │   ├── fmlysys.db
│   │   └── uploads/
│   │
│   └── ...
│
├── backup/
└── temp/
```

其中：

```text
system.db
```

负责保存 FmlySys 实例级数据。

而：

```text
partitions/<partition_id>/
```

表示一套完整、独立的家族业务数据。

---

## 4.2 system.db

系统级数据库用于保存不属于任何一个具体家族业务数据集的内容。

例如：

* 后台管理员账户；
* Google Authenticator TOTP 配置；
* 后台管理员 Session；
* 数据分区注册信息；
* 当前活动数据分区；
* Google Drive OAuth 授权信息；
* Google Drive Refresh Token；
* 备份任务记录；
* 全局系统配置；
* 系统级操作日志。

这些信息不会因为切换家族业务数据分区而改变。

---

## 4.3 数据分区数据库

每个数据分区拥有独立：

```text
fmlysys.db
```

其中保存：

* 家族成员；
* 微信登录身份；
* 普通成员 Session；
* 家族成员权限；
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
* 档案；
* 附件元数据；
* 业务操作审计。

同时拥有自己独立的：

```text
uploads/
```

目录。

---

## 4.4 为什么数据分区采用独立 SQLite

不采用：

```text
所有业务表增加 partition_id
```

然后把所有数据分区强行塞进一个大型 SQLite 数据库。

原因包括：

1. 外部备份导入容易发生 ID 冲突；
2. 不同备份可能对应不同数据库 Schema Version；
3. 容易误把两个数据集中的成员、事务和附件关联起来；
4. 导入失败可能污染当前正常运行的数据；
5. 删除一个导入的数据分区非常困难；
6. 失去真正的数据隔离；
7. 备份、迁移和恢复复杂度反而增加。

因此：

> **一个业务数据分区 = 一个独立 SQLite 数据库 + 一个独立 uploads 目录。**

---

## 4.5 默认数据分区

系统第一次初始化时自动建立：

```text
p_default
```

或其他随机唯一 ID。

该分区成为：

> 默认活动数据分区。

在没有进行备份导入等操作时，用户不会明显感知“数据分区”的存在。

---

## 4.6 活动数据分区

系统任意时刻有且仅有一个：

> **Active Partition / 活动数据分区**

普通成员访问：

```text
/
```

以及：

```text
/api/v1/*
```

时，业务请求默认绑定当前活动数据分区。

客户端不得自行通过参数：

```text
partition_id
```

访问任意其他分区。

数据分区选择属于服务端控制范围。

---

## 4.7 数据分区注册表

`system.db` 中建议建立：

```text
data_partitions
```

至少保存：

```text
id
partition_uuid
display_name

source_type
source_partition_uuid
source_backup_id

path

app_version
schema_version

created_at
imported_at
last_opened_at

status
is_active
```

`source_type` 可以包括：

```text
INITIAL
IMPORTED_FILE
GOOGLE_DRIVE_BACKUP
MANUAL
```

---

## 4.8 数据分区之间不自动合并

V1 明确不提供：

```text
Partition A
+
Partition B
=
Merged Partition
```

能力。

特别禁止自动：

* 合并同名成员；
* 合并公共财产流水；
* 合并事务；
* 合并附件；
* 根据姓名猜测两个人是同一个人；
* 根据金额猜测两条记录属于同一笔业务。

导入备份的语义始终是：

> **创建一个新的、独立的数据分区。**

不是：

> 把数据追加进当前分区。

---

# 5. SQLite 使用原则

所有 SQLite 数据库打开后至少启用：

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

一个业务请求只能明确绑定：

> 一个业务数据分区数据库。

禁止在一个普通业务事务中跨两个业务分区修改数据。

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
   ├── Ledger Service
   ├── Partition Manager
   ├── Backup / Import
   ├── Google Drive Backup
   ├── Audit
   ├── Attachment Service
   │
   ├── system.db
   │
   └── Active Data Partition
       ├── fmlysys.db
       └── uploads/
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
/admin/data-partitions
/admin/data-partitions/import
```

后台负责：

* 家族成员审核；
* 权限管理；
* 系统配置；
* 后台财务管理；
* 敏感资料权限；
* 审计查看；
* 数据分区管理；
* Google Drive 备份；
* 外部备份导入；
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

普通成员 Session 还必须绑定创建该 Session 时的：

> 数据分区。

切换活动数据分区后，应使普通成员 Session 失效并重新认证，防止旧 Session 继续操作新分区。

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
* 不建设第三方业务 OAuth；
* 不提供 CORS 公共调用；
* 不提供开放平台；
* 不承诺 API 长期向第三方兼容。

Google Drive OAuth 属于系统备份功能所需的外部服务授权，不属于面向第三方开发者开放 API。

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

成员和身份属于：

> 当前业务数据分区。

不同数据分区中的成员不存在自动关联关系。

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
身份尚未绑定当前数据分区成员
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

处理当前活动数据分区中的申请。

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

后台管理员属于系统级身份：

> 不随业务数据分区切换。

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

管理员账户保存在：

```text
system.db
```

建议建立：

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
* 保存在浏览器 LocalStorage；
* 包含在普通业务数据分区备份中。

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

后台 Session 保存于：

```text
system.db
```

普通成员 Session 保存于对应业务分区，或者保存在 `system.db` 中并明确记录 `partition_id`。

无论采取哪种方式：

> 普通成员 Session 必须明确绑定数据分区。

推荐字段：

```text
id
token_hash
actor_id
partition_id
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

成员角色属于：

> 当前业务数据分区。

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

系统级数据库和业务数据分区应分别维护 Migration。

例如：

```text
migrations/
├── system/
│   ├── 000001_init.sql
│   └── ...
│
└── partition/
    ├── 000001_init.sql
    ├── 000002_asset_ledger.sql
    └── ...
```

数据库记录：

```text
schema_migrations
```

每次启动：

1. 检查 `system.db` Schema Version；
2. 执行系统数据库未运行 Migration；
3. 检查当前活动数据分区；
4. 执行活动分区未运行 Migration；
5. Migration 失败则终止启动；
6. 不允许带着半升级数据库继续工作。

外部备份导入时：

> Migration 只能作用于刚创建的新数据分区副本。

不得为了兼容导入文件而修改现有活动数据分区。

如果外部备份 Schema Version 高于当前程序支持版本：

> 拒绝导入，并提示先升级 FmlySys。

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

数据库访问层必须明确区分：

```text
SystemRepository
```

与：

```text
PartitionRepository
```

避免业务代码误将系统级数据和分区业务数据混写。

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

该一致性关系只在：

> 同一业务数据分区内部

计算。

不同数据分区的公共财产不得混合统计。

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

业务数据分区建立统一：

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

系统级操作另外记录：

```text
system_audit_logs
```

用于：

* 切换数据分区；
* 导入备份；
* Google Drive 授权；
* Google Drive 备份；
* 修改管理员；
* 系统设置修改。

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

任何重要业务数据修改：

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

系统级配置修改与系统级 Audit 也遵循同样规则。

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

## system.db

建议至少包含：

```text
schema_migrations

admin_users
admin_sessions

data_partitions

google_drive_connections
backup_records

system_settings
system_audit_logs
```

## 每个业务数据分区

建议至少包含：

```text
schema_migrations

members
member_identities
join_requests

roles
member_roles
resource_acl

sessions

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

每个数据分区分别保存自己的附件：

```text
data/
└── partitions/
    └── <partition_id>/
        ├── fmlysys.db
        └── uploads/
```

数据库只保存：

> 附件元数据。

不同数据分区不得共用同一个 `uploads/` 目录。

这样才能保证：

> 备份一个数据分区时，其数据库与附件天然形成完整数据单元。

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

1. 确定当前活动数据分区；
2. 检查当前成员身份；
3. 检查成员是否拥有查看业务对象的权限；
4. 从对应分区的 `uploads/` 读取文件。

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

`/` 登录后的首页至少聚合当前活动数据分区中的：

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

不同数据分区之间不做首页数据聚合。

---

# 87. 后台功能范围

`/admin` V1 至少包含：

## 成员

* 查看当前分区成员；
* 创建成员；
* 禁用成员；
* 审核加入申请；
* 绑定微信身份。

## 权限

* 设置角色；
* 设置敏感资源权限。

## 公共财产

* 查看当前数据分区全账；
* 必要的管理操作；
* 查看不一致告警。

## 审计

* 查看业务操作历史；
* 按用户、对象、时间过滤；
* 查看系统级管理操作历史。

## 数据分区

* 查看全部数据分区；
* 查看当前活动分区；
* 查看分区来源；
* 切换活动数据分区；
* 导入外部备份为新数据分区。

## 备份

* 创建本地完整备份；
* 一键备份当前数据分区至 Google Drive；
* 查看备份记录；
* 查看备份成功/失败状态；
* 管理 Google Drive 授权。

## 系统

* 系统配置；
* 数据库状态；
* SQLite 完整性检查。

---

# 88. 普通成员编辑公共消费

依据业务需求：

> 公共消费记录可以允许家族成员编辑。

技术上必须同时满足：

1. 当前成员拥有编辑权限；
2. 当前成员 Session 属于当前活动数据分区；
3. 使用乐观锁避免覆盖；
4. 修改与 Audit 同事务；
5. 修改后相关余额立即重新计算；
6. 不更新任何历史 `before_balance` / `after_balance`。

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

检查范围始终限定：

> 一个具体业务数据分区。

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

GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
```

不得提交 Git。

Google Drive OAuth Refresh Token 等动态秘密应：

* 保存在 `system.db`；
* 使用 `FMLYSYS_MASTER_KEY` 加密；
* 不进入普通业务数据分区备份。

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
│   ├── partition/
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
│   ├── backup/
│   ├── googledrive/
│   └── httpserver/
│
├── migrations/
│   ├── system/
│   └── partition/
│
├── web/
│   ├── templates/
│   └── static/
│
├── data/
│   ├── system.db
│   │
│   ├── partitions/
│   │   ├── p_default/
│   │   │   ├── fmlysys.db
│   │   │   └── uploads/
│   │   └── ...
│   │
│   ├── backup/
│   └── temp/
│
├── doc/
│   ├── prompt/
│   │   └── first.md
│   ├── requestment.md
│   └── tech/
│       └── tech-master.md
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

数据分区访问统一经过：

```text
PartitionManager
```

避免业务 Handler 自行拼接数据分区文件路径。

备份统一经过：

```text
BackupService
```

避免后台页面直接复制数据库文件。

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

LedgerService 必须绑定一个明确的：

```text
PartitionContext
```

确保整个系统只有一套财务语义，同时不会跨数据分区计算。

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
* 数据分区切换错误；
* 微信认证错误；
* 管理登录；
* 文件错误；
* 备份错误；
* Google Drive API 错误；
* 数据导入错误；
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

### System Audit Log

解决：

> 谁执行了数据分区、备份、导入等系统管理操作。

三者职责必须区分。

---

# 98. 日志隐私

应用日志禁止输出：

* 管理员密码；
* TOTP Secret；
* Session Cookie；
* 微信 App Secret；
* OAuth Access Token；
* Google OAuth Refresh Token；
* Google Client Secret；
* FMLYSYS_MASTER_KEY；
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

备份和导入任务还可以单独产生：

```text
operation_id
```

用于追踪一次完整长操作。

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

Google OAuth 回调也必须使用受支持的安全回调地址。

---

# 101. 系统“非公开”含义

本项目所谓：

> 基本没有对外开放系统的需求

表示：

* 不面向社会注册；
* 不提供公共内容社区；
* 不提供第三方业务 API；
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

Google Drive 属于管理员主动配置的外部备份服务，不改变 FmlySys 本身的非公开定位。

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

尤其：

* 数据分区切换；
* 外部备份导入；
* Google Drive 授权；
* 备份管理；

均只能通过管理员权限操作。

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

数据分区数据库路径也不得直接取自 HTTP 请求参数。

必须根据：

```text
partition_id
```

从 `system.db` 中的合法注册记录解析真实目录。

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

> **V1 基础能力。**

系统至少支持：

1. 创建本地完整业务数据备份；
2. 一键将当前活动数据分区备份至 Google Drive；
3. 从外部 FmlySys 备份文件导入数据；
4. 外部备份导入时创建新的系统数据分区；
5. 导入绝不覆盖原有数据分区。

普通备份的基本单位为：

> **一个完整业务数据分区。**

即：

```text
fmlysys.db
+
uploads/
```

二者必须匹配。

系统级秘密默认不属于普通业务数据备份。

---

## 106.1 “一键备份本系统数据”的含义

后台：

```text
/admin/backup
```

提供：

> 一键备份到 Google Drive

按钮。

默认备份：

> 当前活动业务数据分区的完整业务数据。

包括：

* SQLite 业务数据库；
* 所有有效附件和必要历史附件；
* 数据分区元信息；
* Backup Manifest。

不包括：

* 管理员密码；
* TOTP Secret；
* FMLYSYS_MASTER_KEY；
* Google OAuth Token；
* 微信 App Secret；
* Session；
* 服务器私有配置。

---

# 107. 标准备份文件格式

提供 CLI：

```text
fmlysys backup
```

以及后台备份功能。

统一输出：

```text
backup/
└── fmlysys-backup-v1-20260821-xxxx.tar.gz
```

推荐内部结构：

```text
manifest.json

partition/
├── fmlysys.db
└── uploads/
    ├── ...
    └── ...
```

所有本地备份、Google Drive 备份、外部导入均使用：

> **同一 Backup Package 格式。**

---

## 107.1 Manifest

`manifest.json` 至少记录：

```text
backup_format_version

backup_id

partition_uuid
partition_name

fmlysys_version
schema_version

created_at

database_file
database_size
database_sha256

attachment_count
attachment_total_size

files
```

`files` 可记录：

```text
relative_path
size
sha256
```

确保导入时能够验证：

> 数据库和附件确实属于同一份完整备份。

---

## 107.2 备份包必须自描述

备份文件不得依赖：

* 原服务器绝对路径；
* 原机器用户名；
* Docker Container ID；
* 当前 Partition 本地目录名；
* 当前服务器 IP。

只要 FmlySys 版本兼容：

> Backup Package 应能够在另一套 FmlySys 实例中导入。

---

## 107.3 备份不包含系统秘密

普通业务数据备份不得包含：

```text
system.db
```

中的系统秘密。

特别包括：

* 后台管理员密码 Hash；
* 后台 TOTP Secret；
* Google OAuth Token；
* Google Drive Refresh Token；
* Master Key；
* 后台 Session；
* 当前服务器配置密码。

因此：

> 导入一份家族业务备份不会同时复制另一套服务器的管理员权限。

---

# 108. SQLite 一致性备份

运行中的 SQLite 不应简单：

> 无脑复制 db 文件。

应使用：

* SQLite Backup API；
* `VACUUM INTO`；
* 或其他一致性快照方式。

标准过程：

```text
锁定目标数据分区备份上下文
↓
创建临时工作目录
↓
生成 SQLite 一致性 Snapshot
↓
收集对应 uploads
↓
生成 manifest.json
↓
计算 SHA-256
↓
生成 Backup Package
↓
重新验证 Backup Package
↓
备份完成
```

文件 IO 不应长时间占用 SQLite 写事务。

备份过程中允许系统继续正常提供服务，但生成的数据库快照必须对应一个明确的一致性时间点。

---

# 109. Google Drive 一键备份

FmlySys V1 支持：

> **一键备份当前系统业务数据到 Google Drive 云端硬盘。**

入口：

```text
/admin/backup
```

管理员可以：

1. 首次连接 Google Drive；
2. 查看连接状态；
3. 点击“一键备份到 Google Drive”；
4. 查看备份进度；
5. 查看成功/失败记录。

---

## 109.1 Google Drive 授权

使用：

> Google OAuth 2.0 + Google Drive API

进行授权。

授权仅由：

> 后台管理员

执行。

OAuth 授权信息属于：

> 系统级数据。

不得保存到业务数据分区。

---

## 109.2 Google Drive Token

例如：

```text
access_token
refresh_token
```

不得：

* 写入日志；
* 写入普通业务备份；
* 返回普通成员页面；
* 保存至浏览器 LocalStorage。

Refresh Token 等需要长期保存的敏感凭证，应：

1. 保存于 `system.db`；
2. 使用 `FMLYSYS_MASTER_KEY` 加密。

---

## 109.3 Google Drive 备份目录

系统可以在授权账号的 Google Drive 中创建专用目录：

```text
FmlySys Backups/
```

例如：

```text
FmlySys Backups/
├── fmlysys-backup-v1-20260821-045000.tar.gz
├── fmlysys-backup-v1-20260825-120000.tar.gz
└── ...
```

每次备份默认：

> 新建一个独立文件。

不得默认覆盖之前的备份。

---

## 109.4 Google Drive 备份流程

标准流程：

```text
管理员点击“一键备份到 Google Drive”
↓
确定当前活动数据分区
↓
产生 SQLite 一致性 Snapshot
↓
收集该分区 uploads
↓
生成 manifest
↓
创建 Backup Package
↓
执行本地完整性校验
↓
上传 Google Drive
↓
等待 Google Drive 确认成功
↓
记录远端文件 ID
↓
记录备份成功
↓
清理临时文件
```

---

## 109.5 上传进度

备份界面至少能够展示：

```text
正在准备数据库快照
正在收集附件
正在生成备份文件
正在校验备份
正在上传 Google Drive · 35%
正在上传 Google Drive · 78%
正在确认远端文件
备份完成
```

---

## 109.6 大备份文件

随着：

```text
uploads/
```

增长，备份文件可能逐渐变大。

Google Drive 上传实现应支持：

> 可续传上传。

网络短暂中断时尽可能继续原上传任务，而不是必须从零重新传输整个大文件。

---

## 109.7 Google Drive 备份记录

`system.db` 建议建立：

```text
backup_records
```

至少记录：

```text
id

backup_id
partition_id

destination

file_name
file_size
sha256

google_drive_file_id

started_at
completed_at

status
error_message

created_by_admin_id
```

`destination` 至少包括：

```text
LOCAL
GOOGLE_DRIVE
```

---

## 109.8 Google Drive 失败边界

以下情况：

* Google Drive 未授权；
* OAuth Token 失效；
* 网络中断；
* Google Drive 空间不足；
* API 返回错误；
* 上传后远端校验失败；

必须将：

> Google Drive 备份

标记为失败。

不得因为本地 Backup Package 已经生成，就显示：

> 云端备份成功。

Google Drive 备份失败不得：

* 修改当前业务数据；
* 修改公共财产；
* 修改活动数据分区；
* 导致 FmlySys 业务不可用。

Google Drive 是：

> 备份目的地。

不是：

> FmlySys 运行所依赖的数据库。

---

# 110. 外部备份导入与新数据分区

系统支持管理员从：

> 外部 FmlySys Backup Package

导入数据。

入口：

```text
/admin/data-partitions/import
```

但必须遵循一个不可改变的核心原则：

> **导入备份永远不能覆盖当前原有数据分区。**

导入的语义固定为：

> **Import As New Partition / 导入为新的系统数据分区。**

---

## 110.1 导入示例

当前系统：

```text
Partition A
当前活动
```

管理员导入：

```text
backup-X.tar.gz
```

导入成功后：

```text
Partition A
当前活动

Partition B
由 backup-X 导入
```

而不是：

```text
Partition A
被 backup-X 覆盖
```

---

## 110.2 导入过程

标准流程：

```text
管理员选择备份文件
↓
上传至 data/temp/
↓
检查文件大小和格式
↓
安全解包至临时目录
↓
读取 manifest.json
↓
检查 Backup Format Version
↓
检查 FmlySys / Schema Version
↓
校验数据库 SHA-256
↓
校验附件 Hash
↓
执行 SQLite integrity_check
↓
需要时在新副本执行 Migration
↓
生成新的本地 partition_id
↓
移动至 partitions/<new_partition_id>/
↓
注册 data_partitions
↓
导入完成
```

所有步骤全部成功后：

> 新数据分区才正式出现在系统中。

---

## 110.3 导入失败

如果任何步骤失败：

```text
校验失败
数据库损坏
附件缺失
Schema 不兼容
解压失败
磁盘空间不足
Migration 失败
```

则：

```text
删除临时导入目录
↓
标记导入失败
↓
原系统继续运行
```

导入失败不得影响：

* 当前活动数据库；
* 当前附件；
* 当前家族成员；
* 当前公共财产；
* 当前事务；
* 当前审计历史。

---

## 110.4 导入后不得自动切换

导入成功后：

> 新数据分区默认处于非活动状态。

系统提示：

```text
备份已成功导入为新的数据分区。

当前数据未被覆盖，也未发生改变。
```

如果管理员希望使用新分区：

> 需要再单独执行“切换数据分区”。

即：

```text
Import
```

与：

```text
Switch
```

必须是两个不同操作。

---

## 110.5 同名分区

如果备份中的名称与当前已有数据分区名称相同：

> 仍然建立新的独立 Partition。

例如：

```text
父亲遗产事务
```

已经存在。

再次导入后可以显示：

```text
父亲遗产事务（导入 2026-08-21）
```

但底层必须生成新的：

```text
partition_id
```

---

## 110.6 同一备份重复导入

同一个：

```text
backup_id
```

原则上允许重复导入。

系统可以提示：

```text
该备份此前已经导入过。
继续操作将创建另一个独立数据分区。
```

管理员确认后：

```text
Partition B
Partition C
```

可以同时存在。

不得自动：

* 覆盖 B；
* 合并 B；
* 删除 B。

---

## 110.7 Schema 兼容

如果备份 Schema：

> 低于当前程序支持版本

则允许：

```text
在新分区副本执行 Migration
```

之后导入。

不得修改用户上传的原 Backup Package。

如果备份 Schema：

> 高于当前程序支持版本

则：

> 拒绝导入。

提示：

```text
该备份由较新版本 FmlySys 创建。
请先升级本系统。
```

不得尝试“尽量兼容读取”。

---

## 110.8 SQLite 完整性检查

导入时至少执行：

```sql
PRAGMA integrity_check;
```

只有返回：

```text
ok
```

才能注册为有效数据分区。

---

## 110.9 外部备份属于不可信输入

即使扩展名正确，也必须防止：

* 路径穿越；
* `../`；
* 绝对路径；
* Symbolic Link；
* 解压炸弹；
* 异常巨大文件；
* Manifest 欺骗；
* Hash 不一致；
* 未声明文件。

所有解压内容必须限制在：

```text
data/temp/<import_operation_id>/
```

中。

完成全部校验后才能移动至：

```text
data/partitions/
```

---

# 111. 数据分区管理与恢复

后台提供：

```text
/admin/data-partitions
```

至少显示：

```text
分区名称
分区 ID
当前活动状态

来源
创建时间
导入时间

FmlySys Version
Schema Version

数据库大小
附件数量
附件大小

最近使用时间
```

---

## 111.1 切换数据分区

只有管理员可以切换活动数据分区。

流程：

```text
当前活动：Partition A
↓
管理员选择 Partition B
↓
显示切换确认
↓
关闭/释放旧业务数据库连接
↓
激活 Partition B
↓
更新 system.db
↓
使普通成员 Session 失效
↓
后续请求进入 Partition B
```

切换操作必须记录：

> system_audit_logs。

---

## 111.2 切换不等于复制

切换数据分区：

* 不复制数据；
* 不删除数据；
* 不合并数据；
* 不改变其他 Partition。

只是改变：

> 当前 `/` 使用哪一套业务数据。

---

## 111.3 删除数据分区

删除整个数据分区属于：

> 高风险操作。

V1 可以不提供 Web 删除功能。

如果以后提供：

1. 当前活动分区不能直接删除；
2. 必须二次确认；
3. 明确提示数据库和附件都将被删除；
4. 推荐删除前创建备份；
5. 必须写系统 Audit。

---

## 111.4 普通导入与灾难恢复必须区分

后台：

```text
导入外部备份
```

固定语义是：

```text
Backup
→
New Partition
```

不是：

```text
Backup
→
Overwrite Current Partition
```

如果服务器发生完全损坏，需要从备份重建：

> 属于 Disaster Recovery。

以后可由 CLI：

```text
fmlysys restore
```

负责。

但普通 Web 后台：

> 不提供“一键覆盖当前数据”的 Restore。

---

## 111.5 Google Drive 备份也可作为外部备份来源

Google Drive 中的备份文件与本地标准备份文件格式一致。

管理员可以：

```text
从 Google Drive 下载备份
↓
上传到 FmlySys
↓
导入为新的 Partition
```

V1 不强制实现：

> 在 FmlySys 后台直接浏览 Google Drive 文件并导入。

第一阶段只要求：

1. 一键备份到 Google Drive；
2. 从外部备份文件导入新数据分区。

未来可以再增加：

```text
从 Google Drive 选择备份
↓
直接导入为新分区
```

功能。

---

# 112. 数据导出与其他数据导入

业务数据导出仍可以支持：

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

## 112.1 Backup Import 与普通数据导入不是同一个概念

必须明确区分：

### Backup Import

输入：

```text
FmlySys Backup Package
```

结果：

```text
创建新的完整数据分区
```

属于 V1 核心能力。

### Business Data Import

例如：

```text
CSV
JSON
Excel
```

导入若干业务记录。

V1 不提供万能业务导入器。

初始历史数据如果需要批量迁移，应通过：

* 专用迁移脚本；
* 或管理员专项导入工具；

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

附件上传/下载、备份、Google Drive 上传和外部备份导入除外。

不为了：

```text
10,000 QPS
```

设计系统。

---

# 114. 数据规模预期

每个业务数据分区中的 SQLite 应能够轻松覆盖：

* 数十名成员；
* 数万条财务记录；
* 数万条事务记录；
* 多年操作历史。

真正可能快速增长的是：

> 附件。

因此附件不放 SQLite BLOB。

多个数据分区可以共存，但当前系统不要求同时为多个分区提供高并发访问。

---

# 115. 分页

以下页面必须分页：

* 财务流水；
* 公共消费；
* Audit；
* 档案；
* 家族事务；
* 附件列表；
* 备份记录；
* 数据分区数量较多时的数据分区列表。

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

搜索始终限定：

> 当前活动数据分区。

---

# 117. 定时任务

V1 如需要：

* 提醒生成；
* Session 清理；
* 临时文件清理；
* 失败备份临时目录清理；
* 失败导入临时目录清理；

可在 Go 单体应用内运行轻量 Scheduler。

不引入：

* Celery；
* Kafka；
* 独立 Worker 集群。

重要业务状态不能只存在内存 Scheduler 中。

V1 要求：

> Google Drive 一键手工备份。

不要求自动定时云备份。

以后可以再增加：

```text
每日
每周
每月
```

自动备份计划。

---

# 118. 系统启动

启动过程建议：

```text
读取配置
↓
检查 Data Directory
↓
打开 system.db
↓
设置 PRAGMA
↓
执行 System Migration
↓
读取 Active Partition
↓
检查 Active Partition 目录
↓
打开 Active Partition fmlysys.db
↓
设置 PRAGMA
↓
执行 Partition Migration
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

非活动数据分区原则上不要求启动时全部打开。

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
* 停止新的备份/导入任务；
* 妥善处理中途任务状态；
* 结束后台任务；
* Flush Log；
* 正常关闭活动业务数据库；
* 正常关闭 `system.db`。

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

完整：

```text
/data
```

必须挂载持久卷。

其中包括：

```text
system.db
partitions/
backup/
```

不得只持久化当前活动：

```text
fmlysys.db
```

而遗漏其他数据分区。

不得把：

* 数据库；
* 附件；
* 数据分区；
* 本地备份；

保存在容器临时文件系统。

---

# 122. 不需要高可用集群

第一阶段不支持：

```text
两个 FmlySys 实例同时读写同一个数据目录
```

标准部署为：

> 单实例。

如果未来需要多实例：

> 再评估数据库迁移到 PostgreSQL 或重新设计数据层。

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
* 财务一致性；
* 数据分区解析；
* Backup Manifest；
* Backup Hash；
* 导入校验；
* Partition 切换；
* Google Drive 备份状态机。

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

这些计算必须只发生在同一数据分区。

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
* CSRF 缺失不能修改数据；
* 普通成员不能切换 Partition；
* 普通成员不能导入备份；
* 普通成员不能操作 Google Drive；
* 非活动分区数据不能通过修改 URL 越权读取；
* 切换数据分区后旧普通成员 Session 失效。

---

# 128. SQLite 与备份集成测试

集成测试建议直接创建：

> 临时 system.db + 多个临时 Partition SQLite 数据库。

真实执行：

* Migration；
* Insert；
* Update；
* Transaction；
* Foreign Key；
* Recalculation；
* 创建 Snapshot；
* 打包附件；
* 生成 Manifest；
* Hash 校验；
* 导入备份；
* 新 Partition 注册；
* 数据分区切换；
* SQLite integrity_check。

避免核心财务逻辑只 Mock 数据库。

同时必须覆盖：

### Partition Import Case

```text
当前 Partition A
↓
创建 A 的 Backup Package
↓
重新导入 Backup Package
↓
产生 Partition B
```

验证：

```text
A 完全不变
B 数据完整
A/B path 不相同
A/B database 不相同
A/B uploads 不相同
```

---

# 129. V1 第一阶段实施顺序

建议按以下顺序开发。

## Phase 1：技术骨架与数据分区

* Go 项目；
* 配置；
* `system.db`；
* Data Partition；
* 默认 Partition；
* SQLite；
* System Migration；
* Partition Migration；
* PartitionManager；
* HTTP Router；
* Template；
* Static；
* Audit 基础；
* Session 基础。

数据分区应在业务开发早期确定。

不要先把整个系统写死成：

```text
data/fmlysys.db
```

后期再强行改造成多分区。

---

## Phase 2：后台

* Admin；
* Password；
* Google Authenticator；
* 成员管理；
* 权限；
* 数据分区基础管理页面。

---

## Phase 3：普通成员身份

* 微信认证抽象；
* 微信登录；
* Join Request；
* 审核；
* Session 与 Partition 绑定。

---

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

---

## Phase 5：附件

* 图片；
* 票据；
* PDF；
* 权限读取；
* Partition 独立 uploads。

---

## Phase 6：事务

* Matter；
* 子事务；
* 负责人；
* 参与人；
* 财务关联。

---

## Phase 7：遗产

* 遗产事项；
* 遗产清单；
* 分配；
* 划入公共财产。

---

## Phase 8：备份与数据分区导入

* SQLite 一致性 Snapshot；
* Backup Package；
* Manifest；
* SHA-256 校验；
* 本地备份；
* 外部备份上传；
* Safe Extract；
* Integrity Check；
* Import As New Partition；
* Partition Switch。

---

## Phase 9：Google Drive

* Google OAuth；
* Token 加密保存；
* Drive 文件上传；
* 可续传上传；
* 一键备份；
* 上传进度；
* Backup Records；
* 失败恢复和错误提示。

---

## Phase 10：长期能力

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
* 对公众开放注册；
* 两个数据分区自动合并；
* 外部 Backup 自动覆盖当前数据；
* 根据姓名自动识别跨 Partition 同一成员；
* 自动周期 Google Drive 备份；
* 在 Google Drive 上直接运行 FmlySys 数据库。

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

Google Drive API 只用于：

> 系统数据备份。

不得扩展为资金、账单或私人金融账户同步能力。

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

Google Drive 备份属于：

> 异地备份。

不应被宣传成不可篡改存证。

---

# 137. 数据长期可迁移原则

业务数据库使用：

> SQLite。

附件使用：

> 普通文件。

备份使用：

> 标准自描述 Backup Package。

配置使用：

> 通用文本格式。

必须避免把核心数据绑定到：

> 某个云厂商私有服务。

长期目标是：

```text
一个业务数据分区：

SQLite DB
+
uploads
```

可以被打包成为：

```text
Backup Package
```

然后：

```text
导入另一台 FmlySys
↓
创建新的 Data Partition
↓
继续使用
```

即使 Google Drive 不再使用：

> 本地备份、迁移和恢复能力仍然存在。

Google Drive 只是：

> 可选的异地备份目的地。

不是数据格式本身。

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
Data Partition
Backup
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

或者将：

```text
Import
Restore
Merge
Switch
```

混为一谈。

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

DataPartition
BackupPackage
BackupRecord
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

数据分区
备份文件
备份记录
```

备份相关操作统一使用：

```text
备份
导入
切换
```

三个不同概念。

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

### 15. 数据分区必须真正隔离

每个业务数据分区使用：

```text
独立 SQLite
+
独立 uploads
```

不得依靠在所有业务表中添加 `partition_id` 模拟隔离。

### 16. 外部备份导入永远新增分区

普通后台导入不存在：

> 覆盖当前系统数据

这一语义。

固定为：

```text
Backup Package
→
New Data Partition
```

### 17. 导入与切换必须分离

成功导入备份后：

> 当前活动数据分区不变。

只有管理员单独执行：

> 切换数据分区

后才进入新数据。

### 18. Google Drive 是备份目的地

Google Drive 故障不得影响：

> FmlySys 正常业务运行。

### 19. 云备份不得携带系统秘密

普通业务数据备份不得包含：

* 管理员密码；
* TOTP；
* Google OAuth Token；
* Master Key；
* Session。

### 20. 备份必须可移植、可验证

每个 Backup Package 必须：

* 自描述；
* 可执行 Hash 校验；
* 可执行数据库完整性检查；
* 不依赖原服务器绝对路径；
* 可以导入另一套兼容版本 FmlySys。

---

# 141. 第一版完成判定

当以下流程能够完整闭环时，可以认为 FmlySys 已具备第一版核心技术基础：

```text
管理员初始化
↓
Google Authenticator 登录 /admin
↓
建立默认 Data Partition
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
创建 SQLite 一致性 Snapshot
↓
完整打包当前 Partition 的 SQLite + uploads + manifest
↓
Backup Package 完整性校验通过
↓
管理员连接 Google Drive
↓
点击“一键备份到 Google Drive”
↓
云端备份成功并保存远端文件 ID
↓
将一个外部 Backup Package 上传到系统
↓
系统校验 Manifest / Hash / SQLite
↓
外部 Backup 被导入为新的 Partition B
↓
原 Partition A 数据完全没有变化
↓
Partition B 默认不自动激活
↓
管理员主动切换到 Partition B
↓
旧普通成员 Session 失效
↓
Partition B 的成员、财务、事务、附件均可独立使用
↓
管理员重新切换回 Partition A
↓
Partition A 内容与导入前完全一致
```

以上链路是第一阶段最重要的验收基线。

只有在这套基础稳定之后，再继续扩展：

* 家族长期档案；
* 复杂决议；
* 医疗备忘；
* 家谱；
* 更多提醒方式；
* Google Drive 直接选择备份导入；
* 自动周期云端备份；
* OCR；
* 其他高级功能。
