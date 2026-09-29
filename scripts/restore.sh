#!/usr/bin/env bash
# =============================================================
# AppHub · 恢复脚本
# 用法：sudo ./scripts/restore.sh [备份文件名或路径]
# 流程：停止服务 -> 自动备份当前状态 -> 恢复 -> 启动服务
# =============================================================
set -euo pipefail

SERVICE_NAME="apphub.service"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG="${INSTALL_DIR}/config.yaml"
BIN="${INSTALL_DIR}/apphub"
ARG="${1:-}"

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  C_GREEN=$(tput setaf 2); C_YELLOW=$(tput setaf 3); C_RED=$(tput setaf 1); C_RESET=$(tput sgr0)
else
  C_GREEN=""; C_YELLOW=""; C_RED=""; C_RESET=""
fi
ok()   { printf '%s\n' "${C_GREEN}  ✓${C_RESET} $*"; }
warn() { printf '%s\n' "${C_YELLOW}  !${C_RESET} $*"; }
err()  { printf '%s\n' "${C_RED}[错误]${C_RESET} $*" >&2; }

BACKUP_DIR="${INSTALL_DIR}/backups"
if [ -f "${CONFIG}" ]; then
  DIR_CFG="$(grep -A2 '^backup:' "${CONFIG}" | grep 'directory:' | head -n1 | sed -E 's/.*directory:[[:space:]]*//' | tr -d '"'"'" | tr -d '\r')"
  [ -n "${DIR_CFG}" ] && BACKUP_DIR="${DIR_CFG}"
fi
case "${BACKUP_DIR}" in
  /*) ;;
  *) BACKUP_DIR="${INSTALL_DIR}/${BACKUP_DIR#./}" ;;
esac

printf '%s\n' "========================================="
printf '%s\n' "AppHub 恢复"
printf '%s\n' "========================================="

# 选择备份文件
TARGET="${ARG}"
if [ -z "${TARGET}" ]; then
  mapfile -t FILES < <(ls -1t "${BACKUP_DIR}"/apphub-backup-*.tar.gz 2>/dev/null || true)
  if [ "${#FILES[@]}" -eq 0 ]; then
    err "未找到任何备份文件（目录：${BACKUP_DIR}）"
    exit 1
  fi
  printf '\n%s\n' "可用备份："
  i=1
  for f in "${FILES[@]}"; do
    printf '%s\n' "  ${i}) $(basename "${f}")  ($(du -h "${f}" | awk '{print $1}'))"
    i=$((i + 1))
  done
  printf '%s\n' "  0) 取消"
  read -r -p "请选择: " PICK || PICK="0"
  if [ "${PICK}" = "0" ] || [ -z "${PICK}" ]; then info_msg "已取消"; exit 0; fi
  TARGET="${FILES[$((PICK - 1))]}"
fi

info_msg() { printf '%s\n' "   $*"; }

case "${TARGET}" in
  /*) ;;
  *) TARGET="${BACKUP_DIR}/${TARGET}" ;;
esac

if [ ! -f "${TARGET}" ]; then
  err "备份文件不存在：${TARGET}"
  exit 1
fi
printf '%s\n' "  将恢复：${TARGET}"
printf '\n'
read -r -p "  确认恢复？恢复前会自动备份当前状态。[y/N]: " CONFIRM || CONFIRM="n"
case "${CONFIRM}" in
  Y|y|yes|YES) ;;
  *) printf '%s\n' "已取消"; exit 0 ;;
esac

# 1. 停止服务
if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
  systemctl stop "${SERVICE_NAME}" && ok "已停止服务"
fi

# 2. 恢复前自动备份
if [ -x "${BIN}" ]; then
  PRE_DIR="${BACKUP_DIR}/pre-restore-$(date +%Y%m%d-%H%M%S)"
  mkdir -p "${PRE_DIR}"
  (cd "${INSTALL_DIR}" && "${BIN}" --config "${CONFIG}" --backup "${PRE_DIR}") >/dev/null 2>&1 \
    && ok "已在恢复前备份到 ${PRE_DIR}" || warn "恢复前备份失败（继续恢复）"
fi

# 3. 执行恢复
if [ -x "${BIN}" ]; then
  (cd "${INSTALL_DIR}" && "${BIN}" --config "${CONFIG}" --restore "${TARGET}") && ok "已恢复数据"
else
  err "未找到程序文件，无法恢复"
  exit 1
fi

# 4. 启动服务
if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files 2>/dev/null | grep -q "${SERVICE_NAME}"; then
  systemctl daemon-reload
  systemctl start "${SERVICE_NAME}" && ok "已启动服务"
  sleep 2
  systemctl is-active --quiet "${SERVICE_NAME}" && ok "服务运行正常" || warn "服务状态异常，请检查日志"
fi

printf '\n%s\n' "恢复完成。请重新登录 AppHub 确认数据。"
