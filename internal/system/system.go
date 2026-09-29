// Package system 采集服务器运行信息与资源占用，供首页"系统总览"使用。
//
// 资源数据采用低频采样 + 缓存，避免频繁读取 /proc 造成开销。
package system

import (
	"net"
	"os"
	"runtime"
	"sync"
	"time"
)

// outboundIP 通过 UDP 拨号获取默认出口 IP（不产生实际网络流量）。
func outboundIP() string {
	conn, err := net.Dial("udp", "223.5.5.5:80")
	if err != nil {
		// 退化为遍历网卡地址
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
	defer conn.Close()
	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return ""
	}
	return host
}

// 进程启动时刻，用于无 /proc 环境下估算运行时长。
var bootTime = time.Now()

// Hostname 返回主机名。
func Hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// Arch 返回 CPU 架构（arm64 / amd64 ...）。
func Arch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64"
	case "amd64":
		return "amd64"
	default:
		return runtime.GOARCH
	}
}

var (
	cpuCacheMu  sync.Mutex
	cpuCacheVal float64
	cpuCacheAt  time.Time
	cpuCacheTTL = 3 * time.Second
)

// CPUPercent 返回 CPU 使用率（带 3 秒缓存）。
func CPUPercent() float64 {
	cpuCacheMu.Lock()
	defer cpuCacheMu.Unlock()
	if time.Since(cpuCacheAt) < cpuCacheTTL {
		return cpuCacheVal
	}
	v := sampleCPUPercent()
	cpuCacheVal = v
	cpuCacheAt = time.Now()
	return v
}

// CPUCores 返回逻辑 CPU 核数。
func CPUCores() int { return runtime.NumCPU() }

// UptimeFallback 用于不支持 /proc 的平台。
func UptimeFallback() int64 { return int64(time.Since(bootTime).Seconds()) }
