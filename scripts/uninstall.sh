#!/usr/bin/env bash
# =============================================================
# AppHub · 卸载脚本
# 用法：sudo ./scripts/uninstall.sh
# 默认保留应用数据，避免误删用户配置
# =============================================================
set -euo pipefail

SERVICE_NAME="apphub.service"
DEFAULT_DIR="/opt/apphub"

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  C_RESET=$(tput sgr0); C_GREEN=$(tput setaf 2); C_YELLOW=$(tput setaf 3)
  C_RED=$(tput setaf 1); C_BOLD=$(tput bold)
else
  C_RESET=""; C_GREEN=""; C_YELLOW=""; C_RED=""; C_BOLD=""
fi
info() { printf '%s\n' "[信息] $*"; }
ok()   { printf '%s\n' "${C_GREEN}  ✓${C_RESET} $*"; }
warn() { printf '%s\n' "${C_YELLOW}  !${C_RESET} $*"; }
err()  { printf '%s\n' "${C_RED}[错误]${C_RESET} $*" >&2; }
hr()   { printf '%s\n' "========================================="; }

# 推断安装目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "${SCRIPT_DIR}/../apphub" ]; then
  INSTALL_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
  INSTALL_DIR="${DEFAULT_DIR}"
fi

hr
printf '%s\n' "${C_BOLD}AppHub 卸载程序${C_RESET}"
hr
printf '%s\n' "  安装目录：${INSTALL_DIR}"
printf '\n'
printf '%s\n' "${C_YELLOW}警告：卸载会停止 AppHub 服务并删除程序文件。${C_RESET}"
printf '%s\n' "${C_YELLOW}AppHub 不会删除你被管理的实际应用程序（如 /opt/apps/*）。${C_RESET}"
printf '\n'

read -r -p "  确认卸载？[y/N]: " CONFIRM || true
case "${CONFIRM}" in
  Y|y|yes|YES) ;;
  *) info "已取消卸载"; exit 0 ;;
esac

# 1. 停止并禁用服务
if command -v systemctl >/dev/null 2>&1; then
  if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 && ok "已停止服务"
  fi
  if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
    systemctl disable "${SERVICE_NAME}" >/dev/null 2>&1 && ok "已取消开机自启"
  fi
fi

# 2. 删除 service 文件与软链
rm -f "/etc/systemd/system/${SERVICE_NAME}"
rm -f "/etc/systemd/system/multi-user.target.wants/${SERVICE_NAME}"
ok "已删除 systemd 服务文件"
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload 2>/dev/null && ok "daemon-reload"
  systemctl reset-failed "${SERVICE_NAME}" 2>/dev/null || true
fi

# 3. 删除 sudoers
rm -f /etc/sudoers.d/apphub 2>/dev/null && ok "已删除 sudoers 配置" || true

# 4. 删除软链接
for link in /usr/local/bin/apphub /usr/bin/apphub; do
  if [ -L "${link}" ]; then rm -f "${link}" && ok "已删除软链接 ${link}"; fi
done

# 5. 数据处理
printf '\n'
printf '%s\n' "请选择数据处理方式："
printf '%s\n' "  1) 保留数据（推荐，保留 data/ config.yaml logs backups uploads）"
printf '%s\n' "  2) 删除全部数据（不可恢复）"
printf '%s\n' "  3) 备份后删除（备份到 ${INSTALL_DIR}-backup-<时间>.tar.gz）"
read -r -p "  请选择 [1]: " CHOICE || true
CHOICE="${CHOICE:-1}"

case "${CHOICE}" in
  2)
    rm -rf "${INSTALL_DIR}"
    ok "已删除 ${INSTALL_DIR}（含全部数据）"
    ;;
  3)
    BACKUP="/opt/apphub-backup-$(date +%Y%m%d-%H%M%S).tar.gz"
    if tar -czf "${BACKUP}" -C "$(dirname "${INSTALL_DIR}")" "$(basename "${INSTALL_DIR}")" 2>/dev/null; then
      ok "已备份到 ${BACKUP}"
      rm -rf "${INSTALL_DIR}"
      ok "已删除 ${INSTALL_DIR}"
    else
      err "备份失败，已中止删除，请手动处理"
      exit 1
    fi
    ;;
  *)
    rm -f "${INSTALL_DIR}/apphub"
    rm -rf "${INSTALL_DIR}/runtime"
    rm -rf "${INSTALL_DIR}/scripts"
    ok "已删除程序文件，保留数据与配置"
    printf '%s\n' "  数据目录：${INSTALL_DIR}/data"
    printf '%s\n' "  配置文件：${INSTALL_DIR}/config.yaml"
    printf '%s\n' "  备份目录：${INSTALL_DIR}/backups"
    ;;
esac

# 6. 可选删除系统用户
if id -u apphub >/dev/null 2>&1; then
  printf '\n'
  read -r -p "  是否删除系统用户 apphub？[y/N]: " DELUSER || true
  case "${DELUSER}" in
    Y|y|yes|YES) userdel apphub 2>/dev/null && ok "已删除用户 apphub" || warn "删除用户失败" ;;
  esac
fi

hr
printf '%s\n' "${C_GREEN}卸载完成${C_RESET}"
hr
