# AppHub

**一个轻量、现代化的个人应用导航与管理中心（Personal Application Hub）。**

一个 Go 二进制 + SQLite，把你的所有应用集中到一块面板：
本机应用可以启动、停止、看日志；外部网站/服务可以直接导航、看在线状态——
不用再记 IP、端口和命令。

> 应用分两类：
> - **本地应用**：由 AppHub 所在设备运行，可启动 / 停止 / 重启 / 查日志（systemd 或自定义命令）
> - **非本地应用**：运行在别处的 Web 服务或第三方网站（NAS、云服务器、GitHub、ChatGPT…），只做导航 + 在线检测
>
> 非本地应用不支持也不提供任何服务控制，后端会直接拒绝对它的启停请求。

```
┌──────────────────────────────────────────────┐
│  Armbian AppHub        服务器正常运行  87%   │
├──────────────────────────────────────────────┤
│  12 应用   4 运行   8 停止   0 异常          │
├──────────────────────────────────────────────┤
│  🔐 Vaultwarden   ● 在线                     │
│     内网 http://192.168.1.72:8222  [打开]    │
│     公网 https://vault.example.com [打开]    │
│     [停止] [重启]                            │
├──────────────────────────────────────────────┤
│  🤖 CyberStrikeAI ○ 已停止   [启动] [更多]   │
└──────────────────────────────────────────────┘
```

## 特性

- **双类型应用**：本地应用（可管理）与非本地应用（纯导航），前台卡片带类型标签，后台按类型动态显示配置
- **双地址导航**：本地应用配置内网地址与公网地址（手动填写），另支持附加访问入口；一键复制 / 新标签页打开
- **四种在线检测**：HTTP（可配期望状态码）/ TCP / 进程 PID / systemd，后台统一轮询 + 缓存，浏览器不触发高频 shell
- **两类应用控制**：
  - `systemd`：结构化调用 systemctl（服务名白名单校验，防注入），支持启停/重启/开机自启/journal 日志
  - `command`：结构化 executable+args（Shell 模式需显式开启并警示），独立进程组、超时、输出上限、优雅停止（SIGTERM → SIGKILL 整组清理）
- **状态机防抖**：STARTING/STOPPING 过渡态防重复操作，真实检测确认后立即更新
- **安全**：Argon2id 密码、Session（HttpOnly/SameSite）、CSRF 双重防护、登录失败锁定、限流、审计日志、安全 Header、noindex
- **批量操作**：多选批量启停（并发上限 3）、"启动全部"默认关闭需在设置开启
- **备份/恢复/迁移**：SQLite 一致性快照 + 配置 + 图标打包；恢复前自动备份；整机目录复制即可迁移
- **低资源**：空闲 RAM < 80MB、CPU ≈ 0%，日志/备份自动按保留期清理（SD 卡友好）
- **界面**：Vue3 + Tailwind，深浅色主题（跟随系统），手机/平板/PC 自适应

## 快速开始

```bash
git clone https://github.com/tanglx02/apphub.git
cd apphub
./build.sh                 # 或直接下载 Release 压缩包
sudo ./scripts/install.sh
```

安装向导会依次询问：安装目录（默认 `/opt/apphub`，可自定义）、监听地址（默认 `0.0.0.0`）、
端口（默认 `18080`）、是否安装为 systemd 服务、是否开机自启。

安装完成后：

```bash
systemctl status apphub
# 浏览器打开
http://<服务器IP>:18080
```

首次访问进入初始化页面，创建管理员账户后即可使用。

> **默认不创建任何应用，也不开机自启任何应用。**
> 请在"设置 → 应用管理"中添加你的第一个应用。

## 添加应用

后台"添加应用"时首先选择类型：

- **本地应用**：进入完整管理配置（systemd 服务名 / 启停命令 / 工作目录 / 环境变量…）
- **非本地应用**：只填写名称、访问地址、健康检查方式，无任何管理配置项

### systemd 应用（本地）

例如管理 `cyberstrike-ai.service`：

| 字段 | 值 |
|---|---|
| 应用类型 | systemd 服务 |
| 服务名 | `cyberstrike-ai.service` |
| 状态检测 | systemd（`systemctl is-active`） |

### Command 应用（本地）

例如 `Vaultwarden`：

