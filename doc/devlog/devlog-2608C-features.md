# dev-2608C-step1 开发记录

日期：2026-08-26  
分支：`dev-2608C-step1`

> 历史第 1～22 节保存在 `devlog-2608C-features-01-22.md`；第 23～28 节保存在 `devlog-2608C-features-23-28.md`；第 29～31 节保存在 `devlog-2608C-features-29-31.md`；第 32～35 节保存在 `devlog-2608C-features-32-35.md`；第 36～37 节归档到 `devlog-2608C-features-36-37.md`。本文件继续记录当前开发。

## 38. 后台会话/审计、Passkey 凭据拆分绑定、通知中心与服药时区闭环

日期：2026-08-26

### 38.1 本轮逐项复核结论

本轮重新按用户给出的 13 项清单逐项对照，而不是把上一轮“已实现但尚未提交”的说明直接当成完成。复核发现上一轮最大的共同缺口是代码尚未真正落到 GitHub；另外还有几个必须补齐的实现边界：Passkey 单凭据改绑后登录必须按实际 credential 解析成员；手动服药提醒的 PWA 点击目标必须先进入通知详情；自动提醒失败不能因为失败投递记录而永久占住阶段。

本轮统一补齐并纳入一次提交。

### 38.2 管理员登录态

管理员会话仍然使用 `system.db.admin_sessions` 保存 token hash，因此服务进程重启不会天然丢失登录态。本轮不再使用旧的 12 小时新会话，而新增 30 天持久会话，并在有效管理员请求期间做滑动续期；浏览器 Cookie 同样刷新为 30 天。

过期 token 不允许通过续期逻辑复活；明确退出、管理员密码重置等原有失效机制继续生效。

### 38.3 访问记录与超级审计

访问记录只记录已认证成员的页面/API 请求。以下内容明确过滤：`/static/*`、healthz、favicon、Service Worker，以及 CSS/JS/map/图片/字体等扩展名资源。migration 000010 同时清理 000009 以来已经写入的静态资源访问噪声。

三类后台审计全部改为固定每页 200 条：访问记录、前台超级审计、后台超级审计。分页显示最多 10 个快捷页码，并提供“第一页”“最后一页”，同时显示当前页、总页数、总条数。

超级审计不再根据 POST/PUT/PATCH/DELETE、URL 关键字或表单内容猜测 create/update/delete。新的 `WithSuperAuditV2` 只在请求实际产生 `audit_logs` 业务事实时投影为 `super_audit_logs`。因此登录、TOTP 验证、logout 不会因为“它是一个 POST”就被标成“改”。原来 `audit_log_id IS NULL` 的 fallback 误报在 migration 000010 中清理。

后台“发送 Termux 测试通知”也不再主动伪造一条 `remote_notification_config update`；真正保存远控配置仍属于数据变更并保留业务审计。

### 38.4 系统/设备时区

服务端默认 `time.Local` 设置为 `Asia/Shanghai`，即 UTC+8。浏览器新增 `timezone.js`，使用 `Intl.DateTimeFormat().resolvedOptions().timeZone` 获取设备 IANA 时区并写入 `fmly_timezone` Cookie；共享导航加载该脚本，因此前后台业务页面都能采用设备时区显示 RFC3339 时间。

脚本还统一处理显式 `data-fmly-datetime` 元素和页面普通文本中的 RFC3339 时间，并给新增服药计划表单填入设备时区。IANA 时区无效时统一回退 `Asia/Shanghai`。

服药计划新增 `timezone` 字段，旧计划 migration 默认 `Asia/Shanghai`。计划服药时间解释为该计划时区中的本地墙钟时间，不再直接依赖服务器进程所在时区。

### 38.5 Passkey A/B 可以直接拆分到不同成员

现有“登录身份”继续保留手机号/恢复入口和身份默认成员，但 `passkey_login_credentials` 新增可选 `member_id` 覆盖关联。

后台 `/admin/passkeys` 新页面同时提供：

- 身份默认成员；
- 每一个具体 Passkey 的独立成员覆盖；
- 每个凭据当前实际登录成员的展示；
- 删除具体 Passkey 的原能力。

凭据选择“继承身份默认关联”时 `member_id=NULL`；选择具体成员时只改这一条 credential，不移动/删除同一身份下其他 Passkey。

最关键的登录路径也同步修改：WebAuthn `FinishLogin` 返回实际验证成功的 credential ID 后，系统按 `credential.member_id -> identity.member_id` 的优先级解析本次有效成员，并把该成员写入 Passkey 登录会话。这样 A 可以继续进入成员 1，而 B 可以直接进入成员 2，不需要删除 B、重新注册身份。

