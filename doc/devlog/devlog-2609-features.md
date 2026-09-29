# 2026-09 财务时间线开发记录

日期：2026-09-30  
分支：`dev-2609A-step1`

## 需求与实现方案

在公共资产页 `#timeline-card` 放置扁平入口卡片，点击后覆盖当前视口打开财务时间线；首页公共资产快捷操作区增加同名按钮，导航至 `/assets` 并自动打开浮层。时间线合并公共消费、成员转账、报销登记、资产变动四类有效记录，默认每页 20 条，按发生时刻倒序显示。

浮层由 `/assets` 页面服务端渲染。分页状态使用 `timeline=1&page=N` 表示，因此入口、分页和直接打开链接最终都走现有 `/assets` 路由。该路由由 `member("assets.view", ...)` 保护；首页快捷操作和资产页入口也只在 `assets.view` 权限存在时渲染。查询参数只控制浮层与页码，不授予权限，也没有新增可绕过成员权限的数据接口。

时间线在 Store 层用一条合并查询读取四张业务表的 `active` 记录，使用 `julianday(occurred_at)` 做跨来源时间比较，并以原始时间、类别和记录 ID 为同秒排序的稳定次序。服务端按总记录数计算页数，并把越界页码收敛到最后一页。页面沿用站点时区脚本格式化 ISO 时间戳，显示年月日、小时、分钟和秒。

记录呈现采用纵向动态卡片：类别 TAG、类别图标、参与成员、事项说明、时间和金额分层展示，并用颜色区分消费支出、报销支出、内部转账和资产增减。消费、报销及资产变动按账务方向显示正负号；成员间转账显示转出与转入双方。报销登记展示 `reimbursements` 表中的正式登记；消费发生时由既有业务逻辑自动使用代管资产的金额已经属于消费记录，不重复生成一条报销登记。

## 修改内容

- `internal/store/models.go`：增加时间线记录和分页数据结构。
- `internal/store/financial_timeline.go`：增加四类有效账务记录的统一查询、计数和分页。
- `internal/httpserver/server.go`：在资产页读取分页数据，并依据 `timeline=1` 决定初始浮层状态；时间线仍由既有 `assets.view` 路由权限拦截。
- `web/templates/assets.html`：完成 `#timeline-card` 入口和全屏对话浮层，包含空状态、秒级时间、金额方向及上一页/下一页导航。
- `web/templates/dashboard.html`：在首页 `.quick-actions` 增加“财务时间线”按钮，链接带自动打开参数。
- `web/static/financial-timeline.css`：实现扁平入口、全屏浮层、时间轴卡片、分页和窄屏布局。
- `web/static/financial-timeline.js`：实现打开/关闭、URL 状态同步、浏览器返回、Esc 关闭和对话框焦点管理。

## 检查与工作区状态

对 Go 文件执行了 `gofmt`。按本次任务要求，没有运行测试，没有执行 `git add`，也没有创建 Git commit。修改前工作区已有 `go.mod` 和 `go.sum` 未提交改动；本轮保留这两项改动，没有把它们纳入功能实现。

## 建议 Git 文案

**Title**

```text
feat: add permission-gated financial timeline
```

**Description**

```text
Add a full-screen timeline on /assets that merges public expenses, member transfers, reimbursements, and asset changes in occurrence-time descending order, with second-level localized timestamps and 20-entry pagination.

Expose the timeline from the assets card and dashboard quick actions, including direct links that open the overlay on arrival. Gate both entry points and timeline data through the existing assets.view permission and present records as tagged, color-coded activity cards.
```
