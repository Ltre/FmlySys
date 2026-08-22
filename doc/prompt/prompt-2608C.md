问：
	现在可以开始开发了。
	先搭建好框架，然后先把最核心的公共资产管理（含有消费、内部转账等）、家族事务管理、信息共享实现
	
	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github
	
	
答：
	
	
	我会直接在 dev-2608C-step1 上做第一轮可运行实现，不开 PR、不碰 main。这轮目标按你定的顺序：先把 Go/SQLite/模板/路由/数据分区骨架搭稳，再实现公共资产管理（消费、持有人、内部转账、垫付/报销基础闭环）、家族事务管理、信息共享，并把开发过程和关键取舍写进 doc/devlog/devlog-2608C-features.md，最后提交并推送该分支。

	分支当前还是文档骨架，没有既有业务代码可延续，所以这一步实际上是从零落第一套可运行代码。一个实现取舍我已经确定：Step1 不会为了等微信开放平台配置而卡住核心业务开发——认证层先抽象并提供开发态成员身份，公共资产/事务/共享全部只依赖内部 member_id；微信登录和正式 /admin TOTP 接入可以在后续步骤替换认证适配器，而不会重写业务表。

	核心数据模型已经落到代码里了：公共资产不是“余额字段”，而是由资产事件、直接消费、垫付、内部转账和报销实时推导；同时在写入层加了两条关键保护——内部转账不能透支转出人的虚拟账户，报销不能超过剩余待报销额，也不能透支报销付款人的代管金额。这样 Step1 就不会先做出一个“能录数据但账随时能坏”的壳。

	第一轮静态编译已经过了（用本地 SQLite 驱动占位只验证 Go 代码和模板依赖关系；当前执行环境不能联网拉取 modernc.org/sqlite，所以真实 SQLite 集成运行会在提交记录里明确标注为待联网环境验证）。我又检查了一遍功能闭环，发现“能登记内部转账但看不到转账流水”和“消费金额录错后缺少编辑入口”都不够，这两项会在提交前补上，而不是留一个只有表单的半实现。

	现在三条主线都已经形成可操作页面，而不是只有数据库层：/assets 有资产来源、消费、内部转账、报销和流水；/matters 有父子事务、负责人、日期、状态和关联支出；/share 有分类资料、正文和附件上传/下载。消费金额等核心字段也补了编辑入口，修改会写审计，并对“修改后导致持有人透支 / 低于已报销金额”做拒绝。下一步就是整理 devlog、做最终 diff/编译检查，然后一次性提交到你指定的分支。

	代码和 devlog 已整理完，第二轮 go test ./... 也通过了。现在我在做提交前最后一遍范围检查：只会写入本轮框架和三条核心业务相关文件，以及 .gitignore、README.md、指定 devlog；不会改需求文档、技术文档和 roadmap。

	GitHub 的 Git Data create_tree 写入接口被当前安装权限拒绝了（403），但仓库本身有 push 权限；我会改走 GitHub Contents 写入接口逐文件提交到同一个 dev-2608C-step1 分支。这样不会碰 main，只是提交会由单个大 commit 变成一串连续小 commit。

	Contents 写接口也同样被 GitHub App 拒绝（403）。这不是分支保护问题，而是当前连接器对该仓库实际没有“Contents/Git Data 写”权限；读取和仓库元数据权限正常，但提交代码的 API 被 GitHub 拒绝。

	我不会因此丢掉这轮实现：代码、迁移、页面、测试和 devlog 已在本地工作区完成并通过编译级检查。我会再做一份完整变更包/补丁作为兜底，同时确认是否还有可用的 GitHub 写通道；如果仍被拒绝，我会明确给出被阻塞的唯一点。
	
	第一轮开发代码已经完成，但**最后的 GitHub 提交/推送被当前 GitHub 连接权限阻止了**。

	已经完成的内容包括：

	- Go + SQLite 单体框架、Migration、`system.db + p_default` 数据分区；
	- **公共资产管理**：初始/新增/划出、持有人虚拟账户、公共消费、个人垫付、分次报销、内部转账、动态余额、一致性检查、消费修改审计；
	- **家族事务管理**：父子事务、负责人、状态、日期、关联公共消费及动态支出统计；
	- **信息共享**：资料分类、正文、可见范围基础字段、附件上传/下载、SHA-256、分区独立 `uploads`；
	- `/`、`/assets`、`/matters`、`/share`、`/admin` 框架页面；
	- 开发记录已写入 `doc/devlog/devlog-2608C-features.md`；
	- `go test ./...` 和 `go vet ./...` 已通过编译级检查。由于当前执行环境无法联网下载 `modernc.org/sqlite`，测试时使用了临时 SQLite 驱动占位，仅用于检查 Go 编译和模板；这个限制也已经写进 devlog。

	
	
问：

	我在本地测试需要使用代理：export HTTP_PROXY=http://127.0.0.1:58591; export HTTPS_PROXY=http://127.0.0.1:58591; export ALL_PROXY=socks5://127.0.0.1:51837

	不过这个命令好像不适合windows，你写个windows本地环境的启动脚本，要自动设置代理所用的临时环境变量

	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github
	
	
问：

	我看了 http://localhost:8080/assets 页面，成员管理应该放在后台。该页面中的全部功能，在后台应该也要有，而且可操作的应该要更多。
	1、登记公共资产区域，应该按如下优化：
		1.1、不要以下拉框形式选择[初始资产、资产新增、资产划出、财务调整]，而是要以扁平视图选择资产变动方式。
		1.2、对于同一个成员，只有一次"初始资产"变动记录，以后新资产流入成员手中，都记为"资产新增"。
		1.3、至于"资产划出"，我还不明确有什么用。我的理解是：如果要划出资产，一般也是要将款项归还至当初"资产新增"时所属的来源者，感觉好像是这样理解的。说通俗点，可能类似于撤资。
		1.4、对于"财务调整"，是不是属于财务校对时的修正？
		1.5、应该把完整版的、包含所有资产变动方式的"登记公共资产"功能，迁移到后台。前台可以做得更简单：持有人就不用选了，成员登录进来就只能操作自己的公共资产数据；自己可以增加持有多少资金（对应"资产新增"），也可以减少持有多少资金（对应"资产划出"）；日期时间可选，留空则采用当前时间。
	2、在"新增公共消费"区域，应该作如下修改：
		2.1、经手人就是付款人，把"付款人"删掉。
		2.2、去掉funding_type字段，不需要界定是"直接使用代管公共资产"还是"个人垫付"
		2.3、去掉"直接支出时选择持有人"字段。不要搞得太复杂，前面已经有经手人了，经手人就是付款人，也是直接支出人。
		2.4、支付渠道要明确提供以下选择：支付宝、微信、银行、现金、其它，并提供多个文件（图片、文档pdf|txt|word|excel|ppt ）上传单据、照片、截图、文档等内容，每个文件最大10MB，上传区域的描述定为"支付凭证"。
	3、公共资产内部转账区域，这整个功能可以整体迁移到后台。前台可以这样简单调整：
		3.1、删除转出人、转入人这两个select控件，而是只选择转账对象是谁
		3.2、转账渠道要明确提供以下选择：支付宝、微信、银行、现金、其它。
		3.3、增加转账凭证文件上传，支持多个文件选择，类型以（图片、文档pdf|txt|word|excel|ppt ）上传单据、照片、截图、文档等内容，每个文件最大10MB。
		3.4、要求登录的成员选择转账方向："接收自"、"转给"。
	4、在"编辑公共消费"页面 http://localhost:8080/assets/expenses/1/edit ，作如下修改：
		4.1、应该公开显示修改消费流水的审计记录。
		4.2、提供报销状态以及对应的报销流水，如果存在尚未报销完成的金额，则应该显示"报销"按钮。
	5、登记报销区域，作如下调整：
		5.1、转账渠道要明确提供以下选择：支付宝、微信、银行、现金、其它。
		5.2、增加转账凭证文件上传，支持多个文件选择，类型以（图片、文档pdf|txt|word|excel|ppt ）上传单据、照片、截图、文档等内容，每个文件最大10MB。
		
	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github


