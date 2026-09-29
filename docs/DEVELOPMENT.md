# 开发指南

## 环境要求

- Go 1.25+
- Node.js 18+（仅开发期构建前端需要，生产运行不需要）
- Bash（Linux / Git Bash）

## 目录约定

```
cmd/apphub/        主程序入口
internal/          业务代码（api/auth/config/database/apps/executor/systemd/health/backup/system）
web/               Vue3 + TS + Vite + Tailwind 前端（构建产物嵌入二进制）
scripts/           运维脚本
tests/             mock-app 与集成测试
examples/          示例应用配置
```

## 本地开发

```bash
# 后端
go run ./cmd/apphub --config ./config.yaml

# 前端（另开终端，代理到 18080）
cd web && npm install && npm run dev

# 只编译后端（使用已有前端产物）
go build -o apphub.exe ./cmd/apphub
```

首次运行会自动创建 `data/ logs/ backups/ runtime/ uploads/` 与默认 `config.yaml`，
浏览器打开 `http://127.0.0.1:18080` 进入初始化向导。

## 构建发布

```bash
./build.sh          # 构建前端 + arm64/amd64 二进制 + tar.gz 发布包 → dist/
./build.sh arm64    # 仅 arm64
./build.sh --skip-web
```

版本信息通过 ldflags 注入：`apphub --version` 输出版本、Commit、Build 时间。

## 测试

```bash
go test ./...                          # 单元测试（8 个包）
bash tests/integration/run.sh          # 端到端集成测试
bash tests/integration/run.sh          # 可用 APPHUB_TEST_PORT=18888 换端口
```

`tests/mock-app` 是自带的模拟 HTTP 应用（`/health` + 心跳日志），
用于验证启动 / 停止 / HTTP 检测 / 日志 / 访问地址，不依赖任何外部软件。

## 代码规范

- 禁止把逻辑塞进 main.go；新功能放对应 internal 包
- SQL 一律参数化；新增表结构必须追加 migration，禁止修改历史迁移
- 对外错误必须用户友好，真实错误写日志（`logging.L()`）
- 所有颜色使用 `web/src/assets/main.css` 的 Design Token，组件内禁止硬编码颜色
- 前端状态用组合式 store（`src/stores/`），不引入 Pinia/Redux 等重型状态库

## 修改流程（AI 协作约定）

1. 读 `PROJECT_SPEC.md` → 2. 读 `DEVELOPMENT_STATE.md` → 3. 查代码 → 4. 改动 →
5. 跑 `go test ./...` 与集成测试 → 6. 更新 `DEVELOPMENT_STATE.md`。

修改数据库必须追加 migration；修改 API 必须同步前端；修改安装流程必须同步 README。
