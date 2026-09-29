#!/usr/bin/env bash
# =============================================================
# AppHub · 管理工具（交互式菜单，不依赖第三方 CLI）
# 用法：sudo ./scripts/manage.sh
# =============================================================
set -uo pipefail

SERVICE_NAME="apphub.service"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG="${INSTALL_DIR}/config.yaml"
BIN="${INSTALL_DIR}/apphub"

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  C_RESET=$(tput sgr0); C_GREEN=$(tput setaf 2); C_YELLOW=$(tput setaf 3)
  C_RED=$(tput setaf 1); C_BOLD=$(tput bold); C_DIM=$(tput dim)
else
  C_RESET=""; C_GREEN=""; C_YELLOW=""; C_RED=""; C_BOLD=""; C_DIM=""
fi

hr() { printf '%s\n' "================================"; }
title() { printf '\n%s\n%s\n' "================================" "  $*" ; printf '%s\n' "================================"; }
ok() { printf '%s\n' "${C_GREEN}✓${C_RESET} $*"; }
warn() { printf '%s\n' "${C_YELLOW}!${C_RESET} $*"; }
err() { printf '%s\n' "${C_RED}✗${C_RESET} $*" >&2; }
info() { printf '%s\n' "  $*"; }

require_root() {
  if [ "$(id -u)" != "0" ] && command -v systemctl >/dev/null 2>&1; then
    warn "部分操作需要 root 权限，建议使用 sudo 运行本脚本"
  fi
}

have_systemd() { command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; }

read_port() {
  if [ -f "${CONFIG}" ]; then
    grep -E '^\s*port:' "${CONFIG}" | head -n1 | sed 's/[^0-9]//g'
  fi
}

show_status() {
  printf '\n'
  if have_systemd && systemctl list-unit-files 2>/dev/null | grep -q "${SERVICE_NAME}"; then
    systemctl status "${SERVICE_NAME}" --no-pager -n 0 2>/dev/null | head -n 8 || true
  else
    if [ -f "${BIN}" ]; then info "程序文件：${BIN}"; else err "未找到程序文件"; fi
    if pgrep -f "${INSTALL_DIR}/apphub" >/dev/null 2>&1; then ok "进程运行中"; else warn "进程未运行"; fi
  fi
  local port; port="$(read_port)"; [ -n "${port}" ] && info "监听端口：${port}"
  printf '\n'
}

start_app() {
  if have_systemd && systemctl list-unit-files 2>/dev/null | grep -q "${SERVICE_NAME}"; then
    systemctl start "${SERVICE_NAME}" && ok "已启动服务"
  else
    (cd "${INSTALL_DIR}" && nohup "${BIN}" --config "${CONFIG}" >>"${INSTALL_DIR}/logs/apphub.log" 2>&1 &) 
    sleep 1; ok "已后台启动"
  fi
}

stop_app() {
  if have_systemd; then systemctl stop "${SERVICE_NAME}" 2>/dev/null && ok "已停止服务" && return; fi
  pkill -f "${INSTALL_DIR}/apphub" 2>/dev/null && ok "已停止进程" || warn "未发现运行中的进程"
}

restart_app() {
  if have_systemd && systemctl list-unit-files 2>/dev/null | grep -q "${SERVICE_NAME}"; then
    systemctl restart "${SERVICE_NAME}" && ok "已重启服务"
  else
    stop_app; sleep 1; start_app
  fi
}

show_logs() {
  if have_systemd; then
    journalctl -u "${SERVICE_NAME}" -n 100 --no-pager 2>/dev/null || tail -n 100 "${INSTALL_DIR}/logs/apphub.log"
  else
    tail -n 100 "${INSTALL_DIR}/logs/apphub.log" 2>/dev/null || warn "暂无日志"
  fi
}

enable_service() {
  if ! have_systemd; then err "当前系统不支持 systemd"; return; fi
  systemctl enable "${SERVICE_NAME}" && ok "已设置开机自动启动"
  systemctl start "${SERVICE_NAME}" 2>/dev/null && ok "已启动服务"
}