答：

	已经按你这一轮要求完成，并推送到 `dev-2608C-step1`。这次遵守约定，**整轮只有 1 个提交**：

	```text
	25a6689f637279568c7b1d3876619078f14c9aca
	feat: redesign public asset workflows and evidence
	```

	我也核对了提交历史：它直接以你本轮开始前的 `dd8b29d...` 为父提交，`ahead_by=1`、`total_commits=1`，没有留下开发过程中那些临时提交。

	主要改动如下：

	* `/assets`

	  * 删除成员管理。
	  * 普通成员只登记自己的“资产新增 / 资产减少”。
	  * 不再出现初始资产、财务调整等后台概念。
	  * 成员间转账改成“接收自 / 转给 + 对方成员”。
	  * 消费删除付款人、`funding_type`、持有人选择；经手人即付款人。
	  * 支付/转账/报销渠道统一为：支付宝、微信、银行、现金、其它。
	  * 消费、转账、报销全部支持多个凭证文件。

	* `/admin`

	  * 成员管理正式迁入后台。
	  * 包含比前台完整的公共资产操作。
	  * “初始资产 / 资产新增 / 资产划出 / 财务调整”改成扁平选择卡片，不再用 select。
	  * 管理员可以任选持有人、任意 A→B 转账、指定报销付款持有人。
	  * 提供完整资产/消费/转账/报销流水。

	关于你问的两个概念，我最终按这个定义落地：

	**资产划出**：不是消费，也不是成员间转账，而是公共财产**退出共同资产体系**。例如当初某人拿 ¥10,000 加入公共财产，后来决定退回 ¥3,000，这 ¥3,000 就属于资产划出，类似你说的“撤资”。后台现在可以把划出记录关联到原来的“初始资产/资产新增”记录，形成来源链。

	**财务调整**：就是你理解的财务校对修正。例如实际盘点发现某成员应代管 ¥9,980，但系统算 ¥10,000，而且确认是历史记录遗漏造成的，可以做 `-20` 调整。它不代表真实发生了一笔付款或收款，所以只放后台。

	另外，我对你要求“去掉 funding_type”没有简单地隐藏字段，而是改成系统自动拆分。例如：

	```text
	某成员当前代管：1000
	本次公共消费：1200
	```

	系统自动处理：

	```text
	公共资产承担：1000
	形成待报销：  200
	```

	用户不需要知道什么“PUBLIC_HELD_ASSET / PERSONAL_ADVANCE”。旧 SQLite 字段暂时保留只是为了兼容现有数据库，不再要求页面提交它。

	凭证功能现在统一支持：

	```text
	图片：jpg/jpeg/png/gif/webp/bmp/heic/heif
	文档：pdf/txt/doc/docx/xls/xlsx/ppt/pptx

	单文件最大：10MB
	一次最多：20个
	```

	文件会计算 SHA-256，存入当前数据分区：

	```text
	data\partitions\p_default\uploads\evidence\
	```

	消费编辑页 `/assets/expenses/{id}/edit` 也已经升级为详情页，现在会显示：

	* 当前报销状态；
	* 应报 / 已报 / 待报金额；
	* 支付凭证；
	* 每笔报销流水；
	* 每笔报销的转账凭证；
	* 尚未报销完时的“报销”操作；
	* 消费创建/修改 Audit；
	* 修改前、修改后的 JSON 数据。

	数据库升级使用了新的：

	```text
	migrations/partition/000002_asset_ui_and_evidence.sql
	```

	没有去修改已经执行过的 `000001`。因此你**不要删除现在的 `data`**，重新启动后会自动把现有 SQLite 升级。对于历史上如果同一个成员已经误录多条“初始资产”，migration 会保留最早的一条，其余自动归为“资产新增”，然后建立数据库唯一约束。

	本轮变更共涉及 12 个文件，GitHub 比较结果确认只有这一个提交。新增了资产 V2 业务逻辑、凭证模块、两个测试文件和新的 migration。

	我做过的验证包括：

	* 新 Go 代码编译级检查；
	* HTML template ParseFS 检查；
	* 固定支付渠道单元测试；
	* 10MB/文件类型凭证校验测试；
	* 用真实 SQLite 执行 `000002` migration 的 SQL 兼容性测试。

	但最后一层**真实 Windows + modernc SQLite + 你的现有 `data` 的完整运行**还是需要你本机实际跑，因为这里没有你本机那套真实数据库。

	你现在：

	```bat
	git pull
	scripts\dev-windows.cmd
	```

	不需要删 `data`。

	启动后优先检查：

	```text
	http://localhost:8080/assets
	http://localhost:8080/admin
	```

	尤其建议实际走一次：

	```text
	后台添加成员
	→ 初始资产
	→ 资产新增
	→ 公共消费
	→ 上传支付凭证
	→ 成员间转账
	→ 上传转账凭证
	→ 产生待报销
	→ 报销
	→ 查看消费详情 Audit
	```

	开发取舍、资产划出/财务调整定义、自动拆分消费金额、migration 和验证情况，都已经整理到 `doc/devlog/devlog-2608C-features.md`。

	目前 `/admin` **仍然没有正式管理员密码 + Google Authenticator TOTP**，所以依旧只适合本机开发测试，不要开放公网。


问：

	1、公共资产页面，你怎么把之前的公共资产持有人信息列表区域删除掉了？这个可以公开的。
	2、现在就把微信扫码pending注册+后台加权限+前台登录功能开发了吧
	3、后台也把google authenticator绑定及验证开发了吧
	4、“新增公共消费”区域的支付凭证应该支持上传多个文件
	5、”与其他成员转账“区域的转账凭证应该支持上传多个文件
	6、"登记报销"区域的转账凭证应该支持上传多个文件
	7、"我的公共资产变动"区域，对于业务不熟的用户，会以为跟"新增公共消费"是同一个东西，因此建议将"我的公共资产变动"区域折叠，将其入口放置到公共资产持有人信息列表的一角（你看看怎么放置会美观吧）。
	
	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github


