// Package logging 提供轻量、低开销的分级日志，支持文件轮转与保留期清理。
//
// 设计目标：ARM64 + SD 卡环境，日志不能无限增长。
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Level 日志级别。
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	}
	return "INFO"
}

// ParseLevel 解析级别字符串。
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger 为分级日志器。
type Logger struct {
	mu        sync.Mutex
	level     Level
	dir       string
	maxSize   int64 // 字节
	retention int
	console   bool
	file      *os.File
	curSize   int64
	seq       int
	prefix    string
}

// New 创建日志器；dir 为空时只输出到 stderr。
func New(dir string, level Level, maxSizeMB, retentionDays int, console bool) (*Logger, error) {
	l := &Logger{
		level:     level,
		dir:       dir,
		maxSize:   int64(maxSizeMB) * 1024 * 1024,
		retention: retentionDays,
		console:   console,
		prefix:    "apphub",
	}
	if l.maxSize <= 0 {
		l.maxSize = 20 * 1024 * 1024
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := l.rotate(); err != nil {
			return nil, err
		}
		go l.cleanupLoop()
	}
	return l, nil
}

func (l *Logger) filename() string {
	return filepath.Join(l.dir, l.prefix+".log")
}

// rotate 打开（或轮转）日志文件。
func (l *Logger) rotate() error {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	f, err := os.OpenFile(l.filename(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	l.curSize = st.Size()
	l.file = f
	return nil
}

func (l *Logger) write(level Level, msg string) {
	if level < l.level {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	line := fmt.Sprintf("%s [%s] %s\n", ts, level.String(), msg)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.console {
		if level >= LevelWarn {
			_, _ = os.Stderr.WriteString(line)
		} else {
			_, _ = os.Stdout.WriteString(line)
		}
	}
	if l.file != nil {
		n, _ := l.file.WriteString(line)
		l.curSize += int64(n)
		if l.curSize >= l.maxSize {
			// 轮转：先关闭，重命名，再新建
			_ = l.file.Close()
			old := filepath.Join(l.dir, fmt.Sprintf("%s-%s.log", l.prefix, time.Now().Format("20060102-150405")))
			_ = os.Rename(l.filename(), old)
			if err := l.rotate(); err != nil {
				_, _ = os.Stderr.WriteString("日志轮转失败: " + err.Error() + "\n")
			}
		}
	}
}

// Debug 输出调试日志。
func (l *Logger) Debug(format string, args ...any) {
	l.write(LevelDebug, fmt.Sprintf(format, args...))
}

// Info 输出信息日志。
func (l *Logger) Info(format string, args ...any) {
	l.write(LevelInfo, fmt.Sprintf(format, args...))
}

// Warn 输出警告日志。
func (l *Logger) Warn(format string, args ...any) {
	l.write(LevelWarn, fmt.Sprintf(format, args...))
}

// Error 输出错误日志。
func (l *Logger) Error(format string, args ...any) {
	l.write(LevelError, fmt.Sprintf(format, args...))
}

// SetLevel 动态调整级别。
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Writer 返回可用于标准库 log 输出的 io.Writer。
func (l *Logger) Writer(level Level) io.Writer { return &writer{logger: l, level: level} }

type writer struct {
	logger *Logger
	level  Level
}

func (w *writer) Write(p []byte) (int, error) {
	w.logger.write(w.level, strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// Close 关闭日志文件。
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// cleanupLoop 定期清理过期日志与压缩旧文件。
func (l *Logger) cleanupLoop() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if err := l.Cleanup(); err != nil {
			_, _ = os.Stderr.WriteString("日志清理失败: " + err.Error() + "\n")
		}
	}
}

// Cleanup 删除超过保留期的日志文件。
func (l *Logger) Cleanup() error {
	if l.dir == "" || l.retention <= 0 {
		return nil
	}
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -l.retention)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, l.prefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) && name != l.prefix+".log" {
			_ = os.Remove(filepath.Join(l.dir, name))
		}
	}
	// 仅保留最近 10 个轮转文件
	var rotated []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), l.prefix+"-") && strings.HasSuffix(e.Name(), ".log") {
			rotated = append(rotated, e)
		}
	}
	if len(rotated) > 10 {
		sort.Slice(rotated, func(i, j int) bool {
			a, _ := rotated[i].Info()
			b, _ := rotated[j].Info()
			return a.ModTime().Before(b.ModTime())
		})
		for _, e := range rotated[:len(rotated)-10] {
			_ = os.Remove(filepath.Join(l.dir, e.Name()))
		}
	}
	return nil
}

// 包级默认日志器，便于渐进式替换与测试。
var defaultLogger = func() *Logger {
	l, _ := New("", LevelInfo, 20, 30, true)
	return l
}()

// SetDefault 设置默认日志器。
func SetDefault(l *Logger) {
	if l != nil {
		defaultLogger = l
		log.SetOutput(l.Writer(LevelInfo))
		log.SetFlags(0)
	}
}

// L 返回默认日志器。
func L() *Logger { return defaultLogger }

// Info 使用默认日志器输出。
func Info(format string, args ...any) { defaultLogger.Info(format, args...) }

// Warn 使用默认日志器输出。
func Warn(format string, args ...any) { defaultLogger.Warn(format, args...) }

// Error 使用默认日志器输出。
func Error(format string, args ...any) { defaultLogger.Error(format, args...) }

// Debug 使用默认日志器输出。
func Debug(format string, args ...any) { defaultLogger.Debug(format, args...) }
