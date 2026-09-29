// Package apps 管理应用定义与分类的持久化，并封装启停控制逻辑。
package apps

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/tanglx02/apphub/internal/models"
)

// ErrNotFound 应用不存在。
var ErrNotFound = errors.New("应用不存在")

// Repository 应用仓储。
type Repository struct {
	db *sql.DB
}

// NewRepository 创建应用仓储。
func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

const appColumns = `id, name, slug, description, icon, category_id, tags, internal_url, external_url,
 app_type, systemd_unit, start_command, stop_command, restart_command, shell_mode,
 start_args, stop_args, restart_args, status_type, status_target, expected_codes,
 work_dir, environment, enabled, auto_start, favorite, sort_order, timeout_seconds,
 check_timeout_sec, public_visible, scope, created_at, updated_at`

func scanApp(rows interface{ Scan(dest ...any) error }) (*models.App, error) {
	var a models.App
	var appType, statusType, scope string
	var shell, publicVisible int
	err := rows.Scan(
		&a.ID, &a.Name, &a.Slug, &a.Description, &a.Icon, &a.CategoryID, &a.Tags,
		&a.InternalURL, &a.ExternalURL, &appType, &a.SystemdUnit, &a.StartCommand,
		&a.StopCommand, &a.RestartCommand, &shell, &a.StartArgs, &a.StopArgs,
		&a.RestartArgs, &statusType, &a.StatusTarget, &a.ExpectedCodes, &a.WorkDir,
		&a.Environment, &a.Enabled, &a.AutoStart, &a.Favorite, &a.SortOrder,
		&a.TimeoutSeconds, &a.CheckTimeoutSec, &publicVisible, &scope, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.AppType = models.AppType(appType)
	a.StatusType = models.StatusCheckType(statusType)
	a.ShellMode = shell == 1
	a.PublicVisible = publicVisible == 1
	a.Scope = models.AppScope(scope)
	if a.Scope == "" {
		a.Scope = models.ScopeLocal
	}
	return &a, nil
}

// List 返回全部应用（按排序、收藏优先）。
func (r *Repository) List() ([]models.App, error) {
	rows, err := r.db.Query(`SELECT ` + appColumns + ` FROM apps ORDER BY favorite DESC, sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.App, 0)
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			continue
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Get 按 ID 查询。
func (r *Repository) Get(id int64) (*models.App, error) {
	row := r.db.QueryRow(`SELECT `+appColumns+` FROM apps WHERE id = ?`, id)
	a, err := scanApp(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

// GetBySlug 按 slug 查询。
func (r *Repository) GetBySlug(slug string) (*models.App, error) {
	row := r.db.QueryRow(`SELECT `+appColumns+` FROM apps WHERE slug = ?`, slug)
	a, err := scanApp(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// Create 创建应用。
func (r *Repository) Create(a *models.App) error {
	now := time.Now()
	a.CreatedAt = now
	a.UpdatedAt = now
	if strings.TrimSpace(a.Slug) == "" {
		a.Slug = Slugify(a.Name)
	}
	if a.SortOrder == 0 {
		var maxOrder sql.NullInt64
		_ = r.db.QueryRow(`SELECT MAX(sort_order) FROM apps`).Scan(&maxOrder)
		a.SortOrder = int(maxOrder.Int64) + 1
	}
	shell := 0
	if a.ShellMode {
		shell = 1
	}
	res, err := r.db.Exec(`INSERT INTO apps(name, slug, description, icon, category_id, tags,
	 internal_url, external_url, app_type, systemd_unit, start_command, stop_command, restart_command,
	 shell_mode, start_args, stop_args, restart_args, status_type, status_target, expected_codes,
	 work_dir, environment, enabled, auto_start, favorite, sort_order, timeout_seconds,
	 check_timeout_sec, public_visible, scope, created_at, updated_at)
	 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, a.Slug, a.Description, a.Icon, a.CategoryID, a.Tags, a.InternalURL, a.ExternalURL,
		string(a.AppType), a.SystemdUnit, a.StartCommand, a.StopCommand, a.RestartCommand, shell,
		a.StartArgs, a.StopArgs, a.RestartArgs, string(a.StatusType), a.StatusTarget, a.ExpectedCodes,
		a.WorkDir, a.Environment, boolToInt(a.Enabled), boolToInt(a.AutoStart), boolToInt(a.Favorite),
		a.SortOrder, a.TimeoutSeconds, a.CheckTimeoutSec, boolToInt(a.PublicVisible),
		string(a.Scope), a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	a.ID = id
	return nil
}

// Update 更新应用。
func (r *Repository) Update(a *models.App) error {
	a.UpdatedAt = time.Now()
	if strings.TrimSpace(a.Slug) == "" {
		a.Slug = Slugify(a.Name)
	}
	shell := 0
	if a.ShellMode {
		shell = 1
	}
	_, err := r.db.Exec(`UPDATE apps SET name=?, slug=?, description=?, icon=?, category_id=?, tags=?,
	 internal_url=?, external_url=?, app_type=?, systemd_unit=?, start_command=?, stop_command=?,
	 restart_command=?, shell_mode=?, start_args=?, stop_args=?, restart_args=?, status_type=?,
	 status_target=?, expected_codes=?, work_dir=?, environment=?, enabled=?, auto_start=?,
	 favorite=?, sort_order=?, timeout_seconds=?, check_timeout_sec=?, public_visible=?, scope=?, updated_at=? WHERE id=?`,
		a.Name, a.Slug, a.Description, a.Icon, a.CategoryID, a.Tags, a.InternalURL, a.ExternalURL,
		string(a.AppType), a.SystemdUnit, a.StartCommand, a.StopCommand, a.RestartCommand, shell,
		a.StartArgs, a.StopArgs, a.RestartArgs, string(a.StatusType), a.StatusTarget, a.ExpectedCodes,
		a.WorkDir, a.Environment, boolToInt(a.Enabled), boolToInt(a.AutoStart), boolToInt(a.Favorite),
		a.SortOrder, a.TimeoutSeconds, a.CheckTimeoutSec, boolToInt(a.PublicVisible),
		string(a.Scope), a.UpdatedAt, a.ID)
	return err
}

// Delete 删除应用定义（绝不触碰用户实际项目目录）。
func (r *Repository) Delete(id int64) error {
	res, err := r.db.Exec(`DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFavorite 切换收藏。
func (r *Repository) SetFavorite(id int64, fav bool) error {
	_, err := r.db.Exec(`UPDATE apps SET favorite = ?, updated_at = ? WHERE id = ?`, boolToInt(fav), time.Now(), id)
	return err
}

// Reorder 批量更新排序（管理员拖拽结果）。
func (r *Repository) Reorder(ids []int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE apps SET sort_order = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CountAll 统计应用数量。
func (r *Repository) CountAll() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM apps`).Scan(&n)
	return n, err
}

// CategoryRepository 分类仓储。
type CategoryRepository struct {
	db *sql.DB
}

// NewCategoryRepository 创建分类仓储。
func NewCategoryRepository(db *sql.DB) *CategoryRepository { return &CategoryRepository{db: db} }

// List 返回全部分类。
func (r *CategoryRepository) List() ([]models.Category, error) {
	rows, err := r.db.Query(`SELECT id, name, slug, icon, color, sort_order, created_at FROM categories ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Category, 0)
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Icon, &c.Color, &c.SortOrder, &c.CreatedAt); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get 按 ID 查询分类。
func (r *CategoryRepository) Get(id int64) (*models.Category, error) {
	var c models.Category
	err := r.db.QueryRow(`SELECT id, name, slug, icon, color, sort_order, created_at FROM categories WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Icon, &c.Color, &c.SortOrder, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// Create 创建分类。
func (r *CategoryRepository) Create(c *models.Category) error {
	if strings.TrimSpace(c.Slug) == "" {
		c.Slug = Slugify(c.Name)
	}
	if c.SortOrder == 0 {
		var max sql.NullInt64
		_ = r.db.QueryRow(`SELECT MAX(sort_order) FROM categories`).Scan(&max)
		c.SortOrder = int(max.Int64) + 1
	}
	res, err := r.db.Exec(`INSERT INTO categories(name, slug, icon, color, sort_order, created_at) VALUES(?, ?, ?, ?, ?, ?)`,
		c.Name, c.Slug, c.Icon, c.Color, c.SortOrder, time.Now())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	return nil
}

// Update 更新分类。
func (r *CategoryRepository) Update(c *models.Category) error {
	_, err := r.db.Exec(`UPDATE categories SET name=?, slug=?, icon=?, color=?, sort_order=? WHERE id=?`,
		c.Name, c.Slug, c.Icon, c.Color, c.SortOrder, c.ID)
	return err
}

// Delete 删除分类（不影响应用本体，仅置空其分类）。
func (r *CategoryRepository) Delete(id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE apps SET category_id = 0 WHERE category_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM categories WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Slugify 生成安全的 slug（保留中英文字符与数字）。
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == ' ' || r == '-' || r == '_' || r == '.' || r == '/' {
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = fmt.Sprintf("app-%d", time.Now().Unix()%100000)
	}
	return out
}