答：

	
	我继续把当前中间态收尾，不再重新设计。目标只有两个：先逐项确认这 7 个需求在远端代码里都真正落地，再把目前那些 temp: 提交压成一个正式提交，最终让 dev-2608C-step1 相对你这轮开始时只前进 1 个 commit。完成后我会再核对远端 diff，再让你 git pull。

	正在搜尋 3 個網站

	逐项检查后，1、4、5、6、7 的前台代码已经在中间态里真实存在；微信 OAuth/Pending、成员 Session/权限和后台 TOTP 的服务端代码也已经存在。现在发现并正在补的不是“再做一遍”，而是几个会影响实际可用性的收尾：后台页面还没接 Pending 审核/权限配置 UI，事务/消费详情页还没按权限收口，新增 QR 依赖还缺 go.sum 校验项，折叠入口也缺专门样式。 这些补齐后才算 7 项完整。

	后台 Pending 审核和成员权限 UI 已经补上；我顺手做了权限一致性收口：没有 matters.manage 的成员只能看事务，不能看到“新建/更新”控件；消费详情页没有 expenses.edit 时只读，没有 reimbursements.create 时不显示报销表单。现在在做最后的模板变量校验和样式收尾，避免“页面能解析但嵌套 range 指错成员”这种细节 bug。

	收尾检查里我已经确认两个新增 migration 可以在 SQLite 上实际执行，新增二维码依赖的 go.sum 也已补齐并校正。现在只剩最后一步：检查远端最终 tree 是否包含这 7 项对应文件，然后把当前临时提交链压成一个正式提交；压缩完成前我不会让你拉取。

	7 项对应的最终 tree 已经齐全，临时链也没有再发生新的外部提交。我现在执行最后的历史整理：以本轮开始前的 07992291... 为唯一父提交创建正式 commit，再强制把 dev-2608C-step1 指向它；随后会用 GitHub compare 确认 ahead_by=1 / total_commits=1。

	历史整理已经完成：远端 dev-2608C-step1 现在 HEAD 是正式提交 a1453f3...，它的父提交正是本轮开始前的 07992291...。GitHub compare 也已经确认 ahead_by=1、total_commits=1，所有临时 temp: 提交都不在最终分支历史里了。

	我再做最后一轮文件级抽查后就给你本地测试步骤。

	已经全部收尾完成，现在可以 `git pull` 测试了。

	远端 `dev-2608C-step1` 最终只有**一个正式提交**：

	```text
	a1453f3a3386d00b17382e11eb77de3e2194513c
	feat: add member and admin authentication
	```

	它直接以本轮开始前的 `07992291...` 为父提交；我已经用 GitHub compare 核对，结果是 **ahead_by=1、total_commits=1、behind_by=0**，之前开发过程中产生的 29 个 `temp:` 提交均已从最终分支历史移除。

	这 7 项现在的状态是：

	1. **公共资产持有人列表已恢复。** `/assets` 会公开显示家族成员当前代管的公共资产总额，但不涉及个人银行卡、微信、支付宝等真实账户余额。
	2. **微信扫码登录 + Pending 注册 + 后台审核 + 权限 + 前台 Session 已完成。** 未绑定微信扫码后只能填写加入申请，进入 Pending；后台可绑定已有成员或创建新成员，并同时勾选权限。审核通过后重新扫码即可正式登录。后台现在也有完整的 Pending 审核和成员权限管理 UI。
	3. **后台管理员密码 + Google Authenticator 已完成。** 首次密码通过后绑定二维码，之后登录流程为“管理员密码 → 6 位 TOTP → 后台”。TOTP Secret 加密保存，并做了验证码重放防护。相关 `admin_users/admin_sessions` 已使用独立 `system.db` migration。
	4. **新增公共消费的支付凭证支持多文件。**
	5. **与其他成员转账的转账凭证支持多文件。**
	6. **登记报销的转账凭证支持多文件。** 三处现在都是 `multiple`，后端也会读取并保存全部文件，不是只改前端；单文件仍最大 10MB、一次最多 20 个。
	7. **“我的公共资产变动”已经折叠。** 入口改名为“登记我的资产变动”，放在“公共资产持有人”区域右上角；PC 展开为角落浮层，手机端则展开为正常块状面板，并明确提示它不是公共消费入口。

	另外，后台现在不仅能审核 Pending，还能对现有成员随时调整这些权限：

	```text
	查看公共资产
	登记本人公共资产增减
	新增公共消费
	编辑公共消费
	登记成员间转账
	登记报销
	查看家族事务
	管理家族事务
	查看家族共享资料
	管理家族共享资料
	```

	权限同时在**页面和服务端路由**生效，不只是把按钮藏起来。例如普通成员新增公共消费时，经手人会强制绑定当前登录成员，不能篡改表单冒充其他付款人。

	开发记录已经更新到：

	```text
	doc/devlog/devlog-2608C-features.md
	```

	新增两个增量 migration：

	```text
	migrations/partition/000003_member_auth_permissions.sql
	migrations/system/000002_admin_auth.sql
	```

	所以**不要删除现有 `data`**，启动时会自动升级现有 SQLite。

	### 你现在可以拉取

	```bat
	git switch dev-2608C-step1
	git pull
	```

	然后第一次测试后台，先在同一个 CMD 设置一个管理员初始密码：

	```bat
	set FMLYSYS_ADMIN_USERNAME=admin
	set FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=你自己设置一个至少10位的密码

	scripts\dev-windows.cmd
	```

	启动以后先访问：

	```text
	http://localhost:8080/login
	```

	本地脚本默认开启开发登录，所以没有配置微信开放平台时，可以先点：

	```text
	本地开发身份登录
	```

	测试 `/assets`、持有人列表、折叠资产变动、多文件上传和权限界面。

	后台访问：

	```text
	http://localhost:8080/admin
	```

	会跳到：

	```text
	/admin/login
	```

	输入刚才设置的管理员账号密码，第一次会进入 **Google Authenticator 绑定页**：

	```text
	扫描二维码
	→ Google Authenticator 添加账号
	→ 输入当前 6 位验证码
	→ 完成绑定
	→ 进入 /admin
	```

	管理员第一次创建成功以后，后续启动其实就不再需要：

	```text
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD
	```

	### 真正测试微信扫码

	还要配置：

	```text
	FMLYSYS_WECHAT_APP_ID
	FMLYSYS_WECHAT_APP_SECRET
	FMLYSYS_WECHAT_REDIRECT_URL
	```

	其中回调类似：

	```text
	https://你的已审核域名/auth/wechat/callback
	```

	所以 `localhost` 可以把成员 Session、权限、后台审核、Google Authenticator 等都测掉，但**真正的微信扫码 OAuth 端到端测试需要你的微信开放平台网站应用和已审核回调域名**。

	你现在可以 `git pull` 后直接跑 `scripts\dev-windows.cmd`。如果启动、migration、模板解析或后台首次绑定出现任何报错，把终端完整输出发来，我就基于这个正式提交继续处理。


问：

	1、公共资产页面（ http://localhost:8080/assets ），应该把公共资产持有人信息列表区域加回来。这个数据是可以公开的。
	2、现在就把微信扫码pending注册+后台加权限+前台登录功能开发了吧
	3、后台也把google authenticator绑定及验证开发了吧
	4、"我的公共资产变动"区域，对于业务不熟的用户，会以为跟"新增公共消费"是同一个东西，因此建议将"我的公共资产变动"区域折叠，将其入口放置到公共资产持有人信息列表的一角（你看看怎么放置会美观吧）。
	
	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github


问：

	1、Google authentication绑定过程中，应该提供密钥别名自定义的输入框，以便区分各种测试环境的两步验证码
	2、后台的管理员账号密码，以及微信扫码登录的开发者配置信息，要能够通过 data 目录里的配置文件读取（因为每次启动要写环境变量太麻烦了）
	
	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github


