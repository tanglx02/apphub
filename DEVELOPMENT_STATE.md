# DEVELOPMENT_STATE.md — 开发状态跟踪

> 每次修改代码后必须更新本文件。接手者先读 `PROJECT_SPEC.md`，再读本文件，再看代码。

## 基本信息

| 项 | 值 |
|---|---|
| 当前版本 | v1.1.0（前台只读导航） |
| 数据库版本 | 3（+ public_navigation：apps.public_visible 与前台设置） |
| API 版本 | v1（/api/v1/） |
| 前端构建 | web/dist 经 go:embed 嵌入二进制 |
| 日期 | 2026-09-29 |

## 当前完成状态：Phase 1-16 全部完成 ✅

| Phase | 内容 | 状态 |
|---|---|---|
| 1 | 项目骨架 / SQLite / 配置 / 日志 | ✅ |
| 2 | 登录 / 管理员 / Session / CSRF / 限流 / 审计 | ✅ |
| 3 | 应用 CRUD / 分类 / 排序 / 收藏 / 双地址 | ✅ |
| 4 | systemd 控制（白名单校验 + sudo 最小权限） | ✅ |
| 5 | Command 控制（进程组 / 超时 / 输出上限 / Shell 白名单开关） | ✅ |
| 6 | 健康检查（HTTP/TCP/Process/systemd + 后台缓存轮询） | ✅ |
| 7 | Dashboard（概览 / 卡片 / 搜索 / 拖拽排序 / 批量操作） | ✅ |
| 8 | 移动端适配（375-430px 单列 / 底部导航 / 大按钮） | ✅ |
| 9 | 日志（journalctl / 文件 tail / SSE 实时日志） | ✅ |
| 10 | 备份 / 恢复（VACUUM INTO 快照 + tar.gz） | ✅ |
| 11 | install / uninstall / manage 脚本 | ✅ |
| 12 | systemd 服务（RequiresMountsFor / 资源限制） | ✅ |
| 13 | upgrade / 回滚 | ✅ |
| 14 | 测试（Go 单测 + 集成测试 tests/integration/run.sh） | ✅ 32/0 |
| 15 | UI 打磨（Design Token / 深浅色 / Skeleton / Toast） | ✅ |
| 16 | 打包（build.sh → dist/ arm64 + amd64） | ✅ |

## 模块清单

```
cmd/apphub/main.go            启动 / CLI / TLS / 优雅退出
internal/api/                 路由、中间件、全部 HTTP 处理器
internal/auth/                Argon2id、会话、CSRF、限流、登录锁定
internal/config/              config.yaml 加载/保存/路径解析
internal/database/            连接、PRAGMA、迁移、settings、audit
internal/models/              数据模型与枚举
internal/apps/                应用/分类仓储 + 控制服务（状态机、批量并发）
internal/executor/            命令执行器、进程注册表（unix/windows 分文件）
internal/systemd/             systemctl 结构化调用
internal/health/              检测器 + StatusManager
internal/system/              主机信息采集（linux/windows 分文件）
internal/backup/              备份/恢复（tar.gz + VACUUM INTO）
internal/buildinfo/           版本信息（ldflags 注入）
web/                          Vue3 前端源码 + embed.go
scripts/                      install/uninstall/manage/backup/restore/upgrade
deploy/                       systemd 单元模板、sudoers 模板
tests/mock-app/               模拟 HTTP 应用（启停/检测/日志验证）
tests/integration/run.sh      端到端集成测试（32 项断言）
examples/apps.yaml            示例应用配置
```

## 已知设计决定（不要推翻）

1. **stdlib http.ServeMux**（Go 1.22 路由语法），不引入 chi。
2. **状态过渡态**：启动/停止后短暂显示 STARTING/STOPPING，真实检测确认
   ONLINE/OFFLINE 后立即结束过渡（`health.Manager.runCheck`）。
3. **备份文件名唯一化**：同一秒多次备份自动追加序号，防止恢复前自动备份覆盖源备份。
4. **恢复前置条件**：恢复要求服务停止；CLI `--restore` 通过 `SetDBCloser` 先释放连接，
   避免 Windows 文件占用问题（Linux unlink 语义天然安全）。
5. **Slugify 保留 CJK**：中文应用名生成中文 slug。
6. **--migrate / --init-db / --check / --backup / --restore** 均为一次性命令，执行后退出。
7. **前端轮询**（默认 5s，可配 3/5/10/30s），SSE 仅用于日志与状态事件流，不做全局状态推送依赖。
8. **批量操作并发上限 3**，防止单板机负载突增。

## v1.1 前台只读导航（2026-09-29 交付）

- 新增 Public API：`GET /api/v1/public/{config,apps,categories,summary,overview}`，
  仅 GET、匿名可访问、PublicAppDTO 字段过滤（严禁返回命令/路径/PID 等）。
- 数据库 v3：`apps.public_visible`（应用级"在前台显示"开关）+ 前台导航全局设置。
- 前端路由重构：`/` 前台只读导航（PublicHome，无需登录），`/admin` 后台（原有功能全部保留），
  旧地址 /login、/apps/:id、/settings/** 自动重定向。
- 前台功能：概览统计、分类筛选、搜索、名称/状态/分类排序、收藏（localStorage，不写库）、
  详情弹窗（打开内网/公网）、主题跟随配置、PWA manifest。
- 图标资源 `/uploads/*` 改为公开（前台无需登录即可显示图标）。
- 权限隔离验证：匿名访问所有管理 API（start/stop/restart/delete/update/settings/backup/audit）→ 401。
- 集成测试扩展至 49 项断言，全部通过；已部署 192.168.1.72 并线上验证。

## 未完成 / 待办（v1.2 候选）

- [ ] 在线检查新版本（当前仅显示版本号，按规格 v1 不做自动更新）
- [ ] operator / viewer 角色的 UI 权限区分（数据模型已预留 role 字段）
- [ ] 应用图标在线图标库
- [ ] 邮件 / Webhook 通知（应用异常时提醒）
- [ ] deploy/apphub.service 已由 install.sh 内嵌生成，模板文件可进一步抽取复用

## 已修复的重要 Bug（防止回归）

| 问题 | 根因 | 修复 |
|---|---|---|
| 恢复备份后数据丢失 | 同秒备份文件名冲突，恢复前自动备份覆盖源备份 | `uniqueBackupName` 唯一化 |
| 恢复后应用为空 | 残留 WAL 被新库回放 | 恢复时清理 db/-wal/-shm，占用时报明确错误 |
| --migrate 不退出 | 迁移后未 return | 已修复 |
| 状态长时间停留"启动中" | 过渡态固定 45s | 真实检测确认目标状态后立即结束过渡 |
| `--check` 在新目录失败 | 目录创建晚于自检 | 目录创建提前 |

## 测试状态

- `go test ./...`：全部通过（config/auth/apps/systemd/health/executor/backup/database）
- `bash tests/integration/run.sh`：32 通过 / 0 失败
  （Windows 环境自动降级：mock-app 直接调用二进制而非 shell 脚本）

## 下一步建议

1. 在真实 Armbian ARM64 设备执行 `sudo ./scripts/install.sh` 验证部署链路。
2. 配置 Tailscale/FRP 后验证公网访问 + HTTPS 自签名证书。
3. 首次运行后观察 SD 卡写入量（日志轮转、WAL checkpoint）。
