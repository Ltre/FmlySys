# dev-2608C-step1 开发记录

日期：2026-08-21  
分支：`dev-2608C-step1`

## 1. 本轮目标

根据 `doc/requestment.md`、`doc/tech/tech-master.md` 和初期开发计划，先建立可持续扩展的 Go + SQLite 单体框架，并优先打通三条当前最核心业务线：

1. 公共资产消费及管理；
2. 家族事务管理；
3. 家族信息共享。

本轮不以“把所有规划功能一次做完”为目标。微信正式登录、`/admin` 的密码 + Google Authenticator TOTP、成员审核、备份导入和 Google Drive 等继续按后续步骤接入，但核心业务层不得依赖这些尚未接入的认证实现。

## 2. 关键技术决策及原因

### 2.1 业务身份统一使用内部 member_id

Step1 先自动建立一个开发态成员，用于验证核心功能。公共资产、事务、档案、审计全部只引用内部成员 ID，不直接引用微信 OpenID。

原因：微信开放平台配置不应阻塞业务开发；以后接入微信认证时只需要把微信身份映射到 `member_id`，不需要重写账务和事务表。

### 2.2 从第一天引入 system.db + 默认业务数据分区

目录按技术文档落为：

```text
data/
├── system.db
└── partitions/
    └── p_default/
        ├── fmlysys.db
        └── uploads/
```

`system.db` 当前负责数据分区注册；家族业务数据全部进入 `p_default`。

原因：如果先把全部业务写死到单个 `data/fmlysys.db`，后续实现“外部备份导入为新数据分区”时会迫使整个 Repository/附件路径重构。

### 2.3 余额不落死值

没有建立 `current_balance`、`before_balance`、`after_balance` 作为权威字段。

当前公共财产净额由有效资产事件减有效消费计算；持有人虚拟账户由资产来源、内部转入/转出、直接公共消费和报销动态计算；待报销由垫付消费的应报金额减有效报销计算。

这保持了需求中“存业务事实，不存死余额”的原则。

### 2.4 公共资产内部转账不是消费

`holder_transfers` 独立存储 A → B 的公共资产代管转移。它只改变双方虚拟账户，不改变公共财产净额。

写入时检查转出人的当前代管余额，禁止因内部转账产生负余额。

### 2.5 个人垫付在消费发生时就影响公共财产净额

个人垫付消费会立即减少公共财产净额，并产生待报销；之后的报销只减少某持有人的代管余额和待报销，不再次减少公共财产净额。

报销支持分次发生，并检查：

- 本次报销不能超过剩余待报销；
- 报销付款人的虚拟账户不能被透支。

### 2.6 消费允许修改，但核心账务约束必须重新校验

Step1 已加入公共消费编辑入口。当前可修正标题、分类、金额、发生时间、支付渠道、收款方、用途和关联事务。

修改个人垫付金额时，如果新金额小于已经完成的报销总额则拒绝；修改直接公共消费金额时，如果会导致原公共资产持有人虚拟账户透支则拒绝。

修改前后值写入 `audit_logs`，不采用“金额错了必须作废旧单再建新单”的模式。

### 2.7 家族事务采用通用 Matter

没有分别创建“百日表”“祖屋维修表”等专用系统。`matters` 支持父子层级、类型、负责人、开始/截止日期和状态。

公共消费通过 `matter_id` 关联事务；事务页的实际支出由关联消费实时 SUM，不另存一份 `actual_cost`。

### 2.8 信息共享先做通用 Archive + Attachment

初期支持资料标题、分类、正文、可见范围字段和附件。附件存储在当前数据分区 `uploads/` 中，数据库只保存元数据和 SHA-256。

文件下载走 `/files/{id}`，没有把 uploads 目录直接暴露为静态目录。

当前 Step1 尚未接入正式成员认证，因此 `family/admin` 可见范围只是数据模型和界面入口，真正的 ACL 校验将在成员认证步骤一起补齐；在此之前该分支属于开发态，不应直接作为生产部署。

## 3. 已实现框架

- `cmd/fmlysys`：程序入口与 graceful shutdown；
- `internal/config`：环境配置；
- `internal/db`：SQLite 打开及 Migration；
- `internal/partition`：`system.db`、默认数据分区、活动分区；
- `migrations/system`：系统级 Schema；
- `migrations/partition`：业务 Schema；
- `internal/store`：成员、账务、事务、共享资料 Repository/业务约束；
- `internal/httpserver`：Web 路由和表单处理；
- `web/templates`：首页、公共资产、家族事务、信息共享、消费编辑、后台占位；
- `web/static`：基础响应式样式。

