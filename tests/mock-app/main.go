// Command mock-app 是 AppHub 自带的模拟 HTTP 服务。
//
// 用途：在不依赖任何外部应用的情况下，验证 AppHub 的
// 启动 / 停止 / HTTP 在线检测 / 日志 / 访问地址等能力。
//
// 构建：go build -o mock-app ./tests/mock-app
// 端口：默认 19090，可通过环境变量 MOCK_PORT 覆盖
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	port := 19090
	if v := os.Getenv("MOCK_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"apphub-mock"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8">
<title>AppHub Mock App</title>
<h1>AppHub 模拟应用</h1>
<p>用于验证启停与在线检测。PID=` + strconv.Itoa(os.Getpid()) + `</p>
<p><a href="/health">/health</a></p>`))
	})
	// 每 5 秒输出一行日志，便于验证日志功能
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		i := 0
		for range ticker.C {
			i++
			log.Printf("mock-app 心跳 %d (pid=%d)", i, os.Getpid())
		}
	}()

	log.Printf("mock-app 启动，监听 :%d", port)
	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "mock-app 退出:", err)
		os.Exit(1)
	}
}
