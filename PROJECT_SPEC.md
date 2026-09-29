# PROJECT_SPEC.md — AppHub 完整产品需求

> 本文件是项目的"出生证明"与最终设计目标。任何 AI 或开发者接手本项目前，
> 必须先阅读本文件，再阅读 `DEVELOPMENT_STATE.md`，然后检查代码。
> 不得假设功能不存在，不得重复开发已有功能，不得破坏既有架构。

## 一句话定位（v1.2 更新）

**低资源、可长期运行、带登录权限、支持内网/公网双地址、支持 HTTP/TCP/systemd/自定义命令、
支持一键启停、日志、备份、迁移、systemd、升级回滚的个人应用导航与管理中心（Personal Application Hub）。**

> v1.2 起应用分为两类：LOCAL（本机可管理：启停/systemd/命令）与
> EXTERNAL（非本地：仅导航 + 在线检测，后端拒绝一切服务控制）。

不是"应用导航网页"，而是"本地应用控制中心"。

## 核心目标

1. 统一管理 Armbian 上部署的 Web 应用、服务和工具。
2. 应用默认全部停止，需要时在页面手动启动（auto_start 默认关闭）。
3. 每个应用支持"内网地址"与"公网/穿透地址"，公网地址必须管理员手动配置。
4. 页面实时显示 ONLINE / OFFLINE / STARTING / STOPPING / ERROR / UNKNOWN。
5. 后台可增删改应用，可自定义启停命令与检测方式。
6. 一键安装/卸载/升级/回滚/备份/恢复，数据集中在项目目录便于整机迁移。

## 技术栈（已定型，禁止随意更换）

| 层 | 选型 | 约束 |
|---|---|---|
| 后端 | Go 1.25+，标准库 net/http（Go 1.22 路由语法） | 禁止引入重型 Web 框架 |
| 数据库 | modernc.org/sqlite（纯 Go，无 CGO） | WAL + busy_timeout + 参数化查询 |
| 前端 | Vue 3 + TypeScript + Vite + Tailwind CSS | 通过 go:embed 嵌入，生产无需 Node |
| 密码 | Argon2id（golang.org/x/crypto） | 禁止明文、禁止日志输出密码 |
| 配置 | config.yaml（gopkg.in/yaml.v3） | 相对路径以项目根目录解析 |

生产运行形态：**一个 Go 二进制 + SQLite + 项目目录**，不依赖 Docker / Nginx / 常驻 Node / Python。

## 目录结构（生产）

```
/opt/apphub
├── apphub              # 单一二进制（内嵌前端）
├── config.yaml
├── data/apphub.db      # SQLite（WAL）
├── logs/               # 自身日志（轮转 + 保留期清理）
├── backups/            # 备份（tar.gz）
├── runtime/            # PID 记录、应用输出日志
├── uploads/            # 应用图标
└── scripts/            # install/uninstall/manage/backup/restore/upgrade
```

## 数据库

表：`users`、`sessions`、`apps`、`categories`、`settings`、`audit_logs`、`backups`。
迁移机制：`internal/database/migrations.go`，按序号应用，`PRAGMA user_version` 记录版本；
**只允许追加迁移，禁止修改已发布迁移**，升级不得破坏旧库。

## 关键子系统

- **CommandExecutor**（internal/executor）：默认 executable+args 结构化执行；
  Shell 模式必须显式开启并警告；context 超时；输出上限（防内存膨胀）；
  Setpgid 独立进程组，停止时 SIGTERM → 优雅期 → SIGKILL 整组清理；PID 持久化。
- **systemd 客户端**（internal/systemd）：服务名白名单正则校验（防注入），
  结构化调用 systemctl，非 root 自动走 sudo（配合 /etc/sudoers.d/apphub 最小权限）。
- **StatusManager**（internal/health）：后台统一轮询 + 缓存 + 并发上限，
  浏览器只读缓存；过渡态（STARTING/STOPPING）在真实检测确认到达目标后立即结束。
- **认证**（internal/auth）：Argon2id、会话（DB 持久化 + 内存缓存）、CSRF Token、
  登录失败锁定、滑动窗口限流。
- **API**（internal/api）：统一 `/api/v1/`，响应包络
  `{success, data, message, request_id}`；安全 Header、X-Robots-Tag noindex、
  写请求 CSRF 校验、除登录/初始化外全部需要会话。

## 安全基线

- CSRF：SameSite=Lax Cookie + X-CSRF-Token 双重防护。
- 登录失败锁定（默认 5 次锁 15 分钟）+ 每 IP 限流。
- 安全 Header：nosniff / DENY / no-referrer / CSP / HSTS（仅 HTTPS）。
- 上传：类型白名单，SVG 内容过滤（script/事件属性/javascript: 协议）。
- 删除应用只删除 AppHub 内定义，**绝不删除用户实际项目目录**。
- AppHub 故障不得影响被管理应用；单个应用检测失败不得影响其他应用。

## 硬性平台要求

- ARM64 优先（Armbian / Debian 12），兼容 x86_64 Linux。
- SD 卡场景：systemd 单元包含 `RequiresMountsFor=/opt/apphub`。
- 低资源：空闲 RAM < 80MB，CPU 接近 0%，状态检测异步/缓存/限频。
- 日志与备份按保留期自动清理，避免写满 SD 卡。

## 交付定义（DoD）

能安装/卸载/升级/回滚/启停；应用增删改查与双地址；HTTP/TCP/Process/systemd 四种检测；
日志查看与 SSE 实时日志；搜索/分类/排序/收藏/批量操作（并发上限 3）；登录/改密/会话管理；
审计日志；备份/恢复/迁移；手机端 + PC 端 + 深浅色主题；ARM64 与 x86_64 产物；文档完整。

## 开发流程约定

Phase 1-16 依次为：骨架 → 认证 → CRUD → systemd → Command → 健康检查 → Dashboard →
移动端 → 日志 → 备份恢复 → 安装脚本 → systemd → 升级回滚 → 测试 → UI 优化 → 打包。
禁止：不断推翻架构；为修 Bug 大改无关模块；删除已完成功能而不说明原因。
每次修改代码后必须更新 `DEVELOPMENT_STATE.md`。
