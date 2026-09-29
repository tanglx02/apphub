import io

p = 'internal/apps/repository.go'
s = io.open(p, encoding='utf-8').read()

# 1) appColumns 增加 scope
old = ''' check_timeout_sec, public_visible, created_at, updated_at`'''
new = ''' check_timeout_sec, public_visible, scope, created_at, updated_at`'''
assert old in s, 'appColumns'
s = s.replace(old, new, 1)

# 2) scanApp
old = '''	var a models.App
	var appType, statusType string
	var shell, publicVisible int
	err := rows.Scan(
		&a.ID, &a.Name, &a.Slug, &a.Description, &a.Icon, &a.CategoryID, &a.Tags,
		&a.InternalURL, &a.ExternalURL, &appType, &a.SystemdUnit, &a.StartCommand,
		&a.StopCommand, &a.RestartCommand, &shell, &a.StartArgs, &a.StopArgs,
		&a.RestartArgs, &statusType, &a.StatusTarget, &a.ExpectedCodes, &a.WorkDir,
		&a.Environment, &a.Enabled, &a.AutoStart, &a.Favorite, &a.SortOrder,
		&a.TimeoutSeconds, &a.CheckTimeoutSec, &publicVisible, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.AppType = models.AppType(appType)
	a.StatusType = models.StatusCheckType(statusType)
	a.ShellMode = shell == 1
	a.PublicVisible = publicVisible == 1
	return &a, nil'''
new = '''	var a models.App
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
	return &a, nil'''
assert old in s, 'scanApp'
s = s.replace(old, new, 1)

# 3) Create 列与参数
old = '''	 check_timeout_sec, public_visible, created_at, updated_at)
	 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,'''
new = '''	 check_timeout_sec, public_visible, scope, created_at, updated_at)
	 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,'''
assert old in s, 'Create cols'
s = s.replace(old, new, 1)

old = '''		a.SortOrder, a.TimeoutSeconds, a.CheckTimeoutSec, boolToInt(a.PublicVisible), a.CreatedAt, a.UpdatedAt)'''
new = '''		a.SortOrder, a.TimeoutSeconds, a.CheckTimeoutSec, boolToInt(a.PublicVisible),
		string(a.Scope), a.CreatedAt, a.UpdatedAt)'''
assert old in s, 'Create args'
s = s.replace(old, new, 1)

# 4) Update
old = '''	 favorite=?, sort_order=?, timeout_seconds=?, check_timeout_sec=?, public_visible=?, updated_at=? WHERE id=?`,'''
new = '''	 favorite=?, sort_order=?, timeout_seconds=?, check_timeout_sec=?, public_visible=?, scope=?, updated_at=? WHERE id=?`,'''
assert old in s, 'Update cols'
s = s.replace(old, new, 1)

old = '''		a.SortOrder, a.TimeoutSeconds, a.CheckTimeoutSec, boolToInt(a.PublicVisible), a.UpdatedAt, a.ID)'''
new = '''		a.SortOrder, a.TimeoutSeconds, a.CheckTimeoutSec, boolToInt(a.PublicVisible),
		string(a.Scope), a.UpdatedAt, a.ID)'''
assert old in s, 'Update args'
s = s.replace(old, new, 1)

io.open(p, 'w', encoding='utf-8').write(s)
print('repo ok')
