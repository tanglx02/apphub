// Package backup 实现配置、数据库、上传文件的打包备份与恢复。
//
// 备份内容：config.yaml、SQLite 一致性快照、uploads/ 图标、meta.json 元信息。
// 恢复前会自动备份当前状态，恢复后需要重启服务（由 restore.sh 或 API 触发）。
package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/buildinfo"
	"github.com/tanglx02/apphub/internal/database"
	"github.com/tanglx02/apphub/internal/logging"
	"github.com/tanglx02/apphub/internal/models"
)

// ErrNotFound 备份不存在。
var ErrNotFound = errors.New("备份文件不存在")

// Manager 备份管理器。
type Manager struct {
	db         *database.DB
	root       string
	dir        string
	configPath string
	dataPath   string
	logger     *logging.Logger
	// dbCloser 由 CLI 恢复流程注入：替换数据库文件前先释放连接，
	// 避免 Windows 等平台因文件占用导致恢复失败。
	dbCloser func() error
}

// SetDBCloser 注入数据库连接关闭函数（供 apphub --restore 使用）。
func (m *Manager) SetDBCloser(fn func() error) { m.dbCloser = fn }

// New 创建备份管理器。
func New(db *database.DB, root, dir, configPath, dataPath string, logger *logging.Logger) *Manager {
	if logger == nil {
		logger = logging.L()
	}
	_ = os.MkdirAll(dir, 0o755)
	return &Manager{db: db, root: root, dir: dir, configPath: configPath, dataPath: dataPath, logger: logger}
}

// Dir 返回备份目录。
func (m *Manager) Dir() string { return m.dir }

// meta 备份元信息。
type meta struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	DBVersion int       `json:"db_version"`
	CreatedAt time.Time `json:"created_at"`
	Note      string    `json:"note"`
	Root      string    `json:"root"`
}

// Create 生成备份包。destDir 为空时使用默认备份目录。
func (m *Manager) Create(note string, destDir string) (*models.BackupRecord, error) {
	if destDir == "" {
		destDir = m.dir
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建备份目录失败: %w", err)
	}

	// 文件名需保证唯一：同一秒内的多次备份（例如恢复前的自动备份）不能互相覆盖
	name := uniqueBackupName(destDir)
	dest := filepath.Join(destDir, name)

	dbVersion, _ := m.db.Version()
	mf := meta{
		Name:      name,
		Version:   buildinfo.Version,
		DBVersion: dbVersion,
		CreatedAt: time.Now(),
		Note:      note,
		Root:      m.root,
	}

	// 数据库一致性快照写入临时文件
	tmpDir, err := os.MkdirTemp("", "apphub-backup")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	snapshot := filepath.Join(tmpDir, "apphub.db")
	if err := m.db.Backup(snapshot); err != nil {
		return nil, fmt.Errorf("导出数据库快照失败: %w", err)
	}

	if err := m.writeTar(dest, snapshot, mf); err != nil {
		return nil, err
	}

	st, _ := os.Stat(dest)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	rec := &models.BackupRecord{
		Name:      name,
		Path:      dest,
		SizeBytes: size,
		Note:      note,
		CreatedAt: time.Now(),
	}
	res, err := m.db.Exec(`INSERT INTO backups(name, path, size_bytes, note, created_at) VALUES(?, ?, ?, ?, ?)`,
		rec.Name, rec.Path, rec.SizeBytes, rec.Note, rec.CreatedAt)
	if err == nil {
		if id, e := res.LastInsertId(); e == nil {
			rec.ID = id
		}
	}
	m.logger.Info("已创建备份: %s (%d 字节)", dest, size)
	return rec, nil
}

// uniqueBackupName 生成不冲突的备份文件名。
func uniqueBackupName(dir string) string {
	base := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("apphub-backup-%s.tar.gz", base)
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return name
	}
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("apphub-backup-%s-%d.tar.gz", base, i)
		if _, err := os.Stat(filepath.Join(dir, candidate)); err != nil {
			return candidate
		}
	}
	return fmt.Sprintf("apphub-backup-%s-%d.tar.gz", base, time.Now().UnixNano()%100000)
}

func (m *Manager) writeTar(dest, snapshot string, mf meta) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// meta.json
	metaBytes, _ := json.MarshalIndent(mf, "", "  ")
	if err := writeEntry(tw, "meta.json", metaBytes, mf.CreatedAt); err != nil {
		return err
	}
	// config.yaml
	if data, err := os.ReadFile(m.configPath); err == nil {
		if err := writeEntry(tw, "config.yaml", data, time.Now()); err != nil {
			return err
		}
	}
	// 数据库快照
	if data, err := os.ReadFile(snapshot); err == nil {
		if err := writeEntry(tw, "data/apphub.db", data, time.Now()); err != nil {
			return err
		}
	}
	// uploads 图标
	uploads := filepath.Join(m.root, "uploads")
	if entries, err := os.ReadDir(uploads); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(uploads, e.Name()))
			if err != nil {
				continue
			}
			if err := writeEntry(tw, "uploads/"+e.Name(), data, time.Now()); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeEntry(tw *tar.Writer, name string, data []byte, mod time.Time) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: mod,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// List 列出全部备份（数据库记录 + 磁盘文件合并去重）。