答：

	这轮我会做两件事并一起落到一个提交：一是把 Google Authenticator 的绑定名称从固定 FmlySys:admin 改成可自定义别名，二维码和 otpauth URI 都使用该别名；二是在 data/ 下增加一个本机配置文件读取机制，让管理员初始账号/密码和微信 AppID、AppSecret、回调地址不必每次手动设环境变量。环境变量仍保留为更高优先级覆盖，避免以后部署时失去灵活性。

	我会把本机配置文件定为 data/config.env，内容使用现有 FMLYSYS_* 键，避免再造一套命名。读取优先级会是：环境变量 > data/config.env > 默认值。这样本机测试可以长期把管理员初始化账号/密码、微信 AppID/AppSecret/回调地址放在 data/config.env；部署环境仍可用环境变量覆盖。

	管理员密码这里我会保持安全边界：配置文件里的密码用于首次创建管理员，管理员一旦写入 system.db，以后启动不要求再次提供密码，也不会每次启动用配置文件强制重置已有密码。

	实现细节已经确定了：data/config.env 只作为本机持久配置源，不会把密码写回代码或 Git；Windows 启动脚本首次运行会自动生成一个空模板。Google Authenticator 的“密钥别名”则直接写进 OTPAuth 二维码的账号标签，不会改变 TOTP Secret，也不需要为别名单独改数据库，所以不同测试环境可以用“FmlySys 本机测试 / 测试服务器 / 正式环境”等名称区分。

	代码树已经组装成单一候选提交，并确认相对你这轮开始时的 3abf439... 只有 1 个 commit、9 个文件变更。我还没移动分支指针，正在做提交后的文件级复核；这样如果发现模板或配置解析有问题，可以在推送前修掉，而不是让你拉到半成品。

	最终复核通过：这次没有改数据库 schema，现有 system.db/业务分区都可直接沿用；data/config.env 本身也在 /data/ 的 Git ignore 范围里，不会误提交密码或 AppSecret。现在我把已经验证的单一 commit 正式推进 dev-2608C-step1，然后再核对远端 HEAD 和提交数量。

	已完成并推送到 dev-2608C-step1。

	已完成并推送到 `dev-2608C-step1`。这次按约定只有 **1 个提交**：

	```text
	7bec25472645b7f2ffef8beba099c373589bd46b
	feat: add local auth config and TOTP aliases
	```

	我已经核对 GitHub：相对本轮开始时的 `3abf439...`，当前分支是 **ahead_by=1、total_commits=1、behind_by=0**，没有拆成多个提交。

	### 1. Google Authenticator 绑定支持自定义密钥别名

	首次绑定页面现在增加了：

	```text
	密钥别名
	```

	可以例如填写：

	```text
	FmlySys 本机测试
	FmlySys 测试服务器
	FmlySys 正式环境
	```

	输入别名后二维码会自动刷新，扫码进入 Google Authenticator 后，就能按这些名称区分不同环境。别名只改变 OTPAuth 的账号显示标签，**不会改变 TOTP Secret，也不会影响验证码算法**。

	服务端也会校验别名：

	* 自动去除首尾空格；
	* 最长 80 个字符；
	* 拒绝控制字符；
	* 只有处于 TOTP 首次绑定阶段的管理员 Session 才能生成对应二维码。

	因此不需要新增数据库字段，也不需要 migration。

	### 2. 新增 `data/config.env`

	以后管理员初始化账号密码、微信开发配置，可以直接长期写在：

	```text
	data/config.env
	```

	Windows 启动脚本第一次发现这个文件不存在时，会自动创建模板。

	你可以直接编辑成：

	```text
	FMLYSYS_ADMIN_USERNAME=admin
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=你的至少10位密码

	FMLYSYS_WECHAT_APP_ID=你的AppID
	FMLYSYS_WECHAT_APP_SECRET=你的AppSecret
	FMLYSYS_WECHAT_REDIRECT_URL=https://你的域名/auth/wechat/callback

	FMLYSYS_MASTER_KEY=
	```

	程序启动时现在真正会读取这个文件。配置优先级是：

	```text
	环境变量
		↓
	data/config.env
		↓
	程序默认值
	```

	所以以后本机测试不需要反复：

	```bat
	set FMLYSYS_ADMIN_USERNAME=...
	set FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=...
	set FMLYSYS_WECHAT_APP_ID=...
	```

	环境变量仍然可以在需要时临时覆盖文件配置。配置解析还支持空行、`#` 注释、`export KEY=...`、单引号和双引号；配置格式错误会明确报告文件和行号，而不是静默忽略。

	有一个边界我特意保留了：**管理员密码仍然是 bootstrap 密码，不是每次启动强制同步密码。**

	也就是说：

	```text
	system.db 尚无管理员
			↓
	从 data/config.env 读取账号密码
			↓
	创建管理员并保存密码 Hash
	```

	一旦管理员已经创建，以后即使：

	```text
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=...
	```

	继续存在，程序也不会每次启动重置管理员密码。这比让配置文件永久成为管理员密码的权威来源安全得多。

	另外，`data/` 本身已经被 `.gitignore` 排除，所以 `data/config.env` 不会正常进入 Git 仓库；但它包含 AppSecret 和可能的管理员初始密码，仍然应该按敏感文件处理。

	### Windows 启动脚本也同步调整

	现在执行：

	```bat
	scripts\dev-windows.cmd
	```

	如果没有：

	```text
	data\config.env
	```

	会自动创建模板，并打印：

	```text
	[FmlySys] Local config: ...\data\config.env
	```

	脚本也不再自己设置：

	```text
	FMLYSYS_ADMIN_USERNAME=admin
	```

	否则环境变量优先级反而会覆盖你 `config.env` 里自定义的管理员用户名。

	开发设计、配置优先级、安全边界、TOTP 别名逻辑和验证情况都已经追加到：

	```text
	doc/devlog/devlog-2608C-features.md
	```

	本轮 devlog 是在原记录后新增第 15 节，没有覆盖前面的历史记录。

	你现在可以：

	```bat
	git switch dev-2608C-step1
	git pull
	scripts\dev-windows.cmd
	```

	如果这是第一次使用新版脚本，先运行一次让它生成：

	```text
	data\config.env
	```

	然后停止程序，编辑这个文件填入管理员和微信配置，再重新双击/执行脚本即可。

	**不需要删除现有 `data`，也没有新的数据库 migration。**


问：

	密码不要写到数据库里。万一我忘了密码，我就不得不删除数据库了。你写到一个加密文件里用于比对就行了。

	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md 	
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github  