## 4. 公共资产管理完成情况

已完成：

- 初始公共资产；
- 公共资产新增；
- 公共资产划出；
- 账务调整基础事件；
- 多公共资产持有人虚拟账户；
- 持有人当前代管金额动态计算；
- 公共消费；
- 直接使用代管公共资产消费；
- 个人垫付；
- 待报销金额；
- 分次报销；
- 持有人之间公共资产内部转账；
- 资产来源/调整流水展示；
- 消费流水展示；
- 内部转账流水展示；
- 报销流水展示；
- 消费核心字段修改及 Audit；
- 账务一致性差额展示。

当前一致性公式：

```text
所有持有人虚拟账户合计
-
待报销总额
=
公共财产净额
```

首页和公共资产页会显示差额，系统不会自动创建 Adjustment 将错误“修平”。

## 5. 家族事务管理完成情况

已完成：

- 创建通用事务；
- 父子事务；
- 事务类型；
- 描述；
- 负责人；
- 开始日期；
- 截止日期；
- planned / active / done / cancelled 状态；
- 状态修改 Audit；
- 公共消费关联事务；
- 每个事务关联公共支出动态统计。

当前已经能够直接用“父亲身后事务 → 百日/对年/三年/入祠”这种结构录入，而无需为各类习俗增加专用表。

## 6. 信息共享完成情况

已完成：

- 新建共享资料；
- 分类；
- 正文；
- family/admin 可见范围字段；
- 附件上传；
- 50MB 单文件开发期限制；
- SHA-256；
- 随机/不可预测磁盘文件名；
- 受 Handler 控制的附件下载；
- 共享资料列表。

当前没有建设完整家谱、知识图谱或医疗系统，符合初期计划边界。

## 7. 页面与路由

```text
/                         首页聚合
/assets                   公共资产管理
/assets/expenses/{id}/edit 消费编辑
/matters                  家族事务管理
/share                    家族信息共享
/files/{id}               附件下载
/admin                    后台框架占位
/healthz                  健康检查
```

## 8. 已知边界 / 后续步骤

本轮有意未完成：

- 微信正式登录、申请加入、成员审核；
- `/admin` 管理员密码 + Google Authenticator TOTP；
- CSRF；
- 正式 RBAC / Archive ACL；
- 公共资产事件、内部转账、报销的修改/撤销 UI；
- 消费逐条“消费前余额 / 消费后余额”历史展示；
- 更完整的 Audit 浏览页面；
- 遗产模块；
- 提醒；
- 备份包；
- Google Drive；
- 外部备份导入新 Partition；
- Partition 切换 UI。

这些不影响当前三条核心业务的数据模型继续演进，但在正式对家族成员开放前，认证、CSRF、权限和备份必须补齐。

## 9. 验证记录

当前执行环境无法通过网络解析 GitHub/Go Module 站点，因此不能实际下载 `modernc.org/sqlite` 并进行真实 SQLite 启动测试。

为避免完全跳过编译检查，本地为 `modernc.org/sqlite` 临时建立了只用于编译的占位模块，并执行：

```text
go test ./...
```

结果：通过。

验证内容包括：

- 全部 Go package 可编译；
- 金额元/分转换测试通过；
- 所有 HTML Template 可解析；
- 路由、Store、Migration 代码通过静态编译。

发布到仓库的 `go.mod` 不包含这个临时 replace，仍使用真实依赖：

```text
modernc.org/sqlite v1.34.5
```

因此下一次在具备 Go Module 网络环境的机器上应首先执行：

```bash
go mod download
go test ./...
go run ./cmd/fmlysys
```

并完成真实 SQLite CRUD 冒烟测试。

## 10. Windows 本地代理开发启动脚本

为 Windows 本地开发增加 `scripts/dev-windows.cmd`，用于在启动 FmlySys 前设置当前进程临时代理：

```text
HTTP_PROXY=http://127.0.0.1:58591
HTTPS_PROXY=http://127.0.0.1:58591
ALL_PROXY=socks5://127.0.0.1:51837
```

同时设置小写代理变量以兼容只读取小写环境变量的命令行工具，并设置：

```text
NO_PROXY=127.0.0.1,localhost
```

以保证本机开发请求不经过代理。

脚本使用 Windows `setlocal`，因此这些变量只影响该脚本以及由它启动的 Go 子进程，不写入系统或用户永久环境变量。脚本退出后原环境自动恢复。

