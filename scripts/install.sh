#!/usr/bin/env bash
# =============================================================
# AppHub · 一键安装脚本
# 适用：Debian 12 / Armbian / Ubuntu（ARM64 与 x86_64）
# 用法：sudo ./scripts/install.sh
# =============================================================
set -euo pipefail

APP_NAME="apphub"
SERVICE_NAME="apphub.service"
DEFAULT_DIR="/opt/apphub"
DEFAULT_PORT="18080"

# ---------- 基础输出 ----------
if [ -t 1 ] && command -v tput >/dev/null 2>&1 && [ -n "${TERM:-}" ] && [ "${TERM}" != "dumb" ]; then
  C_RESET=$(tput sgr0); C_GREEN=$(tput setaf 2); C_YELLOW=$(tput setaf 3)
  C_RED=$(tput setaf 1); C_BLUE=$(tput setaf 4); C_BOLD=$(tput bold)
else
  C_RESET=""; C_GREEN=""; C_YELLOW=""; C_RED=""; C_BLUE=""; C_BOLD=""
fi

info()  { printf '%s\n' "${C_BLUE}[信息]${C_RESET} $*"; }
ok()    { printf '%s\n' "${C_GREEN}  ✓${C_RESET} $*"; }
warn()  { printf '%s\n' "${C_YELLOW}  !${C_RESET} $*"; }
err()   { printf '%s\n' "${C_RED}[错误]${C_RESET} $*" >&2; }
title() { printf '\n%s\n' "${C_BOLD}$*${C_RESET}"; }
hr()    { printf '%s\n' "========================================="; }

ask() { # ask "提示" "默认值" -> 写入 REPLY
  local prompt="$1" default="$2"
  if [ -n "${APPHUB_ASSUME_YES:-}" ]; then REPLY="$default"; return; fi
  read -r -p "  ${prompt} [${default}]: " REPLY || true
  [ -z "${REPLY}" ] && REPLY="$default"
}

# ---------- 定位源码目录 ----------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

hr
title "AppHub 安装程序"
hr

# ---------- 环境检测 ----------
title "检测系统..."

if [ "$(uname -s)" != "Linux" ]; then
  err "AppHub 仅支持 Linux 系统"
  exit 1
fi
ok "Linux"

ARCH_RAW="$(uname -m)"
case "${ARCH_RAW}" in
  aarch64|arm64)   ARCH="arm64"; BIN_NAME="apphub-linux-arm64" ;;
  x86_64|amd64)    ARCH="amd64"; BIN_NAME="apphub-linux-amd64" ;;
  armv7l|armv6l)   ARCH="armv7"; BIN_NAME="apphub-linux-armv7" ;;
  *) err "暂不支持的 CPU 架构：${ARCH_RAW}"; exit 1 ;;
esac
ok "${ARCH_RAW} (${ARCH})"

KERNEL="$(uname -r)"
ok "内核 ${KERNEL}"

HAS_SYSTEMD=0
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  HAS_SYSTEMD=1
  ok "systemd 可用"
else
  warn "未检测到 systemd，将跳过服务安装（可用 scripts/manage.sh 手动启动）"
fi

if command -v tar >/dev/null 2>&1 && command -v gzip >/dev/null 2>&1; then
  ok "tar / gzip 可用（备份依赖）"
else
  warn "缺少 tar 或 gzip，备份功能可能不可用"
fi

# ---------- 查找程序文件 ----------
BIN_SRC=""
for candidate in \
  "${SRC_DIR}/dist/${BIN_NAME}" \
  "${SRC_DIR}/dist/apphub" \
  "${SRC_DIR}/apphub" \
  "${SRC_DIR}/bin/${BIN_NAME}"; do
  if [ -f "${candidate}" ]; then BIN_SRC="${candidate}"; break; fi
done

if [ -z "${BIN_SRC}" ]; then
  if command -v go >/dev/null 2>&1; then
    info "未找到预编译程序，尝试使用 Go 编译（约需 1-3 分钟）..."
    (cd "${SRC_DIR}" && GOOS=linux GOARCH="${ARCH}" CGO_ENABLED=0 go build -trimpath -o "dist/${BIN_NAME}" ./cmd/apphub) \
      && BIN_SRC="${SRC_DIR}/dist/${BIN_NAME}"
  fi
fi

if [ -z "${BIN_SRC}" ] || [ ! -f "${BIN_SRC}" ]; then
  err "未找到 AppHub 程序文件，请先执行 ./build.sh 构建，或下载 Release 压缩包"
  exit 1
fi
ok "程序文件 ${BIN_SRC}"