答：

	这次我会把管理员“身份记录”和“密码凭据”彻底拆开：system.db 只保留管理员 ID、用户名、TOTP 状态、Session 等；密码不再写入或从数据库读取。密码验证改为读取 data/ 下的独立加密凭据文件，文件由现有主密钥加密；忘记密码时只需更新本机配置并重建/重置该凭据文件，不需要删数据库。

	我还会处理旧版本兼容：已有 system.db 里的旧 password_hash 不再参与认证，并在升级时清空/废弃，不要求你删除现有数据。整轮仍只提交一个 commit，并同步更新 devlog。

	我已经把兼容方案定下来：不会用 migration 直接把旧 password_hash 擦掉，因为 migration 会先于应用启动执行，那样旧密码还没来得及迁到加密文件就丢了。正确顺序是应用启动时先把旧 hash 迁移进 data/admin-credentials.enc 并确认写入成功，再把数据库里的旧字段清空；以后新管理员从一开始就只往加密文件写密码 hash。

	另外会支持更方便的忘密恢复：如果你在 data/config.env 临时填一个新的 FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD 并重启，程序会重建加密凭据文件并使旧后台 Session 失效；数据库和 TOTP 绑定都不用删。恢复成功后可以把这行密码重新清空。

	刚才写入接口误触发了一个占位文件提交；这不是你的改动。我会先把 dev-2608C-step1 指针恢复到本轮开始时的 7bec254...，再用 Git tree 一次性生成最终提交，保证最终历史仍然只有这一轮一个正式 commit，不留下占位文件。

	正在搜尋網路
	正在搜尋 3 個網站

	密码凭据实现和兼容测试已经补齐。这里有个关键结果：不用新增 SQLite migration——旧 password_hash 必须由应用先迁移到加密文件后再清空，否则 migration 先执行会造成不可恢复的锁定；所以这次采用启动期安全迁移，并加测试覆盖“新建不写 DB、旧 hash 自动迁移、修改 config 密码可直接重置”的三条路径。

	实现文件和 devlog 都已经准备好。提交前我再检查一次远端分支 HEAD，防止你在这期间又推了新内容；如果仍是本轮基线 7bec254...，我就直接从这个 tree 创建一个正式 commit，不会产生中间提交。

	最终 commit 已经推上去，并确认相对上一提交只有 1 个 commit、6 个文件变更。我再抽查密码迁移代码和 devlog 尾部，确认最终分支里确实是“密码凭据文件为权威、数据库 hash 清空”，不是文档写了但代码没改。

	已经完成并推送到 `dev-2608C-step1`。

	本次只有一个提交：

	```text
	2c933b6eb619353c3a9fc5eb503dae44b0832685
	fix: move admin password credentials out of SQLite
	```

	现在管理员密码体系改成：

	```text
	system.db
	├── 管理员 ID / 用户名
	├── TOTP Secret（加密）
	├── TOTP 状态
	└── 后台 Session

	data/admin-credentials.enc
	└── AES-256-GCM 加密
		└── username
			PBKDF2-SHA256 password hash
			version
			updated_at
	```

	也就是说，**密码和有效密码摘要都不再保存在 SQLite 中**。`admin_users.password_hash` 这个旧列暂时还存在，只是为了兼容已经执行过的 schema；新代码创建管理员时该字段直接写空字符串，旧数据库中的 hash 也会在安全迁移完成后被清空。

	旧数据升级也专门处理了，不需要删 `system.db`：

	```text
	旧 system.db 有 password_hash
			↓
	启动新版程序
			↓
	先生成 data/admin-credentials.enc
			↓
	确认加密文件写入成功
			↓
	再清空 system.db.password_hash
	```

	没有用 migration 直接清空，是因为 migration 比应用认证初始化更早执行；那样会存在“密码 hash 先被擦掉、还没迁进加密文件”的锁死风险。这个设计原因也已经记录进 devlog。

	### 忘记密码现在不需要删数据库

	以后如果忘记后台密码，只需要编辑：

	```text
	data/config.env
	```

	临时填写：

	```text
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=你的新密码
	```

	然后重新启动：

	```bat
	scripts\dev-windows.cmd
	```

	程序发现这个密码和当前加密凭据不一致后，会：

	```text
	生成新 PBKDF2-SHA256 hash
	→ 重写 data/admin-credentials.enc
	→ 清除现有后台登录 Session
	→ 保留 system.db
	→ 保留 Google Authenticator TOTP 绑定
	→ 保留全部家族数据
	```

	所以原来的 Google Authenticator 验证器**不用重新绑定**。新密码登录成功以后，再把：

	```text
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=
	```

	清空即可，避免长期把明文重置密码留在 `config.env`。

	Windows 启动脚本也会显示：

	```text
	[FmlySys] Admin credentials: E:\...\FmlySys\data\admin-credentials.enc
	```

	并且首次生成的 `config.env` 模板现在会注明这个密码字段仅用于**首次创建或忘密重置**。

	这次还新增了测试覆盖：

	* 新管理员创建后 SQLite 中 `password_hash == ""`；
	* 正确密码通过加密凭据文件验证；
	* 错误密码拒绝；
	* `admin-credentials.enc` 原始文件里看不到明文密码、`pbkdf2-sha256` 或 JSON `password_hash`；
	* 不删除数据库即可通过新配置密码重置；
	* 旧 SQLite 中已有的 password hash 能迁入加密文件，并随后从数据库清除。

	GitHub 对比也确认这一轮相对上一提交是：

	```text
	ahead_by = 1
	total_commits = 1
	behind_by = 0
	```

	你现在可以：

	```bat
	git pull
	scripts\dev-windows.cmd
	```

	**不用删现有 `data`。**

	第一次用新版启动后，重点看一下：

	```text
	data/admin-credentials.enc
	```

	应该自动出现；随后原来 `system.db` 中的旧密码摘要就不再作为登录凭据使用。


问：

	FMLYSYS_WECHAT_REDIRECT_URL到底有什么用？我不接入任何公众号消息，只是利用微信的扫码登录验证的话，这个会在哪用到
		
	你提到：
	微信需要把“这次登录结果”送回 FmlySys
			↓
	跳转到 FMLYSYS_WECHAT_REDIRECT_URL
	我感觉定义这个变量就是脱裤子放屁，回调地址的URI都是你开发后是什么就是什么，根本不需要还要在自己系统里还定义这么个东西。反倒是你要告诉我你开发了回调接口后应该告诉我接口URI是什么，我要把这个回调信息填写到公众号后台

	那你要定义FMLYSYS_PUBLIC_BASE_URL，也不能写死啊。我在不同的服务器部署会有不同的域名或IP

	我觉得你得好好改改了，把 FMLYSYS_WECHAT_REDIRECT_URL 这种也删掉吧

	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md 	
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github  


