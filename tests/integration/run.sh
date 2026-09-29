#!/usr/bin/env bash
# =============================================================
# AppHub 集成测试
# 覆盖：安装目录初始化 -> 启动 -> 健康检查 -> 初始化管理员
#       -> 登录 -> 应用 CRUD -> 启动/检测/停止 -> 备份 -> 恢复 -> 停止
#
# 用法：./tests/integration/run.sh
# 依赖：curl、go（用于编译 mock-app）
# =============================================================
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# 服务端执行 start_command 时使用的是原生路径：Windows 下需要 C:/... 形式
ROOT_WIN="$(cd "${ROOT}" && pwd -W 2>/dev/null || printf '%s' "${ROOT}")"
# POSIX 环境使用 shell 脚本（同时验证 shell 模式）；
# Windows 下没有 /bin/sh，改为直接调用编译产物（非 shell 模式）
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    # Windows 的 exec 要求可执行文件带 .exe 扩展名
    MOCK_BIN_OUT="mock-app.exe"
    MOCK_START="${ROOT_WIN}/tests/mock-app/mock-app.exe"
    MOCK_STOP=""
    MOCK_SHELL=false
    ;;
  *)
    MOCK_BIN_OUT="mock-app"
    MOCK_START="${ROOT_WIN}/tests/mock-app/start.sh"
    MOCK_STOP="${ROOT_WIN}/tests/mock-app/stop.sh"
    MOCK_SHELL=true
    ;;
esac
WORK="${ROOT}/runtime/integration"
PORT="${APPHUB_TEST_PORT:-18777}"
BASE="http://127.0.0.1:${PORT}"
PASS=0
FAIL=0

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  G=$(tput setaf 2); R=$(tput setaf 1); Y=$(tput setaf 3); N=$(tput sgr0)
else
  G=""; R=""; Y=""; N=""
fi

pass() { printf '%s\n' "${G}  ✓${N} $*"; PASS=$((PASS + 1)); }
fail() { printf '%s\n' "${R}  ✗${N} $*"; FAIL=$((FAIL + 1)); }
info() { printf '%s\n' "  · $*"; }
section() { printf '\n%s\n' "${Y}== $*${N}"; }

