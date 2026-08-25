# dev-2608C-step1 开发记录

日期：2026-08-26  
分支：`dev-2608C-step1`

> 历史第 1～22 节保存在 `devlog-2608C-features-01-22.md`；第 23～28 节保存在 `devlog-2608C-features-23-28.md`；第 29～31 节保存在 `devlog-2608C-features-29-31.md`；第 32～35 节归档到 `devlog-2608C-features-32-35.md`。本文件继续记录当前开发。

## 36. 全局审计与服药计划/通知闭环

日期：2026-08-26

### 36.1 审计设计

原有事务管理和信息共享已经在 Store transaction 中使用 `auditTx` 保存创建、修改、状态变更以及共享附件增删的 `audit_logs`，本轮保留它作为业务变更事实源，而不是另造一套与业务审计互相冲突的数据。

新增 `member_access_logs`，记录已认证前台成员的 IP、成员 ID/姓名快照、请求方法、路径和访问时间；静态资源、健康检查和 Service Worker 不计入访问流水。

新增 `super_audit_logs`，统一保存：前台/后台来源、IP、成员或管理员、数据分类、原始 action、归一化的增/删/改、对象 ID、变动前 JSON、变动后 JSON、HTTP 方法/路径和操作时间。HTTP 写请求完成后，将这次请求实际产生的 `audit_logs` 投影到超级审计；没有业务审计但成功完成的写请求补 fallback 记录。password、secret、token、auth、p256dh、TOTP 等敏感字段在 fallback 中脱敏。

为避免两个并发写请求从全局 `audit_logs` 中错误认领彼此的记录，本轮在“取当前最大 audit ID → 执行业务写 → 投影本次新增 audit”窗口串行化。FmlySys 是小规模家族应用，这比重写所有既有 Store 方法传 request-id 的风险更低。

后台新增 `/admin/audit`，横向 tabs 分为访问记录、前台超级审计、后台超级审计。

### 36.2 服药权限、计划列表与多成员页面

旧 `medication.manage` 在 migration 000009 中自动迁移为：

- `medication.manage_self`：管理自己创建的服药计划；
- `medication.manage_others`：管理他人创建的服药计划；
- `medication.view`：查看服药管理。

权限判断按**计划创建人**而不是服药人。后台创建成员、保存权限和批准加入时，只要选择任一管理权限就自动补 `medication.view`。

`/medication` 统一改名为“服药管理”，去掉“老大”。页面横向 tabs 列出所有存在未标记删除服药计划的成员，即使该成员只剩已经结束的历史计划仍会显示。创建计划后自动切换到被指定服药人的 tab。

新增 `/medication/plans` 扁平列表，状态规则：当前日期早于开始日期=未开始；已达到开始日期且尚未超过结束日期（或无结束日期）=进行中；超过结束日期=已结束。计划支持可选结束日期，留空表示长期。

计划详情支持完整修改、保存服药情况、结束计划、软删除、手动推送提醒和待验证操作。删除只设置 `is_deleted`，不物理删除计划、历史服药记录或审计记录。

统计周期扩展为 7/14/30/90/180/365 天，并按当前服药人计算计划数、已确认服用、未服、未登记和服用率。

### 36.3 服药签到、验证与再次通知

通知点击进入 `/medication/checkin?plan=<id>&date=<date>`。服务端强制当前成员必须等于计划的 `patient_member_id`。签到只有“我已服药”和“等下再说”。

“我已服药”写 `medication_checkins` 为 pending。此时自动催促立即暂停，即使管理者还没点击确认，也不会因为管理者操作不及时继续打扰服药人。

有计划管理权限者可以选择：

- “确实已经服药”：checkin=confirmed，并原子写入/更新最终 `medication_intake_records.status=taken`；
- “并没有服药”：checkin=rejected，并写 `status=missed`，后续 +1h/+2h 节点仍可提醒。

自动提醒循环每分钟扫描一次，按计划时间、+1 小时、+2 小时三个阶段去重投递。没有反馈、选择“等下再说”、被判定“并没有服药”均属于未完成服药确认；最终 taken、confirmed 或 pending 的“我已服药”停止后续催促。

### 36.4 PWA Push 与 Termux/FRP/SSH

PWA 为首选通道。系统随机生成并持久化 P-256 VAPID 私钥；浏览器保存 PushSubscription。服务端实现 Web Push `aes128gcm` payload 与 VAPID ES256，通知点击直接进入签到页。页面处于打开状态时，Service Worker 通过 `postMessage` 触发 Web Speech API 中文语音；PWA 完全后台/关闭时浏览器不允许可靠运行页面 TTS，因此只保证系统通知及系统通知声，不虚构后台 TTS 能力。

当 PWA 没有可用订阅或所有 endpoint 均投递失败时，使用现有 FRP STCP + SSH 兜底，在手机执行 `termux-notification` 与 `termux-tts-speak`。

后台 `/admin/remote-notifications` 配置本地 STCP SSH Host、端口、SSH 用户名和密码。Host 限制为 localhost/127.0.0.1/::1，防止将配置功能变成任意 SSH 跳板。

远控配置采用信封加密：随机 AES-256-GCM 内容密钥加密 JSON；内容密钥再使用系统随机 RSA-3072 公钥 RSA-OAEP(SHA-256) 包装。RSA 私钥持久化在 data 并尽量设置 0600 权限，保证服务重启后无需人工干预即可解密。没有选择“仅把私钥放内存”，因为无外部人工密钥时会造成重启后无法自举。

