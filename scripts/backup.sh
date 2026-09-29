#!/usr/bin/env bash
# =============================================================
# AppHub · 备份脚本
# 用法：sudo ./scripts/backup.sh [输出目录]
# 备份内容：config.yaml + SQLite 一致性快照 + 上传图标
# =============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG="${INSTALL_DIR}/config.yaml"
BIN="${INSTALL_DIR}/apphub"
DEST="${1:-}"

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  C_GREEN=$(tput setaf 2); C_YELLOW=$(tput setaf 3); C_RED=$(tput setaf 1); C_RESET=$(tput sgr0)
else
  C_GREEN=""; C_YELLOW=""; C_RED=""; C_RESET=""
fi
ok()   { printf '%s\n' "${C_GREEN}  ✓${C_RESET} $*"; }
warn() { printf '%s\n' "${C_YELLOW}  !${C_RESET} $*"; }
err()  { printf '%s\n' "${C_RED}[错误]${C_RESET} $*" >&2; }

printf '%s\n' "========================================="
printf '%s\n' "AppHub 备份"
printf '%s\n' "========================================="

# 默认备份目录（读配置，失败则用 ./backups）
BACKUP_DIR="${DEST}"
if [ -z "${BACKUP_DIR}" ] && [ -f "${CONFIG}" ]; then
  BACKUP_DIR="$(grep -A2 '^backup:' "${CONFIG}" | grep 'directory:' | head -n1 | sed -E 's/.*directory:[[:space:]]*//' | tr -d '"'"'" | tr -d '\r')"
fi
[ -z "${BACKUP_DIR}" ] && BACKUP_DIR="./backups"
case "${BACKUP_DIR}" in
  /*) ;;
  *) BACKUP_DIR="${INSTALL_DIR}/${BACKUP_DIR#./}" ;;
esac
mkdir -p "${BACKUP_DIR}"

TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
NAME="apphub-backup-${TIMESTAMP}.tar.gz"
OUT="${BACKUP_DIR}/${NAME}"

# 优先使用程序内置备份（SQLite VACUUM INTO，保证一致性）
if [ -x "${BIN}" ]; then
  printf '%s\n' "  正在导出数据库一致性快照..."
  if (cd "${INSTALL_DIR}" && "${BIN}" --config "${CONFIG}" --backup "${BACKUP_DIR}") ; then
    # 程序生成的备份文件名与时间戳一致
    if [ -f "${OUT}" ]; then
      ok "备份完成：${OUT} ($(du -h "${OUT}" | awk '{print $1}'))"
      exit 0
    fi
    LATEST="$(ls -t "${BACKUP_DIR}"/apphub-backup-*.tar.gz 2>/dev/null | head -n1)"
    if [ -n "${LATEST}" ]; then
      ok "备份完成：${LATEST} ($(du -h "${LATEST}" | awk '{print $1}'))"
      exit 0
    fi
  else
    warn "程序内置备份失败，回退为文件复制方式"
  fi
fi

# 回退：直接打包（建议先停止服务以保证一致性）
warn "使用文件复制方式备份（数据库可能处于写入状态）"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

cp -f "${CONFIG}" "${TMP_DIR}/config.yaml" 2>/dev/null || warn "config.yaml 不存在"
if [ -f "${INSTALL_DIR}/data/apphub.db" ]; then
  cp -f "${INSTALL_DIR}/data/apphub.db" "${TMP_DIR}/apphub.db"
fi
mkdir -p "${TMP_DIR}/uploads"
[ -d "${INSTALL_DIR}/uploads" ] && cp -f "${INSTALL_DIR}"/uploads/* "${TMP_DIR}/uploads/" 2>/dev/null || true

cat > "${TMP_DIR}/meta.json" <<EOF
{
  "name": "${NAME}",
  "created_at": "$(date -Iseconds)",
  "root": "${INSTALL_DIR}",
  "method": "file-copy"
}
EOF

tar -czf "${OUT}" -C "${TMP_DIR}" .
ok "备份完成：${OUT} ($(du -h "${OUT}" | awk '{print $1}'))"
