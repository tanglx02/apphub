#!/usr/bin/env bash
# 停止模拟应用
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_FILE="${DIR}/mock-app.pid"

if [ ! -f "${PID_FILE}" ]; then
  echo "mock-app 未运行（无 PID 文件）"
  exit 0
fi

PID="$(cat "${PID_FILE}")"
if kill -0 "${PID}" 2>/dev/null; then
  kill -TERM "${PID}" 2>/dev/null || true
  for _ in $(seq 1 20); do
    kill -0 "${PID}" 2>/dev/null || break
    sleep 0.2
  done
  if kill -0 "${PID}" 2>/dev/null; then
    kill -KILL "${PID}" 2>/dev/null || true
  fi
  echo "mock-app 已停止 pid=${PID}"
else
  echo "mock-app 进程不存在"
fi
rm -f "${PID_FILE}"