| 字段 | 值 |
|---|---|
| 启动命令 | `/opt/apps/vaultwarden/start.sh` |
| 停止命令 | `/opt/apps/vaultwarden/stop.sh` |
| 工作目录 | `/opt/apps/vaultwarden` |
| 状态检测 | TCP `127.0.0.1:8222` |

> 保存时**不会**执行任何命令；表单提供"测试启动 / 测试停止 / 测试状态"按钮，
> 由管理员主动点击验证。
>
> 非本地应用在后台既没有也没有必要有这些配置；即使手工构造请求，
> 对非本地应用调用启动 / 停止 / 重启接口也会被后端直接拒绝（400）。

### Shell 模式（慎用）

开启后命令通过 `/bin/sh -c` 执行，支持管道与重定向，但该应用将具备执行任意系统命令的能力，
请确保只有可信管理员能访问后台。界面上会有明确警示。

## 管理脚本

```bash
sudo /opt/apphub/scripts/manage.sh      # 交互式中文菜单
```

支持非交互调用：

```bash
/opt/apphub/scripts/manage.sh start | stop | restart | status | logs
```

| 脚本 | 用途 |
|---|---|
| `install.sh` | 一键安装（检测系统/架构/systemd、生成配置、创建服务） |
| `uninstall.sh` | 卸载（默认保留数据，可选备份后删除） |
| `manage.sh` | 交互式管理菜单（启停/日志/密码/备份/恢复） |
| `backup.sh` | 创建备份（优先使用 SQLite VACUUM INTO 一致性快照） |
| `restore.sh` | 恢复备份（停止服务 → 自动备份当前 → 恢复 → 启动） |
| `upgrade.sh` | 升级（备份 → 替换 → 迁移 → 健康检查 → 失败自动回滚） |

## 升级

```bash
sudo ./scripts/upgrade.sh ./dist/apphub-linux-arm64
```

流程：停止服务 → 备份到 `backups/pre-upgrade-<时间>/` → 替换程序 → 数据库迁移 →
启动 → 健康检查（`/health`）→ 失败自动回滚旧程序与旧数据库并重启。

## 备份与迁移

备份内容：`config.yaml`、SQLite 一致性快照、上传图标、元信息。

```bash
# 方式一：脚本
sudo /opt/apphub/scripts/backup.sh

# 方式二：后台 → 备份恢复 → 创建备份（可下载）
```

**整机迁移**：把 `/opt/apphub` 整个目录复制到新设备 → 安装 AppHub →
`restore.sh` 选择备份恢复 → 重启。应用导航配置即刻恢复。

## 配置

`config.yaml` 示例见 [config.example.yaml](config.example.yaml)。所有路径支持相对项目根目录，
整个目录复制到另一台设备即可恢复。常用项：

```yaml
server:
  host: 0.0.0.0
  port: 18080
  https: false          # 启用后默认自动生成自签名证书
  redirect_http: false
  trusted_proxies: ""   # 有反向代理时填写，如 127.0.0.1/32
status:
  interval: 5           # 后台状态检测周期（秒）
security:
  session_timeout: 86400
  login_max_fails: 5
  rate_limit_per_min: 120
```

修改监听地址 / 端口 / HTTPS 需重启服务；其余设置在后台保存后即时生效。

## HTTPS

后台"设置 → HTTPS"开启即可。证书不存在时自动生成自签名证书（有效期 10 年），
也可指定自有证书路径（`data/certs/server.crt` / `.key`）。支持 HTTP → HTTPS 重定向。

## systemd（Linux 环境的服务管理方式）

systemd 是 AppHub 在 Linux 环境下管理本地应用的推荐方式，但不是产品身份的一部分。
`install.sh` 生成的服务单元（`/etc/systemd/system/apphub.service`）要点：

```ini
After=network-online.target
RequiresMountsFor=/opt/apphub   # SD 卡/数据盘挂载后再启动
Restart=on-failure
RestartSec=5
MemoryMax=512M
CPUQuota=50%
```

在后台"安全设置 → AppHub 服务管理"也可以直接启停 / 开关自启。

非 root 运行时，AppHub 通过 sudo 调用 systemctl，安装脚本会写入
`/etc/sudoers.d/apphub` 最小权限白名单（仅允许 systemctl 对服务的启停与状态查询），
**绝不使用 `NOPASSWD: ALL`**。