### 36.5 验证

当前执行容器无法解析 `github.com`，所以无法 clone 完整仓库并真实运行全量 `go test ./...`。本轮没有把这类未执行测试描述成通过，而是实际完成以下检查：

1. migration 000009 使用 Python sqlite3 在模拟 000008 结构上执行通过，确认旧 medication.manage 迁移、计划软删除字段和新日志/签到/推送表均建立；
2. 新增 Store 文件在按当前 Store/Member/audit 接口构造的编译夹具中 `go test` 通过；
3. 新增 HTTP handler、审计 console 在按当前 Server/Store 方法签名构造的编译夹具中 `go test` 通过；
4. RSA + AES-GCM 远控配置做了真实加密保存/重新加载测试，非 loopback Host 拒绝测试通过；
5. Web Push VAPID 持久化与 aes128gcm record 构造做了 smoke test；
6. 新增 Go template 使用 `html/template` 解析通过；`medication-push.js`、`medication-sw.js` 使用 `node --check` 通过。

仓库同时加入相应回归测试。由于完整依赖环境限制，首次在实际开发/部署机器拉取本提交后仍应执行 `go test ./... -count=1` 作为最终完整编译级验证。

## 37. 修复服药页公共导航模板崩溃并恢复源码规范排版

日期：2026-08-26

### 37.1 `/medication` 运行时模板错误

实际浏览器访问 `/medication` 时出现：

```text
template: dashboard.html:3:124: executing "nav" at <.AdminUsername>:
can't evaluate field AdminUsername in type httpserver.medicationPageView
```

根因不是服药数据查询，而是共享 `nav` 模板需要读取 `.AdminUsername` 来区分前后台导航。原来的通用 `view` 包含该字段，但第 36 节新增的四种服药页面专用 view 没有声明它。Go `html/template` 对不存在的结构体字段会直接返回执行错误，并不会把它当成空字符串，所以页面在进入前台导航分支之前就已经 500。

本轮统一给以下四种 view 补上 `AdminUsername string`，前台保持零值即可：

- `medicationPageView`；
- `medicationPlansView`；
- `medicationPlanDetailView`；
- `medicationCheckinView`。

因此 `/medication`、`/medication/plans`、计划详情和服药签到页都具备共享导航模板要求的同一字段契约，而不是只针对当前报错页面做一次性补丁。

同时新增 `TestMedicationViewsRenderSharedNavigation`：直接解析仓库真实的 `dashboard.html`，并分别使用上述四种 view 执行真实 `nav` 模板。这个测试专门防止以后新增/调整服药 view 时再次漏掉共享导航所需字段。

### 37.2 恢复 Go 源码可维护格式

第 36 节通过 Git 对象接口写入代码时，为规避单次传输长度限制，一批新增文件被临时压成了大量分号和单行函数。虽然 Go 语法允许，但严重降低可读性，不应该作为仓库最终源码格式。

本轮对上一提交中实际被压缩的 Go 文件重新使用真实 `gofmt` 规范化，并按职责增加必要空行和分段，包括：

- `cmd/fmlysys/main.go`：配置加载、分区初始化、成员/管理员初始化、提醒循环、中间件链、HTTP Server、信号退出分别成段；
- `internal/httpserver` 下新增的审计 console、超级审计 middleware、服药 view/routes/actions/push handlers；
- `internal/store` 下新增的超级审计、服药计划 CRUD/查询、签到验证、通知投递；
- 上一轮中同样存在密集写法的审计/服药回归测试。

已经正常排版的大文件（例如远控信封加密和 Web Push 核心实现）不做纯噪声重排，避免无意义扩大 diff。

### 37.3 HTML、CSS 与 JavaScript 排版

`dashboard.html` 和 `medication-enhancements.html` 原先几乎一个 template 占一整行。本轮在不改变 Go template 控制逻辑的前提下，按 HTML 层级进行结构化缩进：`html/head/body/main/section/form/table` 等元素分层，`if/range/else/end` 与其控制的 DOM 对齐。

同时把：

- `medication-v2.css` 从单行压缩样式恢复为 selector/property 分行；
- `medication-push.js` 恢复函数、异常分支、Fetch 参数和事件回调的正常缩进；
- `medication-sw.js` 恢复 Push/notificationclick 事件、notification options 和 client 遍历的正常缩进。

这些调整只恢复源代码可维护性，不引入前端构建期 minify，也不改变浏览器最终行为。

### 37.4 验证

本轮实际执行：

1. 所有本轮整理的 Go 文件执行 `gofmt`，同时利用 `gofmt` 的解析过程确认没有因展开代码引入 Go 语法错误；
2. `dashboard.html` 与 `medication-enhancements.html` 使用 Go `html/template` 和现有模板函数集合实际解析通过；
3. 使用前台 view 字段集合实际执行共享 `nav` 模板，确认 `.AdminUsername` 不再产生字段缺失错误；
4. `medication-push.js`、`medication-sw.js` 分别执行 `node --check` 通过；
5. 新增真实 `nav` 回归测试，覆盖服药首页、扁平计划列表、计划详情、签到四种 view。

当前执行容器仍无法直接 clone GitHub 完整仓库，因此不会把未实际运行的 `go test ./...` 描述成通过。提交后应由本地完整仓库继续执行 `go test ./... -count=1`；本轮已经把导致实际页面 500 的模板字段契约纳入自动回归测试。
