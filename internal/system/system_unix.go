//go:build !windows

package system

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// OSName 读取 /etc/os-release 中的 PRETTY_NAME。
func OSName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	return "Linux"
}

// Kernel 返回内核版本。
func Kernel() string {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return "unknown"
	}
	b := make([]byte, 0, 64)
	for _, c := range uts.Release {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

// UptimeSeconds 返回系统运行时长（秒）。
func UptimeSeconds() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return UptimeFallback()
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return UptimeFallback()
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return UptimeFallback()
	}
	return int64(v)
}

// Memory 返回内存总量与已用（MB）。
func Memory() (totalMB, usedMB uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var total, avail uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			avail = parseKB(line)
		}
	}
	totalMB = total / 1024
	if total > avail {
		usedMB = (total - avail) / 1024
	}
	return
}

func parseKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[1], 10, 64)
	return v
}

// Disk 返回指定路径所在文件系统容量（GB）。
func Disk(path string) (totalGB, usedGB float64) {
	if path == "" {
		path = "/"
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0
	}
	total := float64(st.Blocks) * float64(st.Bsize)
	free := float64(st.Bavail) * float64(st.Bsize)
	totalGB = total / 1024 / 1024 / 1024
	usedGB = (total - free) / 1024 / 1024 / 1024
	return
}

// LoadAvg 返回 1/5/15 分钟平均负载。
func LoadAvg() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "n/a"
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return "n/a"
	}
	return strings.Join(fields[:3], " / ")
}

// CPUModel 返回 CPU 型号。
func CPUModel() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Model") || strings.HasPrefix(line, "Hardware") {
			if idx := strings.Index(line, ":"); idx > 0 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}

// sampleCPUPercent 采样两次 /proc/stat 计算 CPU 使用率。
func sampleCPUPercent() float64 {
	read := func() (idle, total uint64) {
		f, err := os.Open("/proc/stat")
		if err != nil {
			return 0, 0
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "cpu ") {
				continue
			}
			fields := strings.Fields(line)[1:]
			var sum uint64
			for _, s := range fields {
				v, _ := strconv.ParseUint(s, 10, 64)
				sum += v
			}
			if len(fields) >= 5 {
				idle, _ = strconv.ParseUint(fields[3], 10, 64) // idle
				// iowait 计入空闲
				if len(fields) >= 6 {
					iowait, _ := strconv.ParseUint(fields[4], 10, 64)
					idle += iowait
				}
			}
			return idle, sum
		}
		return 0, 0
	}

	idle1, total1 := read()
	time.Sleep(250 * time.Millisecond)
	idle2, total2 := read()
	if total2 <= total1 {
		return 0
	}
	totalDelta := total2 - total1
	idleDelta := idle2 - idle1
	used := float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	if used < 0 {
		return 0
	}
	if used > 100 {
		return 100
	}
	return used
}

// LocalIP 返回本机主要的非回环 IPv4 地址。
func LocalIP() string {
	// 优先通过 UDP 拨号获取默认出口网卡地址（不实际发包）
	if ip := outboundIP(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

// ProcessAlive 判断进程是否存活。
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// ProcessStats 读取进程 CPU/内存占用（Linux /proc）。
func ProcessStats(pid int) (cpuPercent float64, memMB float64) {
	if pid <= 0 {
		return 0, 0
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0
	}
	// comm 可能包含空格，从最后一个 ) 之后开始解析
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 || idx+2 >= len(s) {
		return 0, 0
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 22 {
		return 0, 0
	}
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	rss, _ := strconv.ParseUint(fields[21], 10, 64)
	total := utime + stime
	uptime := float64(UptimeSeconds())
	if uptime > 0 {
		seconds := float64(total) / clkTck()
		cpuPercent = seconds / uptime * 100
	}
	memMB = float64(rss) * float64(pageSize()) / 1024 / 1024
	return
}

func clkTck() float64 { return 100 }

func pageSize() int64 { return int64(syscall.Getpagesize()) }