disable_service() {
  if ! have_systemd; then err "当前系统不支持 systemd"; return; fi
  systemctl disable "${SERVICE_NAME}" 2>/dev/null && ok "已取消开机自动启动"
  systemctl stop "${SERVICE_NAME}" 2>/dev/null && ok "已停止服务"
}

do_install() { bash "${SCRIPT_DIR}/install.sh"; }
do_uninstall() { bash "${SCRIPT_DIR}/uninstall.sh"; }

reset_password() {
  if [ ! -f "${BIN}" ]; then err "未找到程序文件"; return; fi
  warn "重置密码会使该用户的全部会话失效"
  (cd "${INSTALL_DIR}" && "${BIN}" --config "${CONFIG}" --reset-admin-password)
}

do_backup() { bash "${SCRIPT_DIR}/backup.sh"; }
do_restore() { bash "${SCRIPT_DIR}/restore.sh"; }

show_config() {
  printf '\n'
  info "项目目录：${INSTALL_DIR}"
  info "配置文件：${CONFIG}"
  if [ -f "${CONFIG}" ]; then
    echo "--------------------------------"
    cat "${CONFIG}"
    echo "--------------------------------"
  else
    warn "配置文件不存在"
  fi
  if [ -f "${INSTALL_DIR}/data/apphub.db" ]; then
    info "数据库大小：$(du -h "${INSTALL_DIR}/data/apphub.db" | awk '{print $1}')"
  fi
  printf '\n'
}

menu() {
  clear 2>/dev/null || true
  printf '%s\n' "================================"
  printf '%s\n' "AppHub 管理工具"
  printf '%s\n' "================================"
  printf '%s\n' " 1. 启动 AppHub"
  printf '%s\n' " 2. 停止 AppHub"
  printf '%s\n' " 3. 重启 AppHub"
  printf '%s\n' " 4. 查看状态"
  printf '%s\n' " 5. 查看日志"
  printf '%s\n' " 6. 启用 systemd"
  printf '%s\n' " 7. 禁用 systemd"
  printf '%s\n' " 8. 安装"
  printf '%s\n' " 9. 卸载"
  printf '%s\n' "10. 重置管理员密码"
  printf '%s\n' "11. 备份"
  printf '%s\n' "12. 恢复"
  printf '%s\n' "13. 查看配置"
  printf '%s\n' "14. 退出"
  printf '%s\n' "================================"
}

main() {
  require_root
  while true; do
    menu
    read -r -p "请选择 [1-14]: " CHOICE || CHOICE="14"
    case "${CHOICE}" in
      1) title "启动 AppHub"; start_app; read -r -p "按回车继续..." _ || true ;;
      2) title "停止 AppHub"; stop_app; read -r -p "按回车继续..." _ || true ;;
      3) title "重启 AppHub"; restart_app; read -r -p "按回车继续..." _ || true ;;
      4) title "服务状态"; show_status; read -r -p "按回车继续..." _ || true ;;
      5) title "最近日志"; show_logs; read -r -p "按回车继续..." _ || true ;;
      6) title "启用 systemd"; enable_service; read -r -p "按回车继续..." _ || true ;;
      7) title "禁用 systemd"; disable_service; read -r -p "按回车继续..." _ || true ;;
      8) title "安装"; do_install; read -r -p "按回车继续..." _ || true ;;
      9) title "卸载"; do_uninstall; read -r -p "按回车继续..." _ || true ;;
      10) title "重置管理员密码"; reset_password; read -r -p "按回车继续..." _ || true ;;
      11) title "备份"; do_backup; read -r -p "按回车继续..." _ || true ;;
      12) title "恢复"; do_restore; read -r -p "按回车继续..." _ || true ;;
      13) title "查看配置"; show_config; read -r -p "按回车继续..." _ || true ;;
      14|q|Q) printf '%s\n' "已退出"; exit 0 ;;
      *) warn "无效选项"; sleep 1 ;;
    esac
  done
}

# 支持非交互调用：./manage.sh start|stop|restart|status|logs
case "${1:-}" in
  start)   start_app ;;
  stop)    stop_app ;;
  restart) restart_app ;;
  status)  show_status ;;
  logs)    show_logs ;;
  "")      main ;;
  *)       echo "用法: $0 [start|stop|restart|status|logs]"; exit 1 ;;
esac