func (m *Manager) List() ([]models.BackupRecord, error) {
	out := make([]models.BackupRecord, 0)
	seen := make(map[string]struct{})

	rows, err := m.db.Query(`SELECT id, name, path, size_bytes, note, created_at FROM backups ORDER BY created_at DESC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var r models.BackupRecord
			if err := rows.Scan(&r.ID, &r.Name, &r.Path, &r.SizeBytes, &r.Note, &r.CreatedAt); err != nil {
				continue
			}
			seen[r.Name] = struct{}{}
			out = append(out, r)
		}
	}

	entries, err := os.ReadDir(m.dir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
				continue
			}
			if _, ok := seen[e.Name()]; ok {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, models.BackupRecord{
				Name:      e.Name(),
				Path:      filepath.Join(m.dir, e.Name()),
				SizeBytes: info.Size(),
				CreatedAt: info.ModTime(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Options 恢复选项。
type Options struct {
	RestoreConfig  bool // 是否恢复 config.yaml（换机恢复建议开启）
	RestoreDB      bool
	RestoreUploads bool
	PreBackup      bool // 恢复前自动备份当前状态
}

// DefaultOptions 默认恢复选项。
func DefaultOptions() Options {
	return Options{RestoreConfig: true, RestoreDB: true, RestoreUploads: true, PreBackup: true}
}

// Restore 从备份包恢复。返回本次自动备份的文件名（可能为 nil）。
func (m *Manager) Restore(name string, opts Options) (*models.BackupRecord, error) {
	path := name
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.dir, filepath.Base(name))
	}
	if _, err := os.Stat(path); err != nil {
		return nil, ErrNotFound
	}

	var pre *models.BackupRecord
	if opts.PreBackup {
		p, err := m.Create("恢复前自动备份", "")
		if err != nil {
			m.logger.Warn("恢复前自动备份失败: %v", err)
		} else {
			pre = p
		}
	}

	tmpDir, err := os.MkdirTemp("", "apphub-restore")
	if err != nil {
		return pre, err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTar(path, tmpDir); err != nil {
		return pre, fmt.Errorf("解压备份失败: %w", err)
	}

	if opts.RestoreDB {
		src := filepath.Join(tmpDir, "data", "apphub.db")
		if _, err := os.Stat(src); err == nil {
			if m.dbCloser != nil {
				_ = m.dbCloser()
			}
			if err := os.MkdirAll(filepath.Dir(m.dataPath), 0o755); err != nil {
				return pre, err
			}
			// 先移除旧库及其 WAL / SHM，避免残留 WAL 被新库回放导致数据错乱
			for _, suffix := range []string{"", "-wal", "-shm"} {
				old := m.dataPath + suffix
				if _, err := os.Stat(old); err != nil {
					continue
				}
				if err := os.Remove(old); err != nil {
					return pre, fmt.Errorf("数据库文件仍被占用（%s），请先停止 AppHub 服务后再恢复", filepath.Base(old))
				}
			}
			if err := copyFile(src, m.dataPath); err != nil {
				return pre, err
			}
		}
	}

	if opts.RestoreConfig {
		src := filepath.Join(tmpDir, "config.yaml")
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(m.configPath, m.configPath+".pre-restore")
			if err := copyFile(src, m.configPath); err != nil {
				return pre, err
			}
		}
	}

	if opts.RestoreUploads {
		src := filepath.Join(tmpDir, "uploads")
		if entries, err := os.ReadDir(src); err == nil {
			dst := filepath.Join(m.root, "uploads")
			_ = os.MkdirAll(dst, 0o755)
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				_ = copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
			}
		}
	}

	m.logger.Info("已从备份恢复: %s", path)
	return pre, nil
}

// Delete 删除备份文件与记录。
func (m *Manager) Delete(name string) error {
	path := filepath.Join(m.dir, filepath.Base(name))
	if _, err := os.Stat(path); err != nil {
		return ErrNotFound
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	_, _ = m.db.Exec(`DELETE FROM backups WHERE name = ?`, name)
	return nil
}

// Cleanup 清理超过保留期的备份。
func (m *Manager) Cleanup(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	list, err := m.List()
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	removed := 0
	for _, r := range list {
		if r.CreatedAt.Before(cutoff) {
			if err := m.Delete(r.Name); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

func extractTar(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return errors.New("备份文件不是有效的 gzip 格式")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// 路径穿越防护
		clean := filepath.Clean(filepath.Join("/", hdr.Name))
		target := filepath.Join(dest, strings.TrimPrefix(clean, "/"))
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return errors.New("备份包包含非法路径")
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// RecordBackup 仅登记一条已存在的备份记录（供脚本生成的备份使用）。
func (m *Manager) RecordBackup(name, path, note string) error {
	st, _ := os.Stat(path)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	_, err := m.db.Exec(`INSERT INTO backups(name, path, size_bytes, note, created_at) VALUES(?, ?, ?, ?, ?)`,
		name, path, size, note, time.Now())
	return err
}

// EnsureSchema 供外部校验数据库连接可用。
func (m *Manager) EnsureSchema() error {
	var n int
	return m.db.QueryRow(`SELECT COUNT(*) FROM apps`).Scan(&n)
}
