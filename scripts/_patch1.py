import io

# 1) 迁移 v4
p = 'internal/database/migrations.go'
s = io.open(p, encoding='utf-8').read()
old = "    ('public_default_theme', 'system');\n`,\n\t},\n}"
new = """    ('public_default_theme', 'system');
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
}"""
assert old in s, 'migrations tail'
s = s.replace(old, new, 1)
io.open(p, 'w', encoding='utf-8').write(s)
print('migration ok')