# ---------- 交互配置 ----------
title "安装配置"

ask "安装目录" "${DEFAULT_DIR}"
INSTALL_DIR="${REPLY}"
ask "监听地址" "0.0.0.0"
LISTEN_HOST="${REPLY}"
ask "监听端口" "${DEFAULT_PORT}"
LISTEN_PORT="${REPLY}"

INSTALL_SYSTEMD="n"
ENABLE_BOOT="n"
if [ "${HAS_SYSTEMD}" = "1" ]; then
  ask "是否安装为 systemd 服务" "Y"
  case "${REPLY}" in Y|y|yes|YES) INSTALL_SYSTEMD="y";; esac
  if [ "${INSTALL_SYSTEMD}" = "y" ]; then
    ask "是否开机自动启动" "Y"
    case "${REPLY}" in Y|y|yes|YES) ENABLE_BOOT="y";; esac
  fi
fi

# ---------- 执行安装 ----------
title "开始安装"

RUN_USER="root"
CREATE_USER="n"
if [ "${HAS_SYSTEMD}" = "1" ] && id root >/dev/null 2>&1; then
  ask "是否创建专用系统用户 apphub（推荐，需配合 sudo 权限管理其它服务）" "n"
  case "${REPLY}" in Y|y|yes|YES) CREATE_USER="y";; esac
fi

mkdir -p "${INSTALL_DIR}"/{data,logs,backups,runtime,uploads,scripts}
ok "创建目录 ${INSTALL_DIR}"

install -m 0755 "${BIN_SRC}" "${INSTALL_DIR}/apphub"
ok "安装程序 ${INSTALL_DIR}/apphub"