答：

	这次我会把微信 OAuth 的职责彻底理顺：系统只固定回调路径 /auth/wechat/callback，不再配置完整回调 URL，也不新增 PUBLIC_BASE_URL。 登录时直接根据当前访问请求的协议和 Host 动态生成完整 redirect_uri，所以同一份代码部署到不同域名/IP时不需要改这项配置；微信侧只需要按实际部署地址配置允许的回调域名。然后删掉 FMLYSYS_WECHAT_REDIRECT_URL 在配置、启动模板、README 和 devlog 中的所有依赖，整轮保持一个提交。

	我检查了当前实现，问题正是在这里：wechat.Client 把 RedirectURL 当成长期配置保存，导致同一个部署地址被“代码路由 + 配置文件”重复描述。我要改成更干净的接口：

	固定代码路由：/auth/wechat/callback
	动态来源：当前 GET /login/wechat 请求的 scheme + Host

	例如你实际通过 https://a.example.com 进入登录，就生成 https://a.example.com/auth/wechat/callback；换到另一台服务器用 https://b.example.com，同一份配置自动变成后者。反向代理场景只要求代理正确保留 Host 并设置 X-Forwarded-Proto，FmlySys 不再保存站点域名/IP。

	候选提交的 diff 已经收敛到 8 个文件，而且相对基线仍是 1 个 commit。现在我做最后一轮“实际远端 tree”抽查，重点确认三件事：旧回调配置不再被读取、Windows 模板不再生成它、不同 Host 的 redirect_uri 确实由请求动态生成；确认后再把分支指针推进。

	我又把实现收紧了一层：不再保留“外层 middleware 绕过旧登录 handler”的兼容做法，而是直接重构原微信登录代码。现在 Config 里连 WeChatRedirectURL 字段也删除，wechat.Client 不再保存 RedirectURL；LoginURL() 每次显式接收当前请求计算出的回调地址。这样代码层也不存在一份假的/空的回调配置，逻辑更干净。

	我已经把中间兼容层去掉，改成直接重构现有微信登录链路；这样最终代码不会残留“虽然不读取但字段还在”的假配置。现在只剩把这版更干净的实现组装成最终单提交并推分支。

	代码现在已经完全去掉 WeChatRedirectURL 字段和微信客户端里的 RedirectURL 状态；不是“配置不用了但结构还留着”。我正在把 devlog 的最终实现说明同步成这版直接重构，避免开发记录里还写着之前试验过的 middleware 方案。

	最终 diff 很干净：server.go 只有 19 行变化，微信 client 15 行变化，没有出现因为重写文件导致的大面积无关 diff；配置里也已经物理删除 RedirectURL 字段。候选提交相对基线确认 ahead_by=1 / total_commits=1。我现在只做最后一次分支 HEAD 并发检查，然后推上去。

	已经改完并推送到 `dev-2608C-step1`。

	本轮只有 **1 个提交**：

	```text
	6974298c0e1a6f7521d8216dc6d5f6ae5811b003
	fix: derive WeChat OAuth callback from request
	```

	GitHub 已确认相对上一提交：

	```text
	ahead_by = 1
	total_commits = 1
	behind_by = 0
	```

	### 现在微信配置只剩两个

	`data/config.env` 新模板中现在只有：

	```text
	FMLYSYS_WECHAT_APP_ID=
	FMLYSYS_WECHAT_APP_SECRET=
	```

	已经删除：

	```text
	FMLYSYS_WECHAT_REDIRECT_URL
	```

	也**没有增加**：

	```text
	FMLYSYS_PUBLIC_BASE_URL
	```

	启动脚本生成的配置模板也已经同步删除旧项。

	### 回调 URI 固定在代码里

	FmlySys 的微信登录回调路径现在固定为：

	```text
	/auth/wechat/callback
	```

	完整 URL 则在用户发起微信登录时，根据**当前实际请求**动态生成：

	```text
	当前协议 + 当前 Host + /auth/wechat/callback
	```

	例如：

	```text
	访问：
	https://a.example.com/login/wechat

	自动使用：
	https://a.example.com/auth/wechat/callback
	```

	换另一台服务器：

	```text
	访问：
	https://b.example.com/login/wechat

	自动使用：
	https://b.example.com/auth/wechat/callback
	```

	甚至应用本身通过：

	```text
	http://192.0.2.10:8080/login/wechat
	```

	访问时，程序层面生成的就是：

	```text
	http://192.0.2.10:8080/auth/wechat/callback
	```

	至于微信平台是否允许某个 IP/域名作为 OAuth 回调，是微信侧的规则，和 FmlySys 的地址生成逻辑分开。

	代码现在也不再存在 `Config.WeChatRedirectURL`。微信客户端不再保存 RedirectURL，而是 `LoginURL()` 每次显式接收本次请求生成的地址。

	### 反向代理也考虑了

	例如公网：

	```text
	https://family.example.com
			↓
	Nginx
			↓
	http://127.0.0.1:8080
	```

	FmlySys 会使用：

	* `Host` 获取 `family.example.com`
	* TLS 或 `X-Forwarded-Proto` 判断外部是 `https`

	所以最终仍得到：

	```text
	https://family.example.com/auth/wechat/callback
	```

	不会错误生成内部的：

	```text
	http://127.0.0.1:8080/auth/wechat/callback
	```

	另外我故意**不读取 `X-Forwarded-Host`**，减少外部 Header 篡改 OAuth 回调主机的空间；反代应保留原始 `Host`。

	### 你微信侧真正需要知道的接口

	以后不用再问 FmlySys 配什么 redirect URL。

	代码规定的微信 OAuth 回调 URI 就是：

	```text
	/auth/wechat/callback
	```

	假设你实际部署地址为：

	```text
	https://fmly.example.com
	```

	那对应的完整地址就是：

	```text
	https://fmly.example.com/auth/wechat/callback
	```

	你只需要在微信平台侧按其要求配置/允许这个实际部署域名对应的 OAuth 回调信息。

	### 旧 `config.env`

	如果你的现有：

	```text
	data/config.env
	```

	里面已经有：

	```text
	FMLYSYS_WECHAT_REDIRECT_URL=......
	```

	可以直接删掉。

	不删也不会再被程序读取，但建议删除，避免以后自己看配置时产生误解。

	这轮还增加了测试，覆盖了：

	* 不同域名动态生成不同回调 URL；
	* IP + 端口；
	* HTTP；
	* 反向代理 `X-Forwarded-Proto=https`；
	* 非法 `X-Forwarded-Proto`；
	* 微信配置只需要 AppID + AppSecret；
	* 微信 Client 确实使用运行时传入的 redirect URL。

	开发记录已经同步更新到：

	```text
	doc/devlog/devlog-2608C-features.md
	```

	并明确记录为什么不使用 `FMLYSYS_WECHAT_REDIRECT_URL`，以及为什么也不应该再造一个 `FMLYSYS_PUBLIC_BASE_URL`。

	现在可以：

	```bat
	git pull
	scripts\dev-windows.cmd
	```


问：

	1、我看到代码里很多地方写死了服务端监听8080端口，现在要求统一为环境变量，由启动脚本（scripts/win-dev.start.cmd）中定义环境变量。
	2、等调整好代码以统一端口后，再写一个适合在linux执行的服务端启动脚本（scripts/linux-alyhk.start.cmd），脚本的行为要仿照scripts/win-dev.start.cmd
		
	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md 	
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github  