cleanup() {
  if [ -n "${SERVER_PID:-}" ] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill -TERM "${SERVER_PID}" 2>/dev/null || true
    sleep 1
  fi
  "${MOCK_STOP}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

section "准备测试环境"
rm -rf "${WORK}"
mkdir -p "${WORK}"
BIN_REL="./runtime/integration/apphub"
BIN="./apphub"
(cd "${ROOT}" && go build -o "${BIN_REL}" ./cmd/apphub) || { fail "编译 apphub 失败"; exit 1; }
pass "编译 apphub"
(cd "${ROOT}/tests/mock-app" && go build -o "${MOCK_BIN_OUT}" main.go) || { fail "编译 mock-app 失败"; exit 1; }
pass "编译 mock-app"

# 之后所有文件操作都在工作目录内进行，使用相对路径
cd "${WORK}"
chmod +x "${BIN}" 2>/dev/null || true
COOKIE="cookie.txt"

section "配置与 CLI"
cat > "config.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
  https: false
  site_name: 集成测试
database:
  path: ./data/apphub.db
logging:
  level: info
  retention_days: 7
status:
  interval: 3
  timeout: 3
EOF
"${BIN}" --config ./config.yaml --migrate >/dev/null 2>&1 && pass "--migrate 可用" || fail "--migrate 失败"
"${BIN}" --config ./config.yaml --check >/dev/null 2>&1 && pass "--check 可用" || fail "--check 失败"
"${BIN}" --version | grep -q "AppHub" && pass "--version 可用" || fail "--version 失败"

section "启动服务"
exec "${BIN}" --config ./config.yaml >"server.log" 2>&1 &
SERVER_PID=$!
for i in $(seq 1 30); do
  if curl -fsS "${BASE}/health" >/dev/null 2>&1; then break; fi
  sleep 0.5
done
curl -fsS "${BASE}/health" >/dev/null 2>&1 && pass "/health 返回 200" || { fail "/health 失败"; exit 1; }
curl -fsS "${BASE}/ready" | grep -q '"ready"' && pass "/ready 返回 200" || fail "/ready 失败"
curl -fsS "${BASE}/" -o curl-out.tmp && pass "SPA 首页可访问" || fail "SPA 首页不可访问"

section "认证"
curl -s -c "${COOKIE}" -X POST "${BASE}/api/v1/auth/setup" -H 'Content-Type: application/json' \
  -d '{"username":"tester","password":"Test@2026","site_name":"集成测试"}' | grep -q '"success":true' \
  && pass "初始化管理员" || fail "初始化管理员失败"
curl -s -c "${COOKIE}" -X POST "${BASE}/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"tester","password":"Test@2026"}' | grep -q 'csrf_token' \
  && pass "登录成功并返回 CSRF" || { fail "登录失败"; exit 1; }

CSRF="$(curl -s -b "${COOKIE}" "${BASE}/api/v1/auth/me" | grep -o '"csrf_token":"[^"]*"' | head -n1 | cut -d'"' -f4)"
[ -n "${CSRF}" ] && pass "获取 CSRF Token" || { fail "CSRF Token 为空"; exit 1; }

# 未携带 CSRF 的写请求必须被拒绝
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps" -H 'Content-Type: application/json' \
  -d '{"name":"x","app_type":"command","start_command":"/bin/true"}' | grep -q 'CSRF_FAILED' \
  && pass "CSRF 防护生效" || fail "CSRF 防护未生效"

# 未登录访问必须被拒绝
curl -s -o curl-out.tmp -w '%{http_code}' "${BASE}/api/v1/apps" | grep -q '401' \
  && pass "未登录访问被拒绝" || fail "未登录仍可访问"

section "应用 CRUD"
# 通过文件提交 JSON，避免不同平台 curl 对多行 -d 参数的处理差异
cat > create-app.json.in <<EOF
{
  "name": "集成测试应用",
  "description": "mock-app 模拟应用",
  "icon": "test",
  "app_type": "command",
  "enabled": true,
  "shell_mode": ${MOCK_SHELL},
  "start_command": "${MOCK_START}",
  "stop_command": "${MOCK_STOP}",
  "status_type": "http",
  "status_target": "http://127.0.0.1:19090/health",
  "internal_url": "http://127.0.0.1:19090",
  "external_url": "https://mock.example.com"
}
EOF
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps" -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: ${CSRF}" -d @create-app.json.in -o create-app.json
APP_ID="$(grep -o '"id":[0-9]*' create-app.json | head -n1 | cut -d: -f2)"
if [ -n "${APP_ID}" ]; then
  pass "添加应用 (id=${APP_ID})"
else
  fail "添加应用失败"
  info "服务端响应: $(head -c 300 create-app.json 2>/dev/null)"
  exit 1
fi

curl -s -b "${COOKIE}" "${BASE}/api/v1/apps" | grep -q '集成测试应用' && pass "应用列表返回" || fail "应用列表失败"

# 非法配置必须被拒绝
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps" -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: ${CSRF}" -d '{"name":"非法","app_type":"systemd","systemd_unit":"rm -rf /"}' \
  | grep -q 'systemd 服务名非法' && pass "非法 systemd 服务名被拒绝" || fail "非法服务名未被拦截"

section "启停与状态检测"
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps/${APP_ID}/start" -H "X-CSRF-Token: ${CSRF}" \
  | grep -q '"success":true' && pass "启动应用" || fail "启动应用失败"

ONLINE=0
for i in $(seq 1 30); do
  sleep 1
  ST="$(curl -s -b "${COOKIE}" "${BASE}/api/v1/apps/${APP_ID}/status" | sed -n 's/.*"status":"\([A-Z]*\)".*/\1/p')"
  if [ "${ST}" = "ONLINE" ]; then ONLINE=1; break; fi
done
[ "${ONLINE}" = "1" ] && pass "HTTP 检测识别为 ONLINE" || fail "状态检测失败（实际 ${ST:-unknown}）"

curl -fsS "http://127.0.0.1:19090/health" >/dev/null 2>&1 && pass "模拟应用实际可访问" || fail "模拟应用不可访问"

curl -s -b "${COOKIE}" "${BASE}/api/v1/apps/${APP_ID}/logs?lines=20" | grep -q '"success":true' \
  && pass "读取应用日志" || fail "读取日志失败"

curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps/${APP_ID}/stop" -H "X-CSRF-Token: ${CSRF}" \
  | grep -q '"success":true' && pass "停止应用" || fail "停止应用失败"

OFFLINE=0
for i in $(seq 1 20); do
  sleep 1
  ST="$(curl -s -b "${COOKIE}" "${BASE}/api/v1/apps/${APP_ID}/status" | sed -n 's/.*"status":"\([A-Z]*\)".*/\1/p')"
  if [ "${ST}" = "OFFLINE" ]; then OFFLINE=1; break; fi
done
[ "${OFFLINE}" = "1" ] && pass "停止后识别为 OFFLINE" || fail "停止后状态未更新（实际 ${ST:-unknown}）"
! curl -fsS "http://127.0.0.1:19090/health" >/dev/null 2>&1 && pass "模拟应用已真正退出" || fail "模拟应用仍在运行"

section "审计日志"
curl -s -b "${COOKIE}" "${BASE}/api/v1/audit?limit=20" | grep -q '启动应用' \
  && pass "审计记录启停操作" || fail "审计日志缺失"

section "前台只读导航（Public API）"
curl -fsS "${BASE}/api/v1/public/config" | grep -q '"enabled":true' \
  && pass "公开配置可匿名访问" || fail "公开配置不可访问"
curl -fsS "${BASE}/api/v1/public/overview" | grep -q '"summary"' \
  && pass "公开概览可匿名访问" || fail "公开概览不可访问"
curl -fsS "${BASE}/api/v1/public/apps" | grep -q '集成测试应用' \
  && pass "前台显示已开启前台展示的应用" || fail "前台应用列表缺失"
curl -s "${BASE}/api/v1/public/apps" | grep -qE 'systemd_unit|start_command|work_dir|environment' \
  && fail "公开接口泄露管理字段" || pass "公开接口未泄露管理字段"

# 添加一个"前台隐藏"应用，验证 public_visible=false 不出现在前台
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps" -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: ${CSRF}" -d '{"name":"隐藏应用","app_type":"command","start_command":"/bin/true","status_type":"none","enabled":true,"public_visible":false}' \
  > hidden-app.json
HIDDEN_ID="$(grep -o '"id":[0-9]*' hidden-app.json | head -n1 | cut -d: -f2)"
[ -n "${HIDDEN_ID}" ] && pass "添加前台隐藏应用 (id=${HIDDEN_ID})" || fail "添加隐藏应用失败"
curl -s "${BASE}/api/v1/public/apps" | grep -q '隐藏应用' \
  && fail "隐藏应用仍出现在前台" || pass "前台隐藏应用不对外展示"

# 后台编辑应用后前台同步（改为隐藏 -> 改回显示）
curl -s -b "${COOKIE}" -X PUT "${BASE}/api/v1/apps/${HIDDEN_ID}" -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: ${CSRF}" -d '{"name":"隐藏应用","app_type":"command","start_command":"/bin/true","status_type":"none","enabled":true,"public_visible":true}' \
  | grep -q '"success":true' && pass "后台编辑应用" || fail "后台编辑失败"
curl -s "${BASE}/api/v1/public/apps" | grep -q '隐藏应用' \
  && pass "后台修改后前台同步" || fail "前台未同步后台修改"
curl -s -b "${COOKIE}" -X DELETE "${BASE}/api/v1/apps/${HIDDEN_ID}" -H "X-CSRF-Token: ${CSRF}" >/dev/null

section "权限隔离（匿名禁止管理操作）"
for m in "POST /api/v1/apps/${APP_ID}/start" "POST /api/v1/apps/${APP_ID}/stop" "POST /api/v1/apps/${APP_ID}/restart"; do
  CODE="$(curl -s -o curl-out.tmp -w '%{http_code}' -X "${m%% *}" "${BASE}${m#* }" -H 'Content-Type: application/json' -d '{}')"
  [ "${CODE}" = "401" ] || [ "${CODE}" = "403" ] \
    && pass "匿名 ${m%% *} ${m#* } -> ${CODE}" || fail "匿名 ${m} 返回 ${CODE}，应拒绝"
done
for m in "DELETE /api/v1/apps/${APP_ID}" "PUT /api/v1/apps/${APP_ID}" "POST /api/v1/apps" "GET /api/v1/settings" "GET /api/v1/audit" "POST /api/v1/backups"; do
  CODE="$(curl -s -o curl-out.tmp -w '%{http_code}' -X "${m%% *}" "${BASE}${m#* }" -H 'Content-Type: application/json' -d '{}')"
  [ "${CODE}" = "401" ] || [ "${CODE}" = "403" ] \
    && pass "匿名 ${m%% *} ${m#* } -> ${CODE}" || fail "匿名 ${m} 返回 ${CODE}，应拒绝"
done

section "双类型应用（LOCAL / EXTERNAL）"
# 添加非本地应用：GitHub（仅导航 + 在线检测）
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps" -H 'Content-Type: application/json'   -H "X-CSRF-Token: ${CSRF}" -d '{"name":"GitHub","type":"EXTERNAL","external_url":"https://github.com","status_type":"http","enabled":true,"public_visible":true}'   > gh-app.json
GH_ID="$(grep -o '"id":[0-9]*' gh-app.json | head -n1 | cut -d: -f2)"
[ -n "${GH_ID}" ] && pass "添加非本地应用 GitHub (id=${GH_ID})" || fail "添加 GitHub 失败"

# 前台 DTO 应带 type=EXTERNAL 与 endpoints，且不含管理字段
curl -s "${BASE}/api/v1/public/apps" | grep -q '"type":"EXTERNAL"'   && pass "前台返回 EXTERNAL 类型" || fail "前台缺少 EXTERNAL 类型"
curl -s "${BASE}/api/v1/public/apps" | grep -q '"endpoints"'   && pass "前台返回访问入口 endpoints" || fail "前台缺少 endpoints"

# 非本地应用：管理操作必须被后端拒绝（400）
for act in start stop restart; do
  CODE="$(curl -s -b "${COOKIE}" -o curl-out.tmp -w '%{http_code}' -X POST "${BASE}/api/v1/apps/${GH_ID}/${act}"     -H 'Content-Type: application/json' -H "X-CSRF-Token: ${CSRF}" -d '{}')"
  BODY="$(cat curl-out.tmp)"
  if [ "${CODE}" = "400" ] || [ "${CODE}" = "403" ]; then
    echo "${BODY}" | grep -q '非本地应用不支持服务控制'       && pass "EXTERNAL ${act} 被拒绝（${CODE} + 提示语）" || fail "EXTERNAL ${act} 拒绝但提示语缺失"
  else
    fail "EXTERNAL ${act} 返回 ${CODE}，应为 400/403"
  fi
done

# 非本地应用在线检测：指向 AppHub 自身健康检查端点，应识别为 ONLINE
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/apps" -H 'Content-Type: application/json'   -H "X-CSRF-Token: ${CSRF}" -d '{"name":"外部自检","type":"EXTERNAL","external_url":"'"${BASE}"'/health","status_type":"http","enabled":true,"public_visible":false}'   > ext-self.json
EXT_ID="$(grep -o '"id":[0-9]*' ext-self.json | head -n1 | cut -d: -f2)"
ONLINE=0
for i in $(seq 1 20); do
  sleep 1
  ST="$(curl -s -b "${COOKIE}" "${BASE}/api/v1/apps/${EXT_ID}/status" | grep -o '"status":"[A-Z]*"' | head -n1 | cut -d'"' -f4)"
  if [ "${ST}" = "ONLINE" ]; then ONLINE=1; break; fi
done
[ "${ONLINE}" = "1" ] && pass "EXTERNAL HTTP 健康检查识别为 ONLINE" || fail "EXTERNAL 健康检查失败（实际 ${ST:-unknown}）"

# 清理临时应用
curl -s -b "${COOKIE}" -X DELETE "${BASE}/api/v1/apps/${GH_ID}" -H "X-CSRF-Token: ${CSRF}" | grep -q '"success":true' && pass "清理 GitHub 测试应用" || fail "清理 GitHub 失败"
curl -s -b "${COOKIE}" -X DELETE "${BASE}/api/v1/apps/${EXT_ID}" -H "X-CSRF-Token: ${CSRF}" >/dev/null

section "备份与恢复"
curl -s -b "${COOKIE}" -X POST "${BASE}/api/v1/backups" -H 'Content-Type: application/json' \
  -H "X-CSRF-Token: ${CSRF}" -d '{"note":"集成测试"}' > backup.json
BACKUP_NAME="$(grep -o '"name":"[^"]*"' backup.json | head -n1 | cut -d'"' -f4)"
[ -n "${BACKUP_NAME}" ] && pass "创建备份 ${BACKUP_NAME}" || fail "创建备份失败"
[ -f "backups/${BACKUP_NAME}" ] && pass "备份文件已落盘" || fail "备份文件不存在"

# 删除应用后通过备份恢复
curl -s -b "${COOKIE}" -X DELETE "${BASE}/api/v1/apps/${APP_ID}" -H "X-CSRF-Token: ${CSRF}" \
  | grep -q '"success":true' && pass "删除应用" || fail "删除应用失败"
curl -s -b "${COOKIE}" "${BASE}/api/v1/apps" | grep -q '集成测试应用' && fail "删除后仍存在" || pass "删除后列表已更新"

# 停止服务后使用 CLI 恢复
kill -TERM "${SERVER_PID}" 2>/dev/null || true
sleep 2
"${BIN}" --config ./config.yaml --restore "backups/${BACKUP_NAME}" >/dev/null 2>&1 \
  && pass "CLI 恢复备份" || fail "CLI 恢复失败"

exec "${BIN}" --config ./config.yaml >"server2.log" 2>&1 &
SERVER_PID=$!
for i in $(seq 1 30); do
  curl -fsS "${BASE}/health" >/dev/null 2>&1 && break
  sleep 0.5
done
sleep 1
# 重新登录（会话已随数据库恢复而失效）
curl -s -c "c2.txt" -X POST "${BASE}/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d '{"username":"tester","password":"Test@2026"}' | grep -q 'csrf_token' \
  && pass "恢复后可重新登录" || fail "恢复后无法登录"
curl -s -b "c2.txt" "${BASE}/api/v1/apps" | grep -q '集成测试应用' \
  && pass "恢复后应用数据完整" || fail "恢复后应用数据缺失"

section "停止服务"
kill -TERM "${SERVER_PID}" 2>/dev/null || true
sleep 1
! curl -fsS "${BASE}/health" >/dev/null 2>&1 && pass "服务已停止" || fail "服务未能停止"

printf '\n=========================================\n'
printf '%s\n' "集成测试完成：通过 ${PASS}，失败 ${FAIL}"
printf '=========================================\n'
[ "${FAIL}" = "0" ] || exit 1
