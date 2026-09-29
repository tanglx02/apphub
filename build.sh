#!/usr/bin/env bash
# =============================================================
# AppHub · 构建脚本
# 产出：
#   dist/apphub-linux-arm64   （Armbian / Debian 12 ARM64）
#   dist/apphub-linux-amd64   （x86_64 Linux）
#   dist/apphub-<arch>.tar.gz （含二进制 + 脚本 + 文档的完整包）
#
# 用法：
#   ./build.sh              # 构建全部架构
#   ./build.sh arm64        # 只构建 arm64
#   ./build.sh --skip-web   # 跳过前端构建（使用已有 web/dist）
# =============================================================
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
ROOT="$(pwd)"

VERSION="${APPHUB_VERSION:-1.0.0}"
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w \
  -X github.com/tanglx02/apphub/internal/buildinfo.Version=${VERSION} \
  -X github.com/tanglx02/apphub/internal/buildinfo.GitCommit=${GIT_COMMIT} \
  -X github.com/tanglx02/apphub/internal/buildinfo.BuildTime=${BUILD_TIME}"

SKIP_WEB=0
TARGETS=()
for arg in "$@"; do
  case "${arg}" in
    --skip-web) SKIP_WEB=1 ;;
    arm64|amd64|armv7) TARGETS+=("${arg}") ;;
    *) echo "未知参数：${arg}"; exit 1 ;;
  esac
done
[ "${#TARGETS[@]}" -eq 0 ] && TARGETS=(arm64 amd64)

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  C_GREEN=$(tput setaf 2); C_YELLOW=$(tput setaf 3); C_BLUE=$(tput setaf 4); C_RESET=$(tput sgr0)
else
  C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_RESET=""
fi
step() { printf '%s\n' "${C_BLUE}==>${C_RESET} $*"; }
ok()   { printf '%s\n' "${C_GREEN}  ✓${C_RESET} $*"; }
warn() { printf '%s\n' "${C_YELLOW}  !${C_RESET} $*"; }

# ---------- 1. 前端构建 ----------
if [ "${SKIP_WEB}" = "0" ]; then
  step "构建前端（Vue 3 + Vite + Tailwind）"
  if command -v npm >/dev/null 2>&1; then
    if [ ! -d "${ROOT}/web/node_modules" ]; then
      (cd "${ROOT}/web" && npm install --no-audit --no-fund)
    fi
    (cd "${ROOT}/web" && npm run build:fast)
    ok "前端构建完成：web/dist"
  else
    if [ -f "${ROOT}/web/dist/index.html" ]; then
      warn "未检测到 npm，使用已有的 web/dist"
    else
      echo "错误：未找到 npm 且 web/dist 不存在，无法构建前端"
      exit 1
    fi
  fi
else
  warn "跳过前端构建"
  [ -f "${ROOT}/web/dist/index.html" ] || { echo "错误：web/dist 不存在"; exit 1; }
fi

# ---------- 2. 后端交叉编译 ----------
mkdir -p "${ROOT}/dist"
step "编译后端（Go，CGO_ENABLED=0 纯静态）"

for arch in "${TARGETS[@]}"; do
  OUT="dist/apphub-linux-${arch}"
  step "  -> linux/${arch}"
  # 注意：-o 必须使用相对路径，Windows 下的 go 无法识别 MSYS 虚拟路径
  CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" go build -trimpath -ldflags "${LDFLAGS}" -o "${OUT}" ./cmd/apphub
  ok "已生成 ${ROOT}/dist/apphub-linux-${arch} ($(du -h "${OUT}" | awk '{print $1}'))"
done

# ---------- 3. 打包 ----------
step "打包发布压缩包"
for arch in "${TARGETS[@]}"; do
  PKG_DIR="${ROOT}/dist/apphub-${VERSION}-linux-${arch}"
  rm -rf "${PKG_DIR}"
  mkdir -p "${PKG_DIR}"
  cp -f "${ROOT}/dist/apphub-linux-${arch}" "${PKG_DIR}/apphub"
  chmod +x "${PKG_DIR}/apphub"
  mkdir -p "${PKG_DIR}/scripts" "${PKG_DIR}/deploy" "${PKG_DIR}/examples" "${PKG_DIR}/data" "${PKG_DIR}/logs" "${PKG_DIR}/backups" "${PKG_DIR}/runtime" "${PKG_DIR}/uploads"
  cp -f "${ROOT}"/scripts/*.sh "${PKG_DIR}/scripts/" 2>/dev/null || true
  cp -f "${ROOT}"/deploy/* "${PKG_DIR}/deploy/" 2>/dev/null || true
  cp -f "${ROOT}"/examples/* "${PKG_DIR}/examples/" 2>/dev/null || true
  [ -f "${ROOT}/README.md" ] && cp -f "${ROOT}/README.md" "${PKG_DIR}/README.md"
  [ -f "${ROOT}/PROJECT_SPEC.md" ] && cp -f "${ROOT}/PROJECT_SPEC.md" "${PKG_DIR}/PROJECT_SPEC.md"
  [ -f "${ROOT}/config.example.yaml" ] && cp -f "${ROOT}/config.example.yaml" "${PKG_DIR}/config.example.yaml"
  chmod +x "${PKG_DIR}"/scripts/*.sh 2>/dev/null || true
  (cd "${ROOT}/dist" && tar -czf "apphub-${VERSION}-linux-${arch}.tar.gz" "apphub-${VERSION}-linux-${arch}")
  ok "已打包 dist/apphub-${VERSION}-linux-${arch}.tar.gz"
done

step "构建完成"
printf '%s\n' "  版本：${VERSION}"
printf '%s\n' "  提交：${GIT_COMMIT}"
printf '%s\n' "  产物：${ROOT}/dist"
ls -la "${ROOT}/dist" | sed 's/^/  /'
