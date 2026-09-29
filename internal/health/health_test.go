package health

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/tanglx02/apphub/internal/models"
)

func TestCheckTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("无法监听端口: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()
	ok, detail := CheckTCP(context.Background(), addr, 2*time.Second)
	if !ok {
		t.Fatalf("在线端口应检测为在线: %s", detail)
	}

	// 关闭后再检测应为离线
	ln.Close()
	ok, _ = CheckTCP(context.Background(), addr, 2*time.Second)
	if ok {
		t.Fatal("已关闭端口不应检测为在线")
	}
}

func TestCheckTCPPortOnly(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	ok, _ := CheckTCP(context.Background(), portStr, 2*time.Second)
	if !ok {
		t.Fatal("纯端口形式应自动按 127.0.0.1 处理")
	}
}

func TestCheckHTTP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/boom", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	srv := &http.Server{Handler: mux}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	base := "http://" + ln.Addr().String()

	if ok, d := CheckHTTP(context.Background(), base+"/health", 2*time.Second, nil); !ok {
		t.Fatalf("200 应视为在线: %s", d)
	}
	if ok, _ := CheckHTTP(context.Background(), base+"/boom", 2*time.Second, nil); ok {
		t.Fatal("500 不应视为在线")
	}
	if ok, _ := CheckHTTP(context.Background(), base+"/boom", 2*time.Second, []int{500}); !ok {
		t.Fatal("期望状态码包含 500 时应视为在线")
	}
	if ok, _ := CheckHTTP(context.Background(), "file:///etc/passwd", 2*time.Second, nil); ok {
		t.Fatal("非 http 协议必须被拒绝")
	}
}

func TestCheckerDispatch(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = host
	_ = port
	pid, _ := strconv.Atoi("1")

	c := &Checker{SystemdActive: func(u string) string { return "active" }}

	app := &models.App{Name: "t", AppType: models.AppTypeSystemd, SystemdUnit: "x.service", StatusType: models.CheckSystemd}
	if st, _ := c.Check(context.Background(), app, 0, time.Second); st != models.StatusOnline {
		t.Fatalf("active 应映射为 ONLINE，实际 %s", st)
	}

	app2 := &models.App{Name: "t", StatusType: models.CheckProcess, StatusTarget: strconv.Itoa(pid)}
	ProcessAlive = func(p int) bool { return p == pid }
	if st, _ := c.Check(context.Background(), app2, 0, time.Second); st != models.StatusOnline {
		t.Fatalf("存活进程应映射为 ONLINE，实际 %s", st)
	}

	app3 := &models.App{Name: "t", StatusType: models.CheckNone}
	if st, _ := c.Check(context.Background(), app3, 0, time.Second); st != models.StatusUnknown {
		t.Fatalf("none 应映射为 UNKNOWN，实际 %s", st)
	}
}

func TestManagerTransitionAndCache(t *testing.T) {
	app := &models.App{ID: 7, Name: "demo", Enabled: true, StatusType: models.CheckNone}
	m := NewManager(
		func() []models.App { return []models.App{*app} },
		func(id int64) int { return 0 },
		&Checker{},
		Options{Interval: time.Second, Timeout: time.Second, Concurrency: 2},
	)
	defer m.Stop()

	// 手动置为启动中
	m.MarkTransition(7, models.StatusStarting, 10*time.Second)
	if got := m.Get(7).Status; got != models.StatusStarting {
		t.Fatalf("过渡态未生效: %s", got)
	}

	// 过渡态期间即使真实检测为 UNKNOWN 也不应覆盖
	m.refreshOneForTest(app)
	if got := m.Get(7).Status; got != models.StatusStarting {
		t.Fatalf("过渡态期间不应被覆盖: %s", got)
	}

	// 清除过渡态后应更新为真实状态
	m.ClearTransition(7)
	m.refreshOneForTest(app)
	if got := m.Get(7).Status; got != models.StatusUnknown {
		t.Fatalf("清除过渡态后应更新为真实状态: %s", got)
	}

	snap := m.Snapshot()
	if _, ok := snap[7]; !ok {
		t.Fatal("快照中应包含该应用")
	}
}

// refreshOneForTest 直接触发一次检测（绕过异步通道）。
func (m *Manager) refreshOneForTest(app *models.App) {
	m.runCheck(app, time.Second)
}