## API

统一前缀 `/api/v1/`，除登录与初始化外均需会话；写请求需携带 `X-CSRF-Token`。

```
GET  /health                      GET  /ready
POST /api/v1/auth/login|setup|logout
GET  /api/v1/auth/me              POST /api/v1/auth/password
GET  /api/v1/apps                 POST /api/v1/apps
GET  /api/v1/apps/:id             PUT  /api/v1/apps/:id      DELETE /api/v1/apps/:id
POST /api/v1/apps/:id/start|stop|restart
GET  /api/v1/apps/:id/status      GET  /api/v1/apps/:id/logs?lines=100&follow=1
POST /api/v1/apps/batch           POST /api/v1/apps/start-all
GET  /api/v1/apps/export          POST /api/v1/apps/import   (JSON/YAML)
GET  /api/v1/categories           POST|PUT|DELETE /api/v1/categories
GET  /api/v1/system/info          GET  /api/v1/system/resources
GET  /api/v1/system/events        (SSE 状态流)
GET|PUT /api/v1/settings
GET  /api/v1/audit                DELETE /api/v1/audit
GET|POST /api/v1/backups          POST /api/v1/backups/restore
```

响应包络：

```json
{ "success": true, "data": {}, "message": "", "request_id": "1-abcdef01" }
```

CLI：

```bash
apphub --version
apphub --config /opt/apphub/config.yaml
apphub --check                 # 环境与配置自检
apphub --migrate               # 数据库迁移后退出
apphub --reset-admin-password  # 交互式重置密码（使全部会话失效）
apphub --backup [目录]          # 命令行备份
apphub --restore <文件>         # 命令行恢复
```

## 开发

```bash
./build.sh              # 前端 + 后端全量构建 → dist/
./build.sh arm64        # 仅构建 arm64
./build.sh --skip-web   # 使用已有 web/dist
cd web && npm run dev   # 前端开发（API 代理到 18080）
go test ./...           # 单元测试
bash tests/integration/run.sh   # 集成测试
```

源码结构、模块说明与设计决定见 [PROJECT_SPEC.md](PROJECT_SPEC.md) 与
[DEVELOPMENT_STATE.md](DEVELOPMENT_STATE.md)。

## 故障排查

```bash
systemctl status apphub                 # 服务状态
journalctl -u apphub -n 100             # 最近日志
journalctl -u apphub -f                 # 实时日志
ss -lntp | grep 18080                   # 端口监听
curl http://127.0.0.1:18080/health      # 健康检查（200）
curl http://127.0.0.1:18080/ready       # 就绪检查（数据库异常返回 503）
/opt/apphub/apphub --config /opt/apphub/config.yaml --check   # 自检
ls -lh /opt/apphub/data/apphub.db       # 数据库文件
```

| 现象 | 处理 |
|---|---|
| 服务起不来 | `journalctl -u apphub -n 50`，常见为端口占用或目录权限 |
| 页面 502/无法访问 | 确认监听地址与端口、防火墙放行 |
| 忘记密码 | `sudo /opt/apphub/scripts/manage.sh` → 10 重置管理员密码 |
| 应用状态一直是"启动中" | 检查应用的启动命令与状态检测目标是否匹配 |
| systemctl 报权限错误 | root 运行，或确认 `/etc/sudoers.d/apphub` 存在且 `visudo -cf` 通过 |
| SD 卡空间不足 | 检查 `logs/` 与 `backups/` 保留天数设置，两者均自动清理 |

## 平台说明

AppHub 是纯 Go 静态二进制，可在主流 Linux 环境（amd64 / arm64）部署。
目前实际测试平台：Armbian 26.11（aarch64）与 Windows（开发环境），
未在其他平台做过完整验证，请以实际测试为准。

## 安全建议

- 通过 Tailscale / FRP / Cloudflare Tunnel 暴露公网时，**务必开启 HTTPS 并使用强密码**
- 管理界面默认全局需要登录，禁止搜索引擎索引（`X-Robots-Tag: noindex, nofollow`）
- Shell 模式应用具备执行系统命令的能力，请确保后台只有可信管理员访问
- 定期执行备份；升级脚本在失败时会自动回滚

## 许可

MIT
