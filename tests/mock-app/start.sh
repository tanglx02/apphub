#!/usr/bin/env bash
# 启动模拟应用（供 AppHub 集成测试使用）
# 用法：./start.sh [端口]
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT="${1:-${MOCK_PORT:-19090}}"
PID_FILE="${DIR}/mock-app.pid"
LOG_FILE="${DIR}/mock-app.log"

cd "${DIR}"

# 必要时编译
if [ ! -x "${DIR}/mock-app" ]; then
  if command -v go >/dev/null 2>&1; then
    (cd "${DIR}" && go build -o mock-app main.go)
  fi
fi
if [ ! -x "${DIR}/mock-app" ]; then
  echo "无法构建 mock-app，请确保已安装 Go" >&2
  exit 1
fi

# 已在运行则直接退出
if [ -f "${PID_FILE}" ] && kill -0 "$(cat "${PID_FILE}")" 2>/dev/null; then
  echo "mock-app 已在运行 (pid=$(cat "${PID_FILE}"))"
  exit 0
fi

MOCK_PORT="${PORT}" nohup "${DIR}/mock-app" >>"${LOG_FILE}" 2>&1 &
PID=$!
echo "${PID}" > "${PID_FILE}"
echo "mock-app 已启动 pid=${PID} port=${PORT}"
