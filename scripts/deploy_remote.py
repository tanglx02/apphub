"""AppHub 部署脚本：上传发布包、安装、配置 systemd、启动并设置开机自启。

用法：python deploy_remote.py <host> <port> <user> <password> <tar.gz>
"""
import sys
import time

import paramiko

HOST = sys.argv[1]
PORT = int(sys.argv[2])
USER = sys.argv[3]
PASSWORD = sys.argv[4]
PKG = sys.argv[5]

INSTALL_DIR = "/opt/apps/apphub"
SERVICE = "/etc/systemd/system/apphub.service"
REMOTE_PKG = "/tmp/apphub.tar.gz"


def run(ssh, cmd, timeout=120, check=True):
    """执行远程命令并打印输出。"""
    print(f"\n$ {cmd}")
    _, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace").strip()
    err = stderr.read().decode("utf-8", "replace").strip()
    code = stdout.channel.recv_exit_status()
    if out:
        print(out)
    if err:
        print("[stderr]", err)
    if check and code != 0:
        raise SystemExit(f"命令失败（退出码 {code}）：{cmd}")
    return code, out, err


def main():
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    print(f"连接 {HOST}:{PORT} ...")
    ssh.connect(HOST, port=PORT, username=USER, password=PASSWORD, timeout=15)
    print("已连接")

    # 1. 环境检测
    _, out, _ = run(ssh, "uname -sm")
    if "aarch64" not in out and "arm64" not in out:
        raise SystemExit(f"目标架构不是 ARM64：{out}，请确认发布包")
    run(ssh, "cat /etc/os-release | head -n2", check=False)
    run(ssh, "systemctl --version | head -n1")
    run(ssh, "command -v curl || apt-get install -y curl", check=False)

    # 2. 上传发布包
    sftp = ssh.open_sftp()
    print(f"\n上传 {PKG} -> {REMOTE_PKG} ...")
    sftp.put(PKG, REMOTE_PKG)
    sftp.close()
    run(ssh, f"ls -lh {REMOTE_PKG}")

    # 3. 解压安装（备份旧版本，保留数据目录）
    run(ssh, f"mkdir -p {INSTALL_DIR}")
    run(
        ssh,
        f"if [ -f {INSTALL_DIR}/apphub ]; then "
        f"cp -f {INSTALL_DIR}/apphub {INSTALL_DIR}/apphub.old-$(date +%Y%m%d%H%M%S) 2>/dev/null; fi;"
        f"mkdir -p {INSTALL_DIR}/{{data,logs,backups,runtime,uploads,scripts,examples,deploy}}",
    )
    run(ssh, f"tar -xzf {REMOTE_PKG} -C {INSTALL_DIR} --strip-components=1")
    run(ssh, f"chmod +x {INSTALL_DIR}/apphub {INSTALL_DIR}/scripts/*.sh")
    run(ssh, f"{INSTALL_DIR}/apphub --version")

    # 4. 生成配置文件（不覆盖已有配置）
    run(
        ssh,
        f"""if [ ! -f {INSTALL_DIR}/config.yaml ]; then cat > {INSTALL_DIR}/config.yaml <<'CFG'
# AppHub 配置（由部署脚本生成）
server:
  host: 0.0.0.0
  port: 18080
  https: false
  redirect_http: false
  http_port: 18081
  trusted_proxies: ""
  site_name: AppHub
  timezone: Asia/Shanghai

database:
  path: ./data/apphub.db
  busy_timeout_ms: 5000

logging:
  level: info
  retention_days: 30
  max_size_mb: 20
  console: true

security:
  session_timeout: 86400
  login_max_fails: 5
  login_lock_minutes: 15
  rate_limit_per_min: 120
  csrf_enabled: true

status:
  interval: 5
  timeout: 5
  concurrency: 4

tls:
  enabled: false
  cert_file: ./data/certs/server.crt
  key_file: ./data/certs/server.key
  auto_generate: true

backup:
  directory: ./backups
  retention_days: 30

executor:
  max_output_bytes: 1048576
  kill_process_tree: true
  grace_period_sec: 10
CFG
fi""",
        check=False,
    )
    run(ssh, f"ls -l {INSTALL_DIR}/config.yaml")

    # 5. 安装 systemd 服务并设置开机自启
    run(
        ssh,
        f"""cat > {SERVICE} <<'UNIT'
[Unit]
Description=AppHub - 本地应用导航与服务控制中心
After=network-online.target
Wants=network-online.target
RequiresMountsFor={INSTALL_DIR}
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
WorkingDirectory={INSTALL_DIR}
ExecStart={INSTALL_DIR}/apphub --config {INSTALL_DIR}/config.yaml
Restart=on-failure
RestartSec=5
TimeoutStopSec=20
KillSignal=SIGTERM
KillMode=mixed
StandardOutput=journal
StandardError=journal
SyslogIdentifier=apphub
MemoryMax=512M
CPUQuota=50%
TasksMax=256
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths={INSTALL_DIR}

[Install]
WantedBy=multi-user.target
UNIT""",
    )
    run(ssh, "systemctl daemon-reload")
    run(ssh, "systemctl enable apphub.service")
    run(ssh, "systemctl restart apphub.service")

    # 6. 健康检查
    print("\n等待服务就绪 ...")
    healthy = False
    for i in range(20):
        time.sleep(1)
        code, out, _ = run(ssh, "curl -fsS http://127.0.0.1:18080/health", check=False)
        if code == 0 and '"status":"ok"' in out:
            healthy = True
            break
    run(ssh, "systemctl is-active apphub.service && systemctl is-enabled apphub.service")
    run(ssh, "systemctl status apphub.service --no-pager -n 3 | head -n 10", check=False)
    run(ssh, f"rm -f {REMOTE_PKG}", check=False)

    if not healthy:
        run(ssh, "journalctl -u apphub -n 30 --no-pager", check=False)
        raise SystemExit("健康检查失败，请查看上方日志")

    print("\n========== 部署完成 ==========")
    print(f"安装目录：{INSTALL_DIR}")
    print(f"访问地址：http://{HOST}:18080")
    print("开机自启：已启用（systemctl is-enabled apphub）")
    ssh.close()


if __name__ == "__main__":
    main()
