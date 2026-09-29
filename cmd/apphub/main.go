// Command apphub 是 AppHub 的主程序：
// 一个轻量级的本地应用导航与服务控制中心。
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tanglx02/apphub/internal/api"
	"github.com/tanglx02/apphub/internal/apps"
	"github.com/tanglx02/apphub/internal/auth"
	"github.com/tanglx02/apphub/internal/backup"
	"github.com/tanglx02/apphub/internal/buildinfo"
	"github.com/tanglx02/apphub/internal/config"
	"github.com/tanglx02/apphub/internal/database"
	"github.com/tanglx02/apphub/internal/executor"
	"github.com/tanglx02/apphub/internal/health"
	"github.com/tanglx02/apphub/internal/logging"
	"github.com/tanglx02/apphub/internal/models"
	"github.com/tanglx02/apphub/internal/system"
	"github.com/tanglx02/apphub/internal/systemd"
)

func main() {
	var (
		showVersion   = flag.Bool("version", false, "显示版本信息")
		configPath    = flag.String("config", "", "配置文件路径（默认 ./config.yaml）")
		resetPassword = flag.Bool("reset-admin-password", false, "重置管理员密码（交互式）")
		runCheck      = flag.Bool("check", false, "执行环境与配置自检")
		runMigrate    = flag.Bool("migrate", false, "执行数据库迁移后退出")
		runBackup     = flag.String("backup", "", "创建备份到指定目录（空为默认备份目录）")
		runRestore    = flag.String("restore", "", "从指定备份文件恢复")
		migrateOnly   = flag.Bool("init-db", false, "仅初始化数据库后退出")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(buildinfo.String())
		return
	}

	path := *configPath
	if path == "" {
		if env := os.Getenv("APPHUB_CONFIG"); env != "" {
			path = env
		} else {
			path = defaultConfigPath()
		}
	}

	cfgManager, err := config.NewManager(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	cfg := cfgManager.Get()

	logger, err := logging.New(cfgManager.LogsDir(), logging.ParseLevel(cfg.Logging.Level),
		cfg.Logging.MaxSizeMB, cfg.Logging.RetentionDays, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	logging.SetDefault(logger)
	defer logger.Close()

	logger.Info("启动 AppHub %s (commit=%s build=%s)", buildinfo.Short(), buildinfo.GitCommit, buildinfo.BuildTime)
	logger.Info("项目根目录: %s", cfgManager.Root())

	// 数据库
	db, err := database.Open(cfgManager.DatabasePath(), cfg.Database.BusyTimeout)
	if err != nil {
		logger.Error("数据库初始化失败: %v", err)
		fmt.Fprintf(os.Stderr, "数据库初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// --migrate / --init-db：执行迁移后即退出，不启动服务
	if *runMigrate || *migrateOnly {
		fmt.Printf("数据库迁移完成，当前 schema 版本: %d\n", mustVersion(db))
		return
	}

	// 目录准备
	for _, dir := range []string{cfgManager.DataDir(), cfgManager.LogsDir(), cfgManager.BackupsDir(),
		cfgManager.UploadsDir(), cfgManager.RuntimeDir(), filepath.Join(cfgManager.RuntimeDir(), "logs"),
		filepath.Join(cfgManager.RuntimeDir(), "pids"), filepath.Join(cfgManager.DataDir(), "certs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Warn("创建目录失败 %s: %v", dir, err)
		}
	}

	settings := database.NewSettingsStore(db)
	audits := database.NewAuditStore(db)

	if *runCheck {
		runSelfCheck(logger, cfgManager, db, settings)
		return
	}

	// 备份 / 恢复（供脚本调用）
	backupMgr := backup.New(db, cfgManager.Root(), cfgManager.BackupsDir(), cfgManager.Path(),
		cfgManager.DatabasePath(), logger)
	if *runBackup != "" || flagBackupSet() {
		rec, err := backupMgr.Create("命令行备份", *runBackup)
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建备份失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("备份已创建: %s (%d 字节)\n", rec.Path, rec.SizeBytes)
		return
	}
	if *runRestore != "" {
		backupMgr.SetDBCloser(db.Close)
		pre, err := backupMgr.Restore(*runRestore, backup.DefaultOptions())
		if err != nil {
			fmt.Fprintf(os.Stderr, "恢复失败: %v\n", err)
			os.Exit(1)
		}
		if pre != nil {
			fmt.Printf("已自动备份当前状态: %s\n", pre.Path)
		}
		fmt.Println("恢复完成，请重启 AppHub 服务")
		return
	}

	if *resetPassword {
		if err := resetAdminPassword(db, logger); err != nil {
			fmt.Fprintf(os.Stderr, "重置密码失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 认证组件
	users := auth.NewUserStore(db.DB)
	sessions := auth.NewSessionStore(db.DB, cfgManager.SessionTTL())
	csrf := auth.NewCSRFManager()
	rateLimit := auth.NewRateLimiter(cfg.Security.RateLimitPerMin, time.Minute)
	loginGuard := auth.NewLoginGuard(cfg.Security.LoginMaxFails, cfg.Security.LoginLockMinutes)

	// 执行器与 systemd
	exec := executor.New(executorConfig(cfg), nil)
	exec.SetLogger(logger)
	executor.SetProcessLogDir(filepath.Join(cfgManager.RuntimeDir(), "logs"))
	executor.Registry().SetDir(filepath.Join(cfgManager.RuntimeDir(), "pids"))
	if err := executor.Registry().Load(); err != nil {
		logger.Warn("加载进程记录失败: %v", err)
	}

	sysd := systemd.NewClient(exec, time.Duration(cfg.Status.StartTimeout)*time.Second)
	if sysd.UseSudo() {
		logger.Info("当前非 root 运行，systemctl 将通过 sudo 调用（需配置 /etc/sudoers.d/apphub）")
	}

	// 应用仓储与服务
	repo := apps.NewRepository(db.DB)
	cats := apps.NewCategoryRepository(db.DB)
	endpoints := apps.NewEndpointRepository(db.DB)

	// 状态管理器
	checker := &health.Checker{
		SystemdActive: func(unit string) string {
			if !systemd.Available() {
				return "unknown"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			state, err := sysd.ActiveState(ctx, unit)
			if err != nil {
				return "unknown"
			}
			return state
		},
	}
	health.ProcessAlive = system.ProcessAlive

	statusMgr := health.NewManager(
		func() []models.App {
			list, err := repo.List()
			if err != nil {
				logger.Warn("读取应用列表失败: %v", err)
				return nil
			}
			return list
		},
		func(appID int64) int {
			if p := executor.Registry().Get(appID); p != nil && p.Alive() {
				return p.PID
			}
			return 0
		},
		checker,
		healthOptions(cfg),
	)

	var appSvc *apps.Service
	appSvc = apps.NewService(repo, cats, exec, sysd, statusMgr,
		func(action models.AuditAction, target, result, detail string, actor apps.Actor) {
			var userID int64
			if u, err := users.GetByUsername(actor.Username); err == nil && u != nil {
				userID = u.ID
			}
			if err := audits.Insert(userID, actor.Username, actor.IP, action, target, result, detail); err != nil {
				logger.Warn("写入审计日志失败: %v", err)
			}
		},
		func(key string, def bool) bool { return settings.GetBool(key, def) },
	)
	appSvc.SetBatchLimit(3)

	deps := api.Deps{
		Config:     cfgManager,
		DB:         db,
		Logger:     logger,
		Users:      users,
		Sessions:   sessions,
		CSRF:       csrf,
		RateLimit:  rateLimit,
		LoginGuard: loginGuard,
		Apps:       appSvc,
		Repo:       repo,
		Cats:       cats,
		Endpoints:  endpoints,
		Status:     statusMgr,
		Sysd:       sysd,
		Backups:    backupMgr,
		Settings:   settings,
		Audits:     audits,
		Exec:       exec,
		TLS:        cfg.Server.HTTPS || cfg.TLS.Enabled,
	}
	api.SetConfigProvider(func() config.Config { return cfgManager.Get() })

	server := api.New(deps)

	// 启动后台状态检测
	statusMgr.Start()
	defer statusMgr.Stop()

	// 应用开机自启（默认全部关闭）
	go func() {
		time.Sleep(2 * time.Second)
		appSvc.AutoStart(context.Background())
	}()

	// 日志与备份定期清理
	go maintenance(logger, audits, backupMgr, cfg)

	// HTTP 服务
	tlsCfg, err := buildTLSConfig(cfgManager, cfg, logger)
	if err != nil {
		logger.Error("TLS 配置失败: %v", err)
	}
	useTLS := tlsCfg != nil

	httpSrv := &http.Server{
		Addr:              cfgManager.Addr(),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig:         tlsCfg,
	}

	// HTTP -> HTTPS 重定向
	if useTLS && cfg.Server.RedirectHTTP && cfg.Server.HTTPPort > 0 && cfg.Server.HTTPPort != cfg.Server.Port {
		go func() {
			redirectSrv := &http.Server{
				Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HTTPPort),
				Handler:           http.HandlerFunc(redirectToHTTPS(cfg.Server.Port)),
				ReadHeaderTimeout: 10 * time.Second,
			}
			logger.Info("HTTP 重定向服务监听 :%d", cfg.Server.HTTPPort)
			if err := redirectSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Warn("HTTP 重定向服务已停止: %v", err)
			}
		}()
	}

	go func() {
		scheme := "http"
		if useTLS {
			scheme = "https"
		}
		logger.Info("AppHub 监听 %s://%s", scheme, cfgManager.Addr())
		printAccessURLs(scheme, cfg)
		var err error
		if useTLS {
			err = httpSrv.ListenAndServeTLS(certPath(cfgManager, cfg), keyPath(cfgManager, cfg))
		} else {
			err = httpSrv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("服务异常退出: %v", err)
			fmt.Fprintf(os.Stderr, "服务启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("正在停止 AppHub ...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("HTTP 服务关闭超时: %v", err)
	}
	logger.Info("AppHub 已停止")
}

func defaultConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "config.yaml"
}

func mustVersion(db *database.DB) int {
	v, err := db.Version()
	if err != nil {
		return 0
	}
	return v
}

func executorConfig(cfg config.Config) executor.Config {
	return executor.Config{
		MaxOutputBytes:  cfg.Executor.MaxOutputBytes,
		GracePeriodSec:  cfg.Executor.GracePeriodSec,
		KillProcessTree: cfg.Executor.KillProcessTree,
		DefaultTimeout:  time.Duration(cfg.Status.StartTimeout) * time.Second,
	}
}

func healthOptions(cfg config.Config) health.Options {
	return health.Options{
		Interval:    time.Duration(cfg.Status.IntervalSeconds) * time.Second,
		Timeout:     time.Duration(cfg.Status.TimeoutSeconds) * time.Second,
		Concurrency: cfg.Status.Concurrency,
	}
}

func certPath(cm *config.Manager, cfg config.Config) string { return cm.Resolve(cfg.TLS.CertFile) }

func keyPath(cm *config.Manager, cfg config.Config) string { return cm.Resolve(cfg.TLS.KeyFile) }

// buildTLSConfig 构造 TLS 配置；未启用 HTTPS 时返回 nil。
func buildTLSConfig(cm *config.Manager, cfg config.Config, logger *logging.Logger) (*tls.Config, error) {
	if !cfg.Server.HTTPS && !cfg.TLS.Enabled {
		return nil, nil
	}
	certFile := certPath(cm, cfg)
	keyFile := keyPath(cm, cfg)
	if _, err := os.Stat(certFile); err != nil || func() bool { _, e := os.Stat(keyFile); return e != nil }() {
		if !cfg.TLS.AutoGenerate && !cfg.TLS.SelfSigned {
			return nil, errors.New("已启用 HTTPS 但未找到证书文件")
		}
		logger.Info("未找到证书，正在生成自签名证书: %s", certFile)
		if err := generateSelfSigned(certFile, keyFile); err != nil {
			return nil, err
		}
	}
	return &tls.Config{MinVersion: tls.VersionTLS12}, nil
}

// generateSelfSigned 生成自签名证书（供内网/穿透场景使用）。
func generateSelfSigned(certFile, keyFile string) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0o755); err != nil {
		return err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	host, _ := os.Hostname()
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "AppHub Self-Signed",
			Organization: []string{"AppHub"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{host, "localhost"},
	}
	if ip := localIP(); ip != "" {
		tmpl.IPAddresses = []net.IP{net.ParseIP(ip)}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certOut, err := os.Create(certFile)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	return pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

func redirectToHTTPS(port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		target := fmt.Sprintf("https://%s:%d%s", host, port, r.URL.RequestURI())
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}

func printAccessURLs(scheme string, cfg config.Config) {
	for _, ip := range localIPs() {
		fmt.Printf("  访问地址: %s://%s:%d\n", scheme, ip, cfg.Server.Port)
	}
}

func localIPs() []string {
	out := make([]string, 0, 4)
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			out = append(out, ip4.String())
		}
	}
	if len(out) == 0 {
		out = append(out, "127.0.0.1")
	}
	return out
}

// maintenance 定期清理审计日志与过期备份。
func maintenance(logger *logging.Logger, audits *database.AuditStore, backups *backup.Manager, cfg config.Config) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if n, err := audits.Prune(cfg.Logging.RetentionDays); err == nil && n > 0 {
			logger.Info("已清理 %d 条过期审计日志", n)
		}
		if n, err := backups.Cleanup(cfg.Backup.RetentionDays); err == nil && n > 0 {
			logger.Info("已清理 %d 个过期备份", n)
		}
		if err := logger.Cleanup(); err != nil {
			logger.Warn("日志清理失败: %v", err)
		}
	}
}

// runSelfCheck 环境与配置自检。
func runSelfCheck(logger *logging.Logger, cm *config.Manager, db *database.DB, settings *database.SettingsStore) {
	fmt.Println("========== AppHub 自检 ==========")
	fmt.Printf("版本:        %s (%s)\n", buildinfo.Short(), buildinfo.GitCommit)
	fmt.Printf("配置文件:    %s\n", cm.Path())
	fmt.Printf("项目根目录:  %s\n", cm.Root())
	fmt.Printf("数据库:      %s\n", cm.DatabasePath())
	fmt.Printf("数据库版本:  %d\n", mustVersion(db))
	fmt.Printf("日志目录:    %s\n", cm.LogsDir())
	fmt.Printf("备份目录:    %s\n", cm.BackupsDir())
	fmt.Printf("systemd:     %s\n", boolToText(systemd.Available()))
	fmt.Printf("监听地址:    %s\n", cm.Addr())

	ok := true
	if err := db.Healthy(); err != nil {
		fmt.Printf("数据库状态:  异常 (%v)\n", err)
		ok = false
	} else {
		fmt.Println("数据库状态:  正常")
	}
	for _, dir := range []string{cm.DataDir(), cm.LogsDir(), cm.BackupsDir(), cm.UploadsDir(), cm.RuntimeDir()} {
		if _, err := os.Stat(dir); err != nil {
			fmt.Printf("目录缺失:    %s\n", dir)
			ok = false
		}
	}
	if !ok {
		os.Exit(1)
	}
	fmt.Println("自检通过")
}

func boolToText(b bool) string {
	if b {
		return "可用"
	}
	return "不可用"
}

// resetAdminPassword 交互式重置管理员密码，并使全部会话失效。
func resetAdminPassword(db *database.DB, logger *logging.Logger) error {
	users := auth.NewUserStore(db.DB)
	list, err := users.List()
	if err != nil || len(list) == 0 {
		return errors.New("未找到管理员账户，请先启动服务并通过初始化页面创建")
	}
	reader := bufio.NewReader(os.Stdin)
	username := list[0].Username
	if len(list) > 1 {
		fmt.Printf("可用管理员: ")
		names := make([]string, 0, len(list))
		for _, u := range list {
			names = append(names, u.Username)
		}
		fmt.Println(strings.Join(names, ", "))
		fmt.Print("请输入要重置的用户名: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			username = input
		}
	}
	fmt.Print("请输入新密码（至少 8 位）: ")
	pwd, _ := reader.ReadString('\n')
	pwd = strings.TrimSpace(pwd)
	if err := auth.ValidatePassword(pwd); err != nil {
		return err
	}
	hash, err := auth.HashPassword(pwd)
	if err != nil {
		return err
	}
	user, err := users.GetByUsername(username)
	if err != nil || user == nil {
		return errors.New("用户不存在")
	}
	if err := users.UpdatePassword(user.ID, hash); err != nil {
		return err
	}
	sessions := auth.NewSessionStore(db.DB, 0)
	_ = sessions.DeleteByUser(user.ID)
	logger.Info("已重置管理员密码: %s", username)
	fmt.Printf("管理员 %s 的密码已重置，全部会话已失效\n", username)
	return nil
}

// flagBackupSet 判断是否显式传入 --backup（值为空字符串也算）。
func flagBackupSet() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--backup" || strings.HasPrefix(arg, "--backup=") {
			return true
		}
	}
	return false
}
