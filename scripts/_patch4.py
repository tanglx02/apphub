import io

p = 'internal/api/handlers_apps.go'
s = io.open(p, encoding='utf-8').read()

# 1) appPayload 增加 Type / Endpoints
old = '''	// PublicVisible 用指针区分"未传"与"显式关闭"：未传时默认在前台显示
	PublicVisible *bool `json:"public_visible"`
}'''
new = '''	// PublicVisible 用指针区分"未传"与"显式关闭"：未传时默认在前台显示
	PublicVisible *bool `json:"public_visible"`
	// Type 应用范围：LOCAL（默认）/ EXTERNAL
	Type      string            `json:"type"`
	Endpoints []models.Endpoint `json:"endpoints"`
}'''
assert old in s, 'payload'
s = s.replace(old, new, 1)

# 2) toApp 设置 Scope
old = '''		CheckTimeoutSec: p.CheckTimeoutSec,
		PublicVisible:   p.PublicVisible == nil || *p.PublicVisible,
	}
}'''
new = '''		CheckTimeoutSec: p.CheckTimeoutSec,
		PublicVisible:   p.PublicVisible == nil || *p.PublicVisible,
		Scope:           normalizeScope(p.Type),
	}
}

// normalizeScope 解析应用范围类型，空值默认 LOCAL。
func normalizeScope(t string) models.AppScope {
	if models.AppScope(strings.ToUpper(strings.TrimSpace(t))) == models.ScopeExternal {
		return models.ScopeExternal
	}
	return models.ScopeLocal
}

// saveEndpoints 用 payload 中的入口列表整体替换应用的附加入口。
func (s *Server) saveEndpoints(appID int64, eps []models.Endpoint) {
	if s.deps.Endpoints == nil || eps == nil {
		return
	}
	if err := s.deps.Endpoints.DeleteByApp(appID); err != nil {
		logError(nil, "清理应用入口失败 app=%d: %v", appID, err)
	}
	for i := range eps {
		e := eps[i]
		e.ID = 0
		e.AppID = appID
		e.URL = strings.TrimSpace(e.URL)
		if e.URL == "" {
			continue
		}
		if !strings.HasPrefix(e.URL, "http://") && !strings.HasPrefix(e.URL, "https://") {
			continue
		}
		if strings.TrimSpace(e.Name) == "" {
			e.Name = "入口"
		}
		if e.Type == "" {
			e.Type = "WEB"
		}
		e.OpenNewTab = true
		if err := s.deps.Endpoints.Create(&e); err != nil {
			logError(nil, "保存应用入口失败 app=%d: %v", appID, err)
		}
	}
}'''
assert old in s, 'toApp'
s = s.replace(old, new, 1)

# 3) validateApp：EXTERNAL 规则
old = '''func validateApp(a *models.App) *ErrHTTP {
	if a.Name == "" {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "应用名称不能为空")
	}'''
new = '''func validateApp(a *models.App) *ErrHTTP {
	if a.Scope == "" {
		a.Scope = models.ScopeLocal
	}
	if a.Name == "" {
		return NewErr(http.StatusBadRequest, CodeBadRequest, "应用名称不能为空")
	}'''
assert old in s, 'validate head'
s = s.replace(old, new, 1)

old = '''	if a.AppType == models.AppTypeSystemd {
		if _, err := systemd.ValidateUnit(a.SystemdUnit); err != nil {
			return NewErr(http.StatusBadRequest, CodeBadRequest, err.Error())
		}
	} else {
		if strings.TrimSpace(a.StartCommand) == "" {
			return NewErr(http.StatusBadRequest, CodeBadRequest, "Command 应用必须配置启动命令")
		}
	}'''
new = '''	if a.Scope == models.ScopeExternal {
		// 非本地应用：只需要访问地址 + 在线检测，所有管理配置一律忽略
		a.SystemdUnit = ""
		a.StartCommand = ""
		a.StopCommand = ""
		a.RestartCommand = ""
		a.StartArgs = ""
		a.StopArgs = ""
		a.RestartArgs = ""
		a.WorkDir = ""
		a.Environment = ""
		a.ShellMode = false
		a.AppType = ""
		a.AutoStart = false
		if a.ExternalURL == "" && a.InternalURL == "" {
			return NewErr(http.StatusBadRequest, CodeBadRequest, "非本地应用必须配置访问地址")
		}
		switch a.StatusType {
		case models.CheckHTTP, models.CheckTCP, models.CheckNone:
		case "":
			a.StatusType = models.CheckHTTP
		default:
			return NewErr(http.StatusBadRequest, CodeBadRequest, "非本地应用仅支持 http / tcp / none 检测方式")
		}
		if a.StatusTarget == "" && a.StatusType != models.CheckNone {
			a.StatusTarget = a.ExternalURL
		}
		return nil
	}
	if a.AppType == models.AppTypeSystemd {
		if _, err := systemd.ValidateUnit(a.SystemdUnit); err != nil {
			return NewErr(http.StatusBadRequest, CodeBadRequest, err.Error())
		}
	} else {
		if strings.TrimSpace(a.StartCommand) == "" {
			return NewErr(http.StatusBadRequest, CodeBadRequest, "Command 应用必须配置启动命令")
		}
	}'''
assert old in s, 'validate branch'
s = s.replace(old, new, 1)

# 4) Create / Update 后保存入口
old = '''	s.deps.Status.Refresh(a.ID)
	s.audit(models.AuditAppCreate, a.Name, "success", fmt.Sprintf("类型=%s", a.AppType), actorFrom(r))
	Created(w, r, a)'''
new = '''	if a.Scope == models.ScopeExternal {
		a.AppType = ""
	}
	s.saveEndpoints(a.ID, p.Endpoints)
	s.deps.Status.Refresh(a.ID)
	s.audit(models.AuditAppCreate, a.Name, "success", fmt.Sprintf("类型=%s", a.Scope.Text()), actorFrom(r))
	Created(w, r, a)'''
assert old in s, 'create save'
s = s.replace(old, new, 1)

old = '''	if err := s.deps.Repo.Update(a); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "更新应用失败", err))
		return
	}
	s.deps.Status.Refresh(id)
	s.audit(models.AuditAppUpdate, a.Name, "success", "", actorFrom(r))'''
new = '''	if a.Scope == models.ScopeExternal {
		a.AppType = ""
	}
	if err := s.deps.Repo.Update(a); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "更新应用失败", err))
		return
	}
	s.saveEndpoints(id, p.Endpoints)
	s.deps.Status.Refresh(id)
	s.audit(models.AuditAppUpdate, a.Name, "success", "", actorFrom(r))'''
assert old in s, 'update save'
s = s.replace(old, new, 1)

# 5) 删除应用级联清理入口
old = '''	if err := s.deps.Repo.Delete(id); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "删除应用失败", err))
		return
	}
	s.audit(models.AuditAppDelete, app.Name, "success", "仅删除 AppHub 中的应用定义", actorFrom(r))'''
new = '''	if err := s.deps.Repo.Delete(id); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "删除应用失败", err))
		return
	}
	if s.deps.Endpoints != nil {
		_ = s.deps.Endpoints.DeleteByApp(id)
	}
	s.audit(models.AuditAppDelete, app.Name, "success", "仅删除 AppHub 中的应用定义", actorFrom(r))'''
assert old in s, 'delete cascade'
s = s.replace(old, new, 1)

io.open(p, 'w', encoding='utf-8').write(s)
print('handlers ok')
