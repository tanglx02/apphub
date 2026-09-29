package database

import (
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/models"
)

// AuditStore 审计日志存储。
type AuditStore struct {
	db *DB
}

// NewAuditStore 创建审计存储。
func NewAuditStore(db *DB) *AuditStore { return &AuditStore{db: db} }

// Insert 写入一条审计记录。
func (s *AuditStore) Insert(userID int64, username, ip string, action models.AuditAction, target, result, detail string) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_logs(user_id, username, ip, action, action_cn, target, result, detail, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, username, ip, string(action), action.Text(), target, result, detail, time.Now())
	return err
}

// List 分页查询审计记录。
func (s *AuditStore) List(limit, offset int, action, keyword string) ([]models.AuditLog, int, error) {
	where := "WHERE 1=1"
	args := make([]any, 0)
	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
	}
	if keyword != "" {
		where += " AND (username LIKE ? OR target LIKE ? OR ip LIKE ? OR detail LIKE ?)"
		kw := "%" + keyword + "%"
		args = append(args, kw, kw, kw, kw)
	}

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT id, user_id, username, ip, action, action_cn, target, result, detail, created_at
		 FROM audit_logs ` + where + ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.AuditLog, 0, limit)
	for rows.Next() {
		var l models.AuditLog
		var act string
		if err := rows.Scan(&l.ID, &l.UserID, &l.Username, &l.IP, &act, &l.ActionCN, &l.Target, &l.Result, &l.Detail, &l.CreatedAt); err != nil {
			continue
		}
		l.Action = models.AuditAction(act)
		if strings.TrimSpace(l.ActionCN) == "" {
			l.ActionCN = l.Action.Text()
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// Clear 清空审计日志。
func (s *AuditStore) Clear() error {
	_, err := s.db.Exec(`DELETE FROM audit_logs`)
	return err
}

// Prune 清理超过保留天数的审计日志。
func (s *AuditStore) Prune(days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	res, err := s.db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, time.Now().AddDate(0, 0, -days))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