考虑到 Step1 尚未完成正式登录认证，开发脚本额外固定：

```text
FMLYSYS_ADDR=127.0.0.1:8080
```

避免开发态服务误监听全部网卡。脚本还会自动切换到仓库根目录、设置当前项目的 `data` 目录，并在启动前检查 `go` 是否已经加入 `PATH`。

## 11. 修复 Windows 首次启动缺失 go.sum

### 11.1 现象

Windows 本地执行 `scripts\dev-windows.cmd` 时，`go run ./cmd/fmlysys` 报错：

```text
missing go.sum entry for module providing package modernc.org/sqlite
```

仓库当前已经声明 `modernc.org/sqlite v1.34.5`，但尚未在具备真实 Go Module 网络环境的机器上生成 `go.sum`。此前的编译验证使用了临时占位依赖，因此远端仓库并没有真实依赖校验文件。

### 11.2 处理方式

Windows 开发启动脚本现在在 `go run` 之前自动执行：

```text
go mod download all
go mod verify
```

原因：

- `go mod download all` 会在当前临时代理环境下下载主模块构建列表中的真实依赖，并补齐本机需要的 `go.sum` 校验项；
- `go mod verify` 在启动应用前验证已下载模块内容与校验值一致；
- 任一步失败都会停止启动，避免把“依赖没准备好”误判成 FmlySys 自身运行错误；
- 不使用每次启动都执行 `go mod tidy` 的方式，避免开发启动脚本无必要地重写 `go.mod`。

首次成功运行后，本地工作区会生成 `go.sum`。该文件属于正常的 Go Module 依赖锁定/校验文件，后续应在有真实网络依赖解析结果后纳入仓库，以使其他机器不再依赖首次运行时生成。

当前远端执行环境仍不能访问 Go Module 网络，因此没有伪造或手工填写任何 checksum；真实 `go.sum` 由用户本地 Go 工具链通过代理生成。

## 12. Windows 启动脚本工作目录无关化

为保证开发启动脚本既可以从仓库根目录执行 `scripts\dev-windows.cmd`，也可以进入 `scripts` 目录后直接执行或双击运行，脚本不再依赖调用者当前工作目录。

脚本现在通过 `%~dp0` 获取 `dev-windows.cmd` 自身所在目录，再规范化得到仓库根目录 `REPO_ROOT`，并在切换目录前检查 `REPO_ROOT\go.mod` 是否存在。数据目录也直接绑定为 `REPO_ROOT\data`。

因此以下启动方式使用同一套路径解析逻辑：

```text
# 位于仓库根目录
scripts\dev-windows.cmd

# 位于 scripts 目录
cd scripts
dev-windows.cmd

# Windows 资源管理器
双击 scripts\dev-windows.cmd
```

该调整避免脚本因 CMD / PowerShell 当前目录或资源管理器双击启动时的工作目录不同而错误定位 `go.mod`、`data` 或 `cmd/fmlysys`。

## 13. 修复 go.mod 需 tidy 与双击失败闪退

### 13.1 现象与根因

真实 Windows Go 环境首次完成模块下载和校验后，`go run ./cmd/fmlysys` 仍可能提示：

```text
go: updates to go.mod needed; to update it:
        go mod tidy
```

`go mod download all` 只负责下载模块，`go mod verify` 只负责校验已下载模块；二者都不会替代 `go mod tidy` 对主模块依赖声明和校验信息进行整理。因此此前脚本的依赖准备流程并不完整。

同时，脚本此前在任何失败路径上直接 `exit /b 1`。从资源管理器双击 `.cmd` 时，CMD 窗口会随脚本结束而关闭，导致错误信息闪过后无法阅读。

### 13.2 修复

Windows 开发启动脚本现在统一执行：

```text
go mod tidy
go mod download all
go mod verify
go run ./cmd/fmlysys
```

其中 `go mod tidy` 在代理环境已经设置之后执行，因此需要补充依赖时仍使用本地代理。整理完成后再下载完整模块并校验，最后才启动应用。

所有失败路径也统一进入 `:fail`：包括仓库根目录解析失败、无法进入仓库目录、找不到 Go、`go mod tidy` 失败、模块下载失败、模块校验失败以及 `go run` 非零退出。失败时会保留明确错误信息并执行 `pause`，因此即使从资源管理器双击脚本，也不会再因错误直接闪退。

该 `pause` 仅在失败路径执行；正常启动和正常退出不会额外要求按键。