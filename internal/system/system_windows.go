//go:build windows

package system

import "os"

func osFindProcess(pid int) (*os.Process, error) { return os.FindProcess(pid) }

// Windows 仅用于开发调试，生产环境为 Linux。以下实现返回保守值。

// OSName 返回操作系统名称。
func OSName() string { return "Windows (dev)" }

// Kernel 返回内核版本占位。
func Kernel() string { return "windows" }

// UptimeSeconds 返回进程运行时长作为近似值。
func UptimeSeconds() int64 { return UptimeFallback() }

// Memory 返回内存信息（Windows 下返回 0，由前端隐藏）。
func Memory() (totalMB, usedMB uint64) { return 0, 0 }

// Disk 返回磁盘信息（Windows 下返回 0）。
func Disk(path string) (totalGB, usedGB float64) { return 0, 0 }

// LoadAvg 返回负载占位。
func LoadAvg() string { return "n/a" }

// CPUModel 返回 CPU 型号占位。
func CPUModel() string { return "" }

// sampleCPUPercent Windows 下不采样，返回 0。
func sampleCPUPercent() float64 { return 0 }

// LocalIP 返回出口 IP。
func LocalIP() string { return outboundIP() }

// ProcessAlive 判断进程是否存活。
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := osFindProcess(pid)
	if err != nil {
		return false
	}
	return p != nil
}

// ProcessStats Windows 下不采集进程资源。
func ProcessStats(pid int) (cpuPercent float64, memMB float64) { return 0, 0 }
