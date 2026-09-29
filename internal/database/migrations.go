package database

import "strings"

// Migration 描述一次数据库迁移。
type Migration struct {
	Name string
	SQL  string
}

// migrations 为有序迁移列表，索引 + 1 即 schema 版本。
// 规则：只允许追加，禁止修改已发布的迁移。
var migrations = []Migration{
	{
		Name: "initial_schema",
		SQL: `
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'admin',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME,
    last_login_ip TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    username   TEXT NOT NULL DEFAULT '',
    role       TEXT NOT NULL DEFAULT 'admin',
    ip         TEXT DEFAULT '',
    user_agent TEXT DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS categories (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    icon       TEXT DEFAULT '',
    color      TEXT DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS apps (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    description     TEXT DEFAULT '',
    icon            TEXT DEFAULT '',
    category_id     INTEGER DEFAULT 0,
    tags            TEXT DEFAULT '',
    internal_url    TEXT DEFAULT '',
    external_url    TEXT DEFAULT '',
    app_type        TEXT NOT NULL DEFAULT 'systemd',
    systemd_unit    TEXT DEFAULT '',
    start_command   TEXT DEFAULT '',
    stop_command    TEXT DEFAULT '',
    restart_command TEXT DEFAULT '',
    shell_mode      INTEGER NOT NULL DEFAULT 0,
    start_args      TEXT DEFAULT '',
    stop_args       TEXT DEFAULT '',
    restart_args    TEXT DEFAULT '',
    status_type     TEXT NOT NULL DEFAULT 'systemd',
    status_target   TEXT DEFAULT '',
    expected_codes  TEXT DEFAULT '',
    work_dir        TEXT DEFAULT '',
    environment     TEXT DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    auto_start      INTEGER NOT NULL DEFAULT 0,
    favorite        INTEGER NOT NULL DEFAULT 0,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    timeout_seconds INTEGER NOT NULL DEFAULT 60,
    check_timeout_sec INTEGER NOT NULL DEFAULT 5,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_apps_category ON apps(category_id);
CREATE INDEX IF NOT EXISTS idx_apps_sort ON apps(sort_order);
CREATE INDEX IF NOT EXISTS idx_apps_enabled ON apps(enabled);

CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL DEFAULT 0,
    username   TEXT DEFAULT '',
    ip         TEXT DEFAULT '',
    action     TEXT NOT NULL,
    action_cn  TEXT DEFAULT '',
    target     TEXT DEFAULT '',
    result     TEXT DEFAULT 'success',
    detail     TEXT DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);

CREATE TABLE IF NOT EXISTS backups (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    path       TEXT DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    note       TEXT DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_backups_created ON backups(created_at);

INSERT OR IGNORE INTO settings(key, value) VALUES
    ('site_name', 'AppHub'),
    ('logo', ''),
    ('default_category', ''),
    ('allow_batch_start', 'false'),
    ('refresh_interval', '5'),
    ('theme', 'system');
`,
	},
	{
		Name: "seed_default_categories",
		SQL: `
INSERT OR IGNORE INTO categories(name, slug, icon, color, sort_order) VALUES
    ('AI', 'ai', 'Sparkles', '#8B5CF6', 1),
    ('安全', 'security', 'Shield', '#EF4444', 2),
    ('监控', 'monitor', 'Activity', '#06B6D4', 3),
    ('运维', 'ops', 'Settings', '#64748B', 4),
    ('下载', 'download', 'Download', '#F59E0B', 5),
    ('密码管理', 'password', 'KeyRound', '#10B981', 6),
    ('开发', 'dev', 'Code', '#3B82F6', 7),
    ('数据库', 'database', 'Database', '#0EA5E9', 8),
    ('媒体', 'media', 'Film', '#EC4899', 9),
    ('网络', 'network', 'Network', '#14B8A6', 10),
    ('自动化', 'automation', 'Workflow', '#A855F7', 11),
    ('其他', 'other', 'LayoutGrid', '#94A3B8', 12);
`,
	},
	{
		Name: "public_navigation",
		SQL: `
-- 前台只读导航：应用级"在前台显示"开关（默认显示）
ALTER TABLE apps ADD COLUMN public_visible INTEGER NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_apps_public ON apps(public_visible);

-- 前台导航全局设置（后台可调）
INSERT OR IGNORE INTO settings(key, value) VALUES
    ('public_enabled', 'true'),
    ('public_title', '我的服务器'),
    ('public_subtitle', '应用导航'),
    ('public_show_resources', 'true'),
    ('public_show_categories', 'true'),
    ('public_show_search', 'true'),
    ('public_allow_favorite', 'true'),
    ('public_default_theme', 'system');
`,
	},
	{
		Name: "app_scope_and_endpoints",
		SQL: `
-- 应用范围类型：LOCAL（本机可管理）/ EXTERNAL（仅导航 + 在线检测）。
-- 现有应用全部视为 LOCAL，与历史行为一致，无需用户重新配置。
ALTER TABLE apps ADD COLUMN scope TEXT NOT NULL DEFAULT 'LOCAL';
CREATE INDEX IF NOT EXISTS idx_apps_scope ON apps(scope);

-- 附加访问入口：本地应用的 内网/公网 快捷入口仍由 internal_url / external_url 承载，
-- 本表用于 EXTERNAL 应用或需要更多入口的场景。
CREATE TABLE IF NOT EXISTS app_endpoints (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    app_id       INTEGER NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL,
    type         TEXT NOT NULL DEFAULT 'WEB',
    enabled      INTEGER NOT NULL DEFAULT 1,
    open_new_tab INTEGER NOT NULL DEFAULT 1,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    description  TEXT DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_endpoints_app ON app_endpoints(app_id);
`,
	},
}

// splitStatements 按分号切分 SQL（兼容语句内的字符串常量）。
func splitStatements(block string) []string {
	var stmts []string
	var cur strings.Builder
	inStr := false
	runes := []rune(block)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\'' && !inStr:
			inStr = true
			cur.WriteRune(r)
		case r == '\'' && inStr:
			// 处理 '' 转义
			if i+1 < len(runes) && runes[i+1] == '\'' {
				cur.WriteRune(r)
				cur.WriteRune(r)
				i++
				continue
			}
			inStr = false
			cur.WriteRune(r)
		case r == ';' && !inStr:
			stmts = append(stmts, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}
