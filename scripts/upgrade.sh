#!/usr/bin/env bash
# =============================================================
# AppHub · 升级脚本（失败自动回滚）
# 用法：sudo ./scripts/upgrade.sh [新程序文件路径]
# 流程：停止 -> 备份(backups/pre-upgrade-时间) -> 替换 -> 迁移 -> 启动 -> 健康检查 -> 失败回滚
# =============================================================
set -uo pipefail

SERVICE_NAME="apphub.service"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
INSTALL_DIR="${APPHUB_DIR:-/opt/apphub}"
CONFIG="${INSTALL_DIR}/config.yaml"
BIN="${INSTALL_DIR}/apphub"

# 若脚本位于安装目录内（/opt/apphub/scripts），则升级该目录
if [ -f "${SRC_DIR}/apphub" ] && [ -f "${SRC_DIR}/config.yaml" ]; then
  INSTALL_DIR="${SRC_DIR}"
  CONFIG="${INSTALL_DIR}/config.yaml"
  BIN="${INSTALL_DIR}/apphub"
fi

NEW_BIN="${1:-}"

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  C_GREEN=$(tput setaf 2); C_YELLOW=$(tput setaf 3); C_RED=$(tput setaf 1); C_RESET=$(tput sgr0)
else
  C_GREEN=""; C_YELLOW=""; C_RED=""; C_RESET=""
fi
ok()   { printf '%s\n' "${C_GREEN}  ✓${C_RESET} $*"; }
warn() { printf '%s\n' "${C_YELLOW}  !${C_RESET} $*"; }
err()  { printf '%s\n' "${C_RED}[错误]${C_RESET} $*" >&2; }

printf '%s\n' "========================================="
printf '%s\n' "AppHub 升级程序"
printf '%s\n' "========================================="

if [ ! -f "${CONFIG}" ]; then
  err "未找到配置文件 ${CONFIG}，请确认安装目录"
  exit 1
fi

ARCH="$(uname -m)"
case "${ARCH}" in
  aarch64|arm64) BIN_NAME="apphub-linux-arm64" ;;
  x86_64|amd64)  BIN_NAME="apphub-linux-amd64" ;;
  *) BIN_NAME="apphub" ;;
esac

# 推断新程序文件
if [ -z "${NEW_BIN}" ]; then
  for candidate in "${SRC_DIR}/dist/${BIN_NAME}" "${SRC_DIR}/dist/apphub" "${SRC_DIR}/apphub"; do
    if [ -f "${candidate}" ] && [ "${candidate}" != "${BIN}" ]; then NEW_BIN="${candidate}"; break; fi
  done
fi
if [ -z "${NEW_BIN}" ] || [ ! -f "${NEW_BIN}" ]; then
  err "未找到新版程序文件。请将新二进制放到 dist/ 或作为参数传入。"
  echo "  示例：sudo ./scripts/upgrade.sh ./dist/apphub-linux-arm64"
  exit 1
fi

OLD_VERSION="$( "${BIN}" --version 2>/dev/null | head -n1 || echo 'unknown')"
NEW_VERSION="$( "${NEW_BIN}" --version 2>/dev/null | head -n1 || echo 'unknown')"
printf '%s\n' "  当前版本：${OLD_VERSION}"
printf '%s\n' "  新版本：  ${NEW_VERSION}"
printf '%s\n' "  安装目录：${INSTALL_DIR}"
printf '\n'
read -r -p "  确认升级？[y/N]: " CONFIRM || CONFIRM="n"
case "${CONFIRM}" in Y|y|yes|YES) ;; *) printf '%s\n' "已取消"; exit 0 ;; esac

# 1. 停止服务
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
  systemctl stop "${SERVICE_NAME}" && ok "已停止服务"
fi

# 2. 升级前备份
PRE_DIR="${INSTALL_DIR}/backups/pre-upgrade-$(date +%Y%m%d-%H%M%S)"
mkdir -p "${PRE_DIR}"
cp -f "${BIN}" "${PRE_DIR}/apphub.old" 2>/dev/null || true
cp -f "${CONFIG}" "${PRE_DIR}/config.yaml" 2>/dev/null || true
if [ -f "${INSTALL_DIR}/data/apphub.db" ]; then
  cp -f "${INSTALL_DIR}/data/apphub.db" "${PRE_DIR}/apphub.db" 2>/dev/null || true
fi
ok "已备份到 ${PRE_DIR}"

# 3. 替换程序
install -m 0755 "${NEW_BIN}" "${BIN}" && ok "已更新程序文件"

# 4. 数据库迁移（不启动服务）
(cd "${INSTALL_DIR}" && "${BIN}" --config "${CONFIG}" --migrate) && ok "数据库迁移完成"

# 5. 启动服务
STARTED=0
if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files 2>/dev/null | grep -q "${SERVICE_NAME}"; then
  systemctl daemon-reload
  systemctl start "${SERVICE_NAME}" 2>/dev/null && STARTED=1
else
  (cd "${INSTALL_DIR}" && nohup "${BIN}" --config "${CONFIG}" >>"${INSTALL_DIR}/logs/apphub.log" 2>&1 &)
  STARTED=1
fi

# 6. 健康检查
PORT="$(grep -E '^\s*port:' "${CONFIG}" | head -n1 | sed 's/[^0-9]//g')"
[ -z "${PORT}" ] && PORT="18080"
HEALTH_OK=0
for i in $(seq 1 15); do
  sleep 1
  if command -v curl >/dev/null 2>&1; then
    if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then HEALTH_OK=1; break; fi
  else
    if (exec 3<>/dev/tcp/127.0.0.1/"${PORT}") 2>/dev/null; then HEALTH_OK=1; break; fi
  fi
done

if [ "${HEALTH_OK}" = "1" ]; then
  ok "健康检查通过，升级完成"
  printf '\n%s\n' "  新版本：${NEW_VERSION}"
  exit 0
fi

# 7. 回滚
err "健康检查失败，正在回滚..."
if command -v systemctl >/dev/null 2>&1; then systemctl stop "${SERVICE_NAME}" 2>/dev/null || true; fi
pkill -f "${INSTALL_DIR}/apphub" 2>/dev/null || true

if [ -f "${PRE_DIR}/apphub.old" ]; then
  install -m 0755 "${PRE_DIR}/apphub.old" "${BIN}" && ok "已恢复旧程序"
fi
if [ -f "${PRE_DIR}/apphub.db" ]; then
  cp -f "${PRE_DIR}/apphub.db" "${INSTALL_DIR}/data/apphub.db" && ok "已恢复数据库"
  rm -f "${INSTALL_DIR}/data/apphub.db-wal" "${INSTALL_DIR}/data/apphub.db-shm"
fi
if [ -f "${PRE_DIR}/config.yaml" ]; then
  cp -f "${PRE_DIR}/config.yaml" "${CONFIG}" && ok "已恢复配置文件"
fi

if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files 2>/dev/null | grep -q "${SERVICE_NAME}"; then
  systemctl start "${SERVICE_NAME}" 2>/dev/null && ok "已用旧版本重新启动"
fi

err "升级已回滚，请查看日志排查原因：journalctl -u ${SERVICE_NAME} -n 100"
exit 1
