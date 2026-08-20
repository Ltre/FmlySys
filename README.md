# FmlySys

家族公共事务与共同资产治理系统。

## Step1 当前实现

- Go 单体 Web 框架、SQLite 与默认数据分区；
- 公共资产：初始/新增/划出、持有人虚拟账户、消费、个人垫付、分次报销、内部转账、一致性检查；
- 家族事务：父子事务、状态、负责人、日期、关联公共支出；
- 信息共享：分类资料、正文、附件上传与下载；
- 关键写操作 Audit 基础；
- `/admin` 路由框架。

## 本地运行

```bash
go mod download
go run ./cmd/fmlysys
```

默认访问：`http://127.0.0.1:8080`。

可配置：

```text
FMLYSYS_ADDR=:8080
FMLYSYS_DATA_DIR=data
FMLYSYS_DEV_MEMBER=开发管理员
```

> 当前 `dev-2608C-step1` 为开发阶段，普通成员微信认证及 `/admin` 的正式 TOTP 登录尚未接入，系统会自动建立一个开发态成员用于验证核心功能。