# 脚本
if [ -d "${SRC_DIR}/scripts" ]; then
  cp -f "${SRC_DIR}"/scripts/*.sh "${INSTALL_DIR}/scripts/" 2>/dev/null || true
  chmod +x "${INSTALL_DIR}"/scripts/*.sh 2>/dev/null || true
  ok "安装管理脚本"
fi

# 示例与文档
[ -f "${SRC_DIR}/README.md" ] && cp -f "${SRC_DIR}/README.md" "${INSTALL_DIR}/README.md"
[ -d "${SRC_DIR}/examples" ] && { mkdir -p "${INSTALL_DIR}/examples"; cp -f "${SRC_DIR}"/examples/* "${INSTALL_DIR}/examples/" 2>/dev/null || true; }

# 生成配置文件（不覆盖已有配置）
if [ ! -f "${INSTALL_DIR}/config.yaml" ]; then
  cat > "${INSTALL_DIR}/config.yaml" <<EOF
# AppHub 配置文件
# 所有相对路径均相对于本文件所在目录（项目根目录）

server:
  host: ${LISTEN_HOST}
  port: ${LISTEN_PORT}
  https: false
  redirect_http: false
  http_port: $((LISTEN_PORT + 1))
  trusted_proxies: ""
  site_name: AppHub
  logo: ""
  timezone: Asia/Shanghai
  read_timeout_sec: 30
  write_timeout_sec: 60

database:
  path: ./data/apphub.db
  busy_timeout_ms: 5000

logging:
  level: info
  retention_days: 30
  max_size_mb: 20
  console: true

security:
  session_timeout: 86400
  login_max_fails: 5
  login_lock_minutes: 15
  rate_limit_per_min: 120
  csrf_enabled: true
  cookie_secure: auto

status:
  interval: 5
  timeout: 5
  concurrency: 4
  start_timeout: 60
  stop_timeout: 30
  restart_timeout: 90

tls:
  enabled: false
  self_signed: false
  cert_file: ./data/certs/server.crt
  key_file: ./data/certs/server.key
  auto_generate: true

backup:
  directory: ./backups
  retention_days: 30
  auto_enabled: false

executor:
  max_output_bytes: 1048576
  kill_process_tree: true
  grace_period_sec: 10
EOF
  ok "生成配置文件"
else
  warn "config.yaml 已存在，保持原配置"
fi

# 系统用户
if [ "${CREATE_USER}" = "y" ]; then
  if ! id -u apphub >/dev/null 2>&1; then
    useradd --system --home-dir "${INSTALL_DIR}" --shell /usr/sbin/nologin apphub 2>/dev/null || \
      useradd -r -d "${INSTALL_DIR}" -s /usr/sbin/nologin apphub || warn "创建用户失败，继续使用 root"
  fi
  RUN_USER="apphub"
  chown -R apphub:apphub "${INSTALL_DIR}" 2>/dev/null || true
  ok "已创建系统用户 apphub"

  # 最小 sudo 权限：仅允许 systemctl 管理服务
  if command -v sudo >/dev/null 2>&1 && [ -d /etc/sudoers.d ]; then
    cat > /etc/sudoers.d/apphub <<'EOF'
# AppHub 最小权限：仅允许 systemctl 对指定服务执行状态查询与启停
# 修改本文件后请使用 visudo -cf /etc/sudoers.d/apphub 校验
apphub ALL=(root) NOPASSWD: /bin/systemctl start apphub.service
apphub ALL=(root) NOPASSWD: /bin/systemctl stop apphub.service
apphub ALL=(root) NOPASSWD: /bin/systemctl restart apphub.service
apphub ALL=(root) NOPASSWD: /bin/systemctl status apphub.service
apphub ALL=(root) NOPASSWD: /bin/systemctl is-active *
apphub ALL=(root) NOPASSWD: /bin/systemctl show *
apphub ALL=(root) NOPASSWD: /bin/systemctl start *.service
apphub ALL=(root) NOPASSWD: /bin/systemctl stop *.service
apphub ALL=(root) NOPASSWD: /bin/systemctl restart *.service
apphub ALL=(root) NOPASSWD: /usr/bin/journalctl -u *
EOF
    chmod 0440 /etc/sudoers.d/apphub
    ok "已写入 /etc/sudoers.d/apphub（最小权限）"
  fi
fi

# systemd 服务
if [ "${INSTALL_SYSTEMD}" = "y" ]; then
  cat > "/etc/systemd/system/${SERVICE_NAME}" <<EOF
[Unit]
Description=AppHub - 本地应用导航与服务控制中心
Documentation=https://github.com/tanglx02/apphub
After=network-online.target
Wants=network-online.target
# SD 卡 / 独立数据盘场景：确保项目目录所在文件系统已挂载后再启动
RequiresMountsFor=${INSTALL_DIR}
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
User=${RUN_USER}
Group=${RUN_USER}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/apphub --config ${INSTALL_DIR}/config.yaml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5
TimeoutStopSec=20
KillSignal=SIGTERM
KillMode=mixed
StandardOutput=journal
StandardError=journal
SyslogIdentifier=apphub

# 资源限制（低资源设备友好，可按需调整）
MemoryMax=512M
CPUQuota=50%
TasksMax=256
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=false
ReadWritePaths=${INSTALL_DIR}

[Install]
WantedBy=multi-user.target
EOF
  ok "安装 systemd 服务 /etc/systemd/system/${SERVICE_NAME}"

  systemctl daemon-reload
  ok "daemon-reload"

  if [ "${ENABLE_BOOT}" = "y" ]; then
    systemctl enable "${SERVICE_NAME}" >/dev/null 2>&1 && ok "已设置开机自动启动"
  else
    warn "未设置开机自动启动（可用 systemctl enable ${SERVICE_NAME} 开启）"
  fi

  systemctl restart "${SERVICE_NAME}" >/dev/null 2>&1 || systemctl start "${SERVICE_NAME}"
  sleep 2
  if systemctl is-active --quiet "${SERVICE_NAME}"; then
    ok "服务已启动"
  else
    warn "服务启动异常，请执行 journalctl -u ${SERVICE_NAME} -n 50 查看日志"
  fi
else
  info "未安装 systemd 服务，可手动启动：cd ${INSTALL_DIR} && ./apphub --config ./config.yaml"
fi

# ---------- 完成 ----------
PRIMARY_IP="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}' | head -n1)"
[ -z "${PRIMARY_IP}" ] && PRIMARY_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -z "${PRIMARY_IP}" ] && PRIMARY_IP="127.0.0.1"

hr
title "安装完成"
hr
printf '%s\n' "  安装目录： ${INSTALL_DIR}"
printf '%s\n' "  配置文件： ${INSTALL_DIR}/config.yaml"
printf '%s\n' "  数据库：   ${INSTALL_DIR}/data/apphub.db"
printf '%s\n' "  日志目录： ${INSTALL_DIR}/logs"
printf '%s\n' "  备份目录： ${INSTALL_DIR}/backups"
printf '\n'
printf '%s\n' "  访问地址： http://${PRIMARY_IP}:${LISTEN_PORT}"
printf '\n'
printf '%s\n' "  首次访问会进入初始化页面，请创建管理员账户。"
printf '%s\n' "  管理命令： ${INSTALL_DIR}/scripts/manage.sh"
if [ "${INSTALL_SYSTEMD}" = "y" ]; then
  printf '%s\n' "  服务状态： systemctl status ${SERVICE_NAME}"
fi
hr
