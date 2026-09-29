package apps

import (
	"database/sql"
	"errors"

	"github.com/tanglx02/apphub/internal/models"
)

// ErrEndpointNotFound 附加入口不存在。
var ErrEndpointNotFound = errors.New("访问入口不存在")

// EndpointRepository 应用附加访问入口存储。
type EndpointRepository struct {
	db *sql.DB
}

// NewEndpointRepository 创建入口仓储。
func NewEndpointRepository(db *sql.DB) *EndpointRepository {
	return &EndpointRepository{db: db}
}

// ListByApp 返回应用的全部附加入口（按排序字段）。
func (r *EndpointRepository) ListByApp(appID int64) ([]models.Endpoint, error) {
	rows, err := r.db.Query(
		`SELECT id, app_id, name, url, type, enabled, open_new_tab, sort_order, description
		 FROM app_endpoints WHERE app_id = ? ORDER BY sort_order ASC, id ASC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Endpoint, 0, 4)
	for rows.Next() {
		var e models.Endpoint
		var enabled, newTab int
		if err := rows.Scan(&e.ID, &e.AppID, &e.Name, &e.URL, &e.Type, &enabled, &newTab, &e.SortOrder, &e.Description); err != nil {
			return nil, err
		}
		e.Enabled = enabled == 1
		e.OpenNewTab = newTab == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// Get 按 ID 查询单个入口。
func (r *EndpointRepository) Get(id int64) (*models.Endpoint, error) {
	var e models.Endpoint
	var enabled, newTab int
	err := r.db.QueryRow(
		`SELECT id, app_id, name, url, type, enabled, open_new_tab, sort_order, description
		 FROM app_endpoints WHERE id = ?`, id).
		Scan(&e.ID, &e.AppID, &e.Name, &e.URL, &e.Type, &enabled, &newTab, &e.SortOrder, &e.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEndpointNotFound
	}
	if err != nil {
		return nil, err
	}
	e.Enabled = enabled == 1
	e.OpenNewTab = newTab == 1
	return &e, nil
}

// Create 新增入口。
func (r *EndpointRepository) Create(e *models.Endpoint) error {
	res, err := r.db.Exec(
		`INSERT INTO app_endpoints(app_id, name, url, type, enabled, open_new_tab, sort_order, description)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		e.AppID, e.Name, e.URL, e.Type, boolToInt(e.Enabled), boolToInt(e.OpenNewTab),
		e.SortOrder, e.Description)
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// Update 更新入口。
func (r *EndpointRepository) Update(e *models.Endpoint) error {
	res, err := r.db.Exec(
		`UPDATE app_endpoints SET name=?, url=?, type=?, enabled=?, open_new_tab=?, sort_order=?, description=?
		 WHERE id=?`,
		e.Name, e.URL, e.Type, boolToInt(e.Enabled), boolToInt(e.OpenNewTab),
		e.SortOrder, e.Description, e.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEndpointNotFound
	}
	return nil
}

// Delete 删除入口。
func (r *EndpointRepository) Delete(id int64) error {
	res, err := r.db.Exec(`DELETE FROM app_endpoints WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEndpointNotFound
	}
	return nil
}

// DeleteByApp 删除应用的全部入口（应用删除时级联清理）。
func (r *EndpointRepository) DeleteByApp(appID int64) error {
	_, err := r.db.Exec(`DELETE FROM app_endpoints WHERE app_id = ?`, appID)
	return err
}
