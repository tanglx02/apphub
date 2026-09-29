import io

p = 'internal/apps/service.go'
s = io.open(p, encoding='utf-8').read()

# 1) 新增守卫
old = '''// ErrNoCommand 未配置对应命令。
var ErrNoCommand = errors.New("未配置该操作所需的命令")'''
new = '''// ErrNoCommand 未配置对应命令。
var ErrNoCommand = errors.New("未配置该操作所需的命令")

// ErrExternalControl 非本地应用不支持服务控制。
var ErrExternalControl = errors.New("非本地应用不支持服务控制")

// ensureLocal 服务控制仅允许本地应用；EXTERNAL 只做导航与在线检测。
func ensureLocal(app *models.App) error {
	if app != nil && app.Scope == models.ScopeExternal {
		return ErrExternalControl
	}
	return nil
}'''
assert old in s, 'guard def'
s = s.replace(old, new, 1)

# 2) Start
old = '''// Start 启动应用。
func (s *Service) Start(ctx context.Context, id int64, actor Actor) error {
	app, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	lock := s.lockFor(id)'''
new = '''// Start 启动应用。
func (s *Service) Start(ctx context.Context, id int64, actor Actor) error {
	app, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if err := ensureLocal(app); err != nil {
		return err
	}
	lock := s.lockFor(id)'''
assert old in s, 'Start'
s = s.replace(old, new, 1)

io.open(p, 'w', encoding='utf-8').write(s)
print('ok')