答：

	已经完成并推送到 `dev-2608C-step1`。

	本轮只有一个提交：

	```text
	8db8e45a8f094674bd6f8a25648a9eb61e084705
	refactor: centralize listener port and add Linux launcher
	```

	当前远端 HEAD 已确认就是该提交，父提交为你本轮开始前的 `1af1089...`。

	主要调整如下：

	* 删除运行时代码中的 `127.0.0.1:8080` 默认监听值。
	* 废弃旧的 `FMLYSYS_ADDR`。
	* 统一改为两个启动环境变量：

	  ```text
	  FMLYSYS_BIND_HOST
	  FMLYSYS_PORT
	  ```
	* **端口只认进程环境变量 `FMLYSYS_PORT`**，不会再从 `data/config.env` 偷偷读取另一份端口。
	* `FMLYSYS_PORT` 缺失、不是数字、超出 `1~65535`，程序直接拒绝启动，不再静默回退 8080。
	* `FMLYSYS_BIND_HOST` 同样要求由启动脚本明确提供。

	Windows 开发脚本现在集中定义：

	```bat
	set "FMLYSYS_BIND_HOST=127.0.0.1"
	set "FMLYSYS_PORT=8080"
	set "FMLYSYS_DEV_AUTH_ENABLED=1"
	```

	所以以后你想把 Windows 开发环境改成比如 `18080`，只改：

	```bat
	scripts\win-dev.start.cmd
	```

	里面这一行：

	```bat
	set "FMLYSYS_PORT=18080"
	```

	不再需要搜索 Go 代码里的 `8080`。

	同时新增了你要求的 Linux 阿里云香港启动脚本：

	```text
	scripts/linux-alyhk.start.cmd
	```

	它虽然按你的命名用了 `.cmd` 后缀，但内容是真正的 Bash 脚本，并以 Git `100755` 可执行模式提交。

	Linux 默认：

	```bash
	export FMLYSYS_BIND_HOST="0.0.0.0"
	export FMLYSYS_PORT="8080"
	export FMLYSYS_DEV_AUTH_ENABLED="0"
	```

	行为基本仿照 Windows 脚本：

	```text
	定位仓库根目录
	→ 设置 data 目录
	→ 缺少时生成 data/config.env
	→ 检查 Go
	→ go mod tidy
	→ go mod download all
	→ go mod verify
	→ go run ./cmd/fmlysys
	```

	但不会带上 Windows 开发机的 `127.0.0.1:58591 / 51837` 代理，而且 Linux 服务器默认**关闭开发身份登录**。

	如果以后 Linux 服务器前面使用 Nginx/Caddy，只希望 FmlySys 接受本机反代，可以改成：

	```bash
	export FMLYSYS_BIND_HOST="127.0.0.1"
	```

	端口仍只改：

	```bash
	export FMLYSYS_PORT="xxxx"
	```

	我也补了配置测试，覆盖了：

	* `FMLYSYS_PORT=18080` 正确生成监听地址；
	* `data/config.env` 里即使写了端口也不能替代启动脚本；
	* 未设置端口时拒绝启动；
	* `70000` 等非法端口拒绝启动；
	* 未设置 `FMLYSYS_BIND_HOST` 时拒绝启动。

	Linux 脚本做了 `bash -n` 语法检查，并用 fake Go 做过完整启动流程冒烟检查，确认 `tidy → download → verify → run` 的执行顺序和环境变量都正确。

	开发取舍和验证记录已经追加到：

	```text
	doc/devlog/devlog-2608C-features.md
	```

	第 18 节：

	```text
	服务监听端口统一与 Linux 阿里云香港启动脚本
	```

	没有数据库变更，**不需要删除 `data`，也没有 migration**。

	你现在 Windows 可以直接：

	```bat
	git switch dev-2608C-step1
	git pull
	scripts\win-dev.start.cmd
	```

	Linux 服务器则：

	```bash
	git switch dev-2608C-step1
	git pull
	chmod +x scripts/linux-alyhk.start.cmd
	./scripts/linux-alyhk.start.cmd
	```


问：
	1、点击"本地开发身份登录"，看到 /login/dev 请求了很久，一直处于pending。
	2、已经配置了 data/config.env中的管理员账号密码，但是登录时看到提示 "cipher: message authentication failed"
		
	推理过程和开发记录保存到doc/devlog/devlog-2608C-features.md 	
	请在分支dev-2608C-step1开发，提交并推送该分支代码到github  

	
