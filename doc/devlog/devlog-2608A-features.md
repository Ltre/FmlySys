# dev-2609A-step1 开发记录

日期：2026-09-03  
分支：`dev-2609A-step1`

> 按本轮约定，本次设计与开发记录保存于用户指定的 `doc/devlog/devlog-2608A-features.md`。文件名沿用用户给定路径，不擅自改成 2609A。

## 1. “查资料”页内浏览器、设备缓存与家庭推荐 Feed

### 1.1 目标拆解

本轮新增前台一级菜单“查资料”，位置固定在“通知中心”左侧。页面同时承担三类能力：

1. 类浏览器操作：内置地址栏、前进/后退/刷新和标签页，成员直接输入网址后由 FmlySys 所在服务器访问外部网站；
2. 快捷入口：地址栏下方展示 Google、Wikipedia、YouTube、Google 新闻、Google 翻译、PubMed 等推荐网址入口；
3. 家庭导向 Feed：再向下提供类似 Google Feed 的推荐卡片，根据当前成员实际拥有的权限，从正在进行的家族事务、服药计划和家庭共享资料中生成“接下来值得查什么”。

家庭数据只用于 FmlySys 服务端内部生成标题和搜索建议；在成员主动点击某项前，不会把家族事务正文、药品或共享资料主动提交给外部网站。

### 1.2 标签页与设备缓存

浏览器最多允许 5 个标签页。前端不是同时创建 5 个 iframe，而是只保留一个实际 iframe，切标签时重载对应 URL；这样“5 个标签”只是最多 5 份轻量状态，不会让五套完整网页 DOM 同时驻留内存。

每个成员的标签页状态使用 `localStorage`，key 中包含成员 ID，保存当前标签、URL、标题和最多 30 项轻量历史，因此同一设备上不同成员不会在正常 UI 中混用标签状态。

代理内容使用 `/research/` scope 下的 Service Worker 缓存在浏览器 Cache Storage：

- 只缓存 `GET /research/proxy` 的成功响应；
- 请求 URL 显式带当前 member ID，因此缓存项按成员和目标 URL 分离；
- 最多保留 60 个代理资源请求，超出后删除较旧项，避免设备缓存无限增长；
- 在线时优先重新请求服务器，网络失败时回退到设备缓存。

### 1.3 境外服务器代理与安全边界

页内浏览器以 FmlySys 部署服务器作为出口，因此若生产实例位于境外，外部网站请求就从该境外服务器发出。没有额外引入第三方代理服务或代理账户配置。

任意 URL 反向代理如果直接开放会形成 SSRF，并且第三方 JavaScript 如果在 FmlySys 登录域下执行，可能接触家族系统会话。因此本轮明确采用“阅读型页内浏览器”边界：

- 只接受 HTTP/HTTPS；
- 只允许 80/443 端口；
- 拒绝 localhost、`.local`、URL 用户名密码；
- IPv4/IPv6 的 loopback、private、link-local、multicast、unspecified 地址全部拒绝；
- 域名解析结果只要夹带任一非公网 IP 就拒绝，连接阶段再次解析与检查，降低 DNS rebinding 风险；
- 最多跟随 5 次重定向，每次重定向重新验证目标；
- 不向目标网站转发 FmlySys Cookie、Authorization 等用户凭据；
- 单资源最大 8 MiB，外部访问超时 12 秒；
- HTML 删除 `<script>`、meta refresh 和内联 `on*` 事件属性，并设置 `script-src 'none' / connect-src 'none' / frame-src 'none'` CSP；
- HTML 中 href/src/action 和 CSS `url(...)` 尽量重写回 `/research/proxy`，让后续阅读资源继续从服务器出口访问。

这意味着当前版本偏向“搜索、阅读资料、点链接继续浏览”，不是完整兼容所有重 JavaScript 网站的 Chrome 替代品。这个限制是为了不把任意第三方脚本放进带有 FmlySys 登录态的同源环境。

### 1.4 家庭推荐 Feed

Feed 不是把家庭数据库发给外部推荐算法，而是在服务端根据当前权限局部生成：

- `matters.view`：取计划中/进行中的事务，生成对应检索建议；
- `medication.view`：取当前服药计划药名，生成药品说明与注意事项检索建议；
- `share.view`：取近期 family 可见共享资料，生成继续查阅建议；
- 所有人额外获得家庭办事权益、家庭资产与财务管理等通用入口。

最终点击“查相关资料”后才在页内浏览器中打开 Google 搜索结果。

### 1.5 页面与路由

新增：

- `GET /research`：查资料主页面；
- `GET /research/proxy?member=<id>&url=<url>`：经过登录态、成员匹配和公网目标校验的阅读代理；
- `GET /research/sw.js`：只服务 `/research/` scope 的设备缓存 Service Worker；
- `web/templates/research.html`；
- `web/static/research.js`；
- `web/static/research.css`。

共享导航在“通知中心”左侧加入“查资料”，主程序通过 `WithResearch` 挂载新路由，不新增数据库 migration。

### 1.6 验证

提交前执行：

1. 新增/修改 Go 文件执行 `gofmt`；
2. `research.js` 使用 `node --check` 验证语法；
3. 单独验证所有代理正则可被 Go `regexp.MustCompile` 正常编译，并修正了 Go RE2 不支持反向引用的差异；
4. 代理安全测试覆盖 localhost、127.0.0.1、RFC1918、169.254.169.254、IPv6 loopback/ULA、非 80/443 端口、URL 用户名密码，以及公网 IPv4/IPv6；
5. HTML 改写测试覆盖 script/事件属性删除和链接回写代理；
6. 模板使用 `html/template` 加占位函数解析，确保 `research.html` 与共享 `nav` 定义可共同解析；
7. 当前执行容器没有完整拉取仓库依赖，因此不把未实际执行的全仓 `go test ./...` 虚报为通过；最终仍建议在实际部署开发机补一次全量测试和真实外网网站兼容性验收。