`/passkey/account` 也按本次 Passkey 会话的有效成员恢复普通成员 session，避免又被身份默认成员覆盖回去。

### 38.6 通知中心

新增成员通知中心 `/notifications` 与通知详情 `/notifications/{id}`。本轮严格只把两种服药通知写入中心：

1. 有权限人员手动点击“提醒服药”：先为服药人创建 `medication_manual` 通知；PWA 点击进入通知详情，详情展示提醒正文和精确签到链接。自动 scheduled/+1h/+2h 提醒不写通知中心。
2. 服药人在签到页点击“等下再说”：根据计划创建人与 `medication.manage_self/manage_others` 权限计算真正有权管理该计划的成员，排除当前服药人后，为每位管理者创建 `medication_later` 通知并尝试 PWA 推送。

通知创建、首次标记已读属于真实数据变化，继续写业务审计。

### 38.7 服药结束状态与操作锁定

`medication_plans` 新增 `ended_at/ended_by`。原 `end_date` 保留“自然有效期截止日期”的含义；点击“结束计划”现在写精确 `ended_at`，因此当日点击后立即进入“已手动结束”，不再因为 `end_date=今天` 的包含式日期判断回到“进行中”。详情页原来位于结束按钮左侧的日期控件已经删除。

服药主页、全部计划页、计划详情和签到页都识别“已手动结束/已过期/未开始/进行中”。已结束或已过期计划的服药登记、签到验证、手动提醒、结束按钮全部禁用，按钮和输入控件使用 `cursor:not-allowed` 并灰显；页面顶部显示醒目状态提示。计划历史信息仍可查看，删除与必要的计划资料维护不被误当成服药行为锁死。

签到页“我已服药”改为大号绿色按钮，“等下再说”改为大号红色按钮；在不可签到状态下两者统一灰显禁用。

### 38.8 日期/统计控件自动加载

计划详情的“查看/登记日期”删除“查看”按钮，`date` input 发生变化立即 GET 提交。

`/medication` 的“查看日期”和“统计周期”删除“应用”按钮，两者任何一个变化都立即提交当前筛选表单，同时保留当前成员 tab。

### 38.9 自动服药提醒时区与三个节点

旧循环按统一 `Asia/Shanghai` 计算全部计划且每分钟 ticker 的相位可能导致明显延后。本轮改为：

- 每条计划保存自己的 IANA `timezone`；
- 每次扫描分别把当前绝对时间转换到计划时区；
- 用“计划本地日期 + scheduled_time + 计划时区”构造真实计划时间点；
- 服务启动立即扫描，运行期间每 5 秒扫描一次，因此正常运行时在计划分钟、+1 小时、+2 小时节点最多只有数秒级调度延迟；
- scheduled、plus1h、plus2h 分别独立检查；只要最终服药仍未完成，就会进入对应节点；
- `taken/confirmed` 或“我已服药”的 pending 状态停止后续催促；`later/rejected/missed/未反馈` 仍允许后续节点；
- 只有 `status='sent'` 的自动投递才算该阶段完成。000009 原来的唯一索引会让失败记录也占住阶段，本轮改成只对成功投递建立唯一约束；全通道失败后允许至少间隔 1 分钟重试，避免 5 秒循环持续轰炸外部通道。

手动结束的计划不会进入提醒候选。

### 38.10 迁移与兼容

新增 migration `000010_session_audit_timezone_notification.sql`：

- Passkey credential/session 增加成员覆盖字段；
- medication plan 增加 timezone/ended_at/ended_by；
- 新建 member_notifications；
- 清理 fallback 超级审计和静态资源访问日志；
- 重建自动提醒投递唯一索引，只锁定成功投递。

旧 Passkey credential 的 `member_id` 为 NULL，因此继续继承原 identity.member_id；旧服药计划 timezone 自动为 `Asia/Shanghai`，无需人工补数据。

### 38.11 验证策略

本轮提交前执行以下针对性验证：

1. 所有新增/修改 Go 文件执行 `gofmt`，利用 Go parser 检查语法；
2. migration 000010 使用 Python sqlite3 在模拟 000009 结构执行，检查新增列、通知表、历史错误审计清理与新索引；
3. 新增 HTML templates 使用 Go `html/template` + 现有函数占位实际解析；
4. `timezone.js` 使用 `node --check`；
5. 对 Passkey credential 覆盖成员 SQL、审计分页 SQL、服药 stage/timezone 判定做独立 SQLite/Go 夹具检查；
6. 完整仓库依赖环境若仍无法联网解析 github.com，则不会把未执行的 `go test ./...` 描述为已通过；应在实际开发机继续执行全量测试。