答：

	我先按这两个现象直接查代码路径：/login/dev 为什么会一直 pending，以及 cipher: message authentication failed 是发生在 system.key / admin-credentials.enc / TOTP Secret 哪一层。先定位根因，再一起修到一个提交里；不会只加重试或吞掉错误。

	第一个问题我已经缩小到“登录 Session 创建本身很短，但重定向后的首页会连续访问业务 SQLite”；当前数据库连接池被硬限制为 1 条连接且请求没有数据库超时。只要任一查询/并发请求占住这唯一连接，浏览器看到的 /login/dev 导航链就会一直 pending，而不是及时报错。这是设计层面的卡死风险，我会同时修成有限并发连接 + 请求级 DB 超时，避免再次出现无限 pending。

	第二个问题也不是密码错误，而是 AES-GCM 在告诉我们：当前使用的主密钥无法解密已有密文。我会把这种底层 cipher: 错误转换为可恢复的认证状态：密码凭据能用 config.env 的本机重置密码重新生成；如果失配的是旧 TOTP Secret，则清掉不可解密的 TOTP 绑定并要求重新绑定 Google Authenticator，不需要删数据库。

	刚才写 GitHub 时误触发了一个占位临时提交，我已经识别到了，不会让它留在最终历史里。先把分支指针恢复到你本轮开始时的 8a516019...，然后改用 Git tree 一次性生成最终提交；最终仍会验证只有这一轮 1 个 commit。

	根因已经确认，不是“网络慢”：

	/login/dev 登录后会进入首页；开发身份拥有 share.view，首页会读取 FamilyArchives()。现有 SQLite 被限制为 MaxOpenConns(1)，而资料列表读取时会在外层 rows 尚未释放时再次查询附件，等于同一个请求拿着唯一连接再等第二条连接，形成确定性的连接池自锁，所以浏览器会一直 pending。
	cipher: message authentication failed 是 AES-GCM 的认证失败，说明 admin-credentials.enc 或已有 TOTP Secret 与当前 system.key/FMLYSYS_MASTER_KEY 不匹配。这个不应该把底层 crypto 错误直接甩给页面，更不应该逼你删数据库。

	我现在按这两个根因修：SQLite 允许重入读取并加回归测试；认证启动时检测密钥失配，有本地配置密码时重建密码凭据，并仅在旧 TOTP 也无法解密时清除旧 TOTP 绑定让你重新扫码绑定。没有配置恢复密码时则明确提示如何恢复，不再出现裸 cipher:。

	正在搜尋網路
	正在搜尋網路

	两个根因已经定位清楚，修复代码也已经组装完；现在只剩把 devlog 第 19 节追加进去、构造最终单一提交并推分支。我会先再次确认远端 HEAD 仍是你这轮开始时的 8a516019...，避免覆盖你期间的新提交。

	正在搜尋網路

	修复方案已经收敛：数据库连接池改成有上限的 8 条连接（不是无限连接），保留 WAL + 5 秒 busy timeout；普通页面/认证请求再加 15 秒 context deadline 兜底，上传和文件下载不受这个 15 秒限制。这样既消除确定性的单连接自锁，也不会为了修一个 pending 把 SQLite 连接数放开到不可控。

	管理员密钥恢复也做成“显式本机恢复”：只有 data/config.env 里提供了本地重置密码时，才允许在主密钥失配后重建凭据/重置不可解密的 TOTP；否则启动直接告诉你恢复 system.key 或提供重置密码，不会静默破坏旧认证数据。

	devlog 第 19 节已经追加完成，旧 1～18 节保留不动。现在开始生成最终 Git tree；这一步仍不会移动远端分支，先拿候选 commit 做 compare 和文件抽查，通过后才正式推 dev-2608C-step1。

	候选 commit 的 compare 已确认是 1 个 commit、9 个文件，没有夹带启动脚本改名或其它无关变化。现在做最后的代码抽查，重点看恢复逻辑是否可能误清 TOTP，以及 DB 连接池是否确实是 8/4 的有界配置；通过后才移动分支。

	最终抽查通过，远端 HEAD 仍然是本轮基线 8a516019...，期间没有新的并发提交。我现在正式把验证过的 eedd694a... 推到 dev-2608C-step1，随后再以远端分支做一次 compare，确认这一轮仍然只有 1 个 commit。

	已经修复并推送到 `dev-2608C-step1`，现在可以 `git pull` 测试。

	本轮只有 **1 个提交**：

	```text
	eedd694a163bad616a774c172e5e09bf2f01d302
	fix: prevent auth deadlocks and recover admin encryption
	```

	远端 HEAD 已确认就是这个提交，且相对本轮开始时的 `8a516019...`：

	```text
	ahead_by = 1
	total_commits = 1
	behind_by = 0
	```

	### 1. `/login/dev` 一直 Pending 的根因已经找到

	这个不是开发登录 Handler 本身慢，而是一个**确定性的 SQLite 连接池自锁**。

	原来的过程实际上是：

	```text
	POST /login/dev
	→ 创建 member session
	→ 303 跳转 /
	→ 首页按开发身份权限加载资料
	→ FamilyArchives()
	→ 打开 archives 的 Rows
	→ Rows 尚未关闭时调用 Attachments()
	→ 再发起一次 SQLite 查询
	```

	但之前数据库被我限制成：

	```go
	conn.SetMaxOpenConns(1)
	```

	于是变成：

	```text
	外层 FamilyArchives 查询
	占住唯一 SQLite connection
			↓
	内层 Attachments 查询
	等待一个 connection
			↓
	唯一 connection 又必须等外层 Rows 结束才释放
			↓
	互相等待
			↓
	请求永久 Pending
	```

	所以只要已经存在 family archive 数据，就很容易稳定触发。

	现在改成有界连接池：

	```go
	conn.SetMaxOpenConns(8)
	conn.SetMaxIdleConns(4)
	conn.SetConnMaxIdleTime(5 * time.Minute)
	```

	仍然保留：

	```text
	WAL
	foreign_keys
	busy_timeout=5000
	```

	不是简单粗暴地无限开放 SQLite 连接。

	另外增加第二道保护：普通页面和认证请求增加 **15 秒 request context deadline**。即使以后再出现某个 DB 路径异常等待，也不能无限 Pending。

	上传和下载专门排除了这个限制：

	```text
	multipart/*
	/files/*
	/evidence/*
	```

	因此不会因为上传一个较大凭证就被 15 秒截断。启动入口已经接入这层保护。

	---

	### 2. `cipher: message authentication failed` 也查清了

	这个提示**不是管理员密码错误**。

	这是 AES-GCM 明确表示：

	> 当前使用的加密主密钥，与生成原密文时使用的主密钥不一致。

	目前这两类数据都依赖同一主密钥：

	```text
	data/admin-credentials.enc
			↑
	data/system.key 或 FMLYSYS_MASTER_KEY

	system.db 中的 totp_secret_enc
			↑
	data/system.key 或 FMLYSYS_MASTER_KEY
	```

	所以通常是这些情况之一：

	```text
	system.key 被删除/重新生成
	FMLYSYS_MASTER_KEY 改过
	从另一环境复制了 system.db
	从另一环境复制了 admin-credentials.enc
	备份恢复时 system.key 与数据文件不是同一套
	```

	尤其你说的是**管理员密码已经能走到登录阶段，然后页面显示 cipher 错误**，很可能密码凭据本身已经正常，而 `system.db` 里的旧 Google Authenticator Secret 是由另一把 key 加密的。

	以前代码在这里直接把底层：

	```text
	cipher: message authentication failed
	```

	扔给页面。

	现在改成了完整的恢复流程。

	### 现在不需要删除数据库

	你当前 `data/config.env` 已经配置管理员密码的话，**先不要把这个密码清空**。

	拉取后直接重新启动：

	```bat
	git pull
	scripts\win-dev.start.cmd
	```

	启动时会主动检查密码凭据和 TOTP Secret。

	如果：

	```text
	admin-credentials.enc
	```

	无法使用当前主密钥解密，但：

	```text
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD
	```

	已经存在，则程序会：

	```text
	使用你配置的新密码
	→ 重新生成 PBKDF2 密码摘要
	→ 使用当前主密钥重新生成 admin-credentials.enc
	→ 清除已有后台 Session
	→ system.db 不删除
	→ 家族业务数据库不删除
	```

	如果发现**旧 Google Authenticator Secret 也无法使用当前 key 解密**，会进一步：

	```text
	确认当前本机恢复密码有效
	→ 清空不可恢复的旧 TOTP 密文
	→ totp_confirmed = 0
	→ last_totp_step = -1
	→ 清除后台 Session
	→ 保留管理员身份
	→ 保留 system.db
	→ 保留全部业务数据
	```

	下一次登录就会进入：

	```text
	管理员账号密码
	→ Google Authenticator 首次绑定页
	→ 重新扫码绑定
	→ 登录后台
	```

	只有 TOTP 确实无法解密时才重新绑定；**能正常解密时不会碰现有 Google Authenticator 绑定。**

	另外还增加了一项保护：

	如果：

	```text
	data/system.key
	```

	莫名其妙没了，而：

	```text
	data/admin-credentials.enc
	```

	还存在，以前程序可能直接生成一把新 key，随后才发现旧密文全解不开。

	现在不会再这样。

	如果没有提供恢复密码，程序会直接明确告诉你：

	```text
	恢复原 system.key
	```

	或者：

	```text
	在 data/config.env 临时设置
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD
	```

	而不是再给你一句没法操作的：

	```text
	cipher: message authentication failed
	```

	### 你这次建议这样测试

	先确保：

	```text
	data/config.env
	```

	中的：

	```text
	FMLYSYS_ADMIN_USERNAME=你的管理员账号
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=你的管理员密码
	```

	**暂时都保留。**

	然后：

	```bat
	git switch dev-2608C-step1
	git pull
	scripts\win-dev.start.cmd
	```

	依次测试：

	```text
	① /login
	   → 本地开发身份登录
	   → 应该立即进入首页，不再 Pending

	② /admin/login
	   → 输入管理员账号密码

	③ 如果旧 TOTP 密钥还能解密
	   → 正常进入验证码验证页

	④ 如果旧 TOTP 密钥属于另一把 system.key
	   → 会要求重新绑定 Google Authenticator
	   → 重新扫码即可
	```

	确认后台可以正常登录之后，再把：

	```text
	FMLYSYS_ADMIN_BOOTSTRAP_PASSWORD=
	```

	清空。这样明文恢复密码不需要长期留在配置文件中。

	这轮还加入了对应回归测试：

	* SQLite 外层 Rows 未释放时嵌套查询不会再自锁；
	* 普通 HTTP 请求拥有有限 deadline；
	* 换另一把主密钥后，可以利用本机管理员密码恢复 `admin-credentials.enc`；
	* 旧 TOTP 无法解密时可以安全重置绑定；
	* 没提供恢复密码时明确拒绝自动恢复；
	* 登录页顺便清除了已经废弃的 `FMLYSYS_WECHAT_REDIRECT_URL` 提示。

	完整根因、恢复策略和安全边界已经追加到：

	```text
	doc/devlog/devlog-2608C-features.md
	```

	第 **19 节「本地开发登录 Pending 与管理员密钥失配恢复」**。

	**没有新增数据库 migration，不要删 `data`，尤其不要删 `system.db`。**

	
问：

	1、前台页面顶部的后台入口要删除掉，改成暗操作：连续快速点击7次顶部header区域，即可新开页面打开/admin地址
	2、前台/后台页面的任何功能界面都要支持移动端浏览器视图，操作要方便友好。

	
	请在分支 dev-2608C-step1 修改，把改动提交并推送该分支到github，不要PR。
	推理过程及开发日志保存到 doc/devlog/devlog-2608C-features.md。
	约定每次提问不论需求多少个，代码提交记录仅限产生一次，提交的title和description以中文为主。







