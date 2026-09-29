package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/tanglx02/apphub/internal/models"
	"github.com/tanglx02/apphub/internal/system"
)

// ============================================================
// Public API：前台只读导航专用。
//
// 设计约束：
//  1. 仅提供 GET 方法，全部匿名可访问（无会话要求）；
//  2. 返回值一律使用 PublicAppDTO 等公开视图，禁止直接返回数据库模型；
//  3. 绝不暴露 systemd 服务名、启停命令、工作目录、环境变量、PID、路径；
//  4. 不提供任何会改变服务器状态的能力。
// ============================================================

// publicConfigFrom 读取前台导航配置。
func (s *Server) publicConfigFrom() models.PublicConfig {
	st := s.deps.Settings
	title := strings.TrimSpace(st.Get("public_title", ""))
	if title == "" {
		title = s.siteName()
	}
	return models.PublicConfig{
		Enabled:        st.GetBool("public_enabled", true),
		Title:          title,
		Subtitle:       strings.TrimSpace(st.Get("public_subtitle", "应用导航")),
		ShowResources:  st.GetBool("public_show_resources", true),
		ShowCategories: st.GetBool("public_show_categories", true),
		ShowSearch:     st.GetBool("public_show_search", true),
		AllowFavorite:  st.GetBool("public_allow_favorite", true),
		DefaultTheme:   strings.TrimSpace(st.Get("public_default_theme", "system")),
	}
}

// publicDisabled 未启用前台时统一返回 403。
func (s *Server) publicDisabled(w http.ResponseWriter, r *http.Request) bool {
	if s.publicConfigFrom().Enabled {
		return false
	}
	writeJSON(w, r, http.StatusForbidden, ErrorPayload{Code: "PUBLIC_DISABLED", Message: "前台导航未启用"}, "前台导航未启用")
	return true
}

// buildPublicApps 组装公开应用 DTO（复用仓储与状态缓存，不复制数据逻辑）。
func (s *Server) buildPublicApps() []models.PublicApp {
	list, err := s.deps.Repo.List()
	if err != nil {
		return []models.PublicApp{}
	}
	cats, _ := s.deps.Cats.List()
	catMap := make(map[int64]models.Category, len(cats))
	for _, c := range cats {
		catMap[c.ID] = c
	}

	out := make([]models.PublicApp, 0, len(list))
	for i := range list {
		a := list[i]
		// 前台只显示：已启用 且 开启"在前台显示" 的应用
		if !a.Enabled || !a.PublicVisible {
			continue
		}
		dto := models.PublicApp{
			ID:          a.ID,
			Name:        a.Name,
			Type:        a.Scope,
			Description: a.Description,
			Icon:        a.Icon,
			Status:      s.deps.Status.Get(a.ID).Status,
			Endpoints:   s.publicEndpoints(&a),
			Tags:        a.Tags,
			SortOrder:   a.SortOrder,
		}
		if dto.Type == "" {
			dto.Type = models.ScopeLocal
		}
		if c, ok := catMap[a.CategoryID]; ok {
			dto.Category = c.Name
			dto.CategoryIcon = c.Icon
			dto.CategoryColor = c.Color
		}
		out = append(out, dto)
	}
	return out
}

// publicEndpoints 组装前台可见的访问入口：
//   - 本地应用：内网 / 公网 快捷入口（internal_url / external_url）+ 附加入口表；
//   - 非本地应用：访问地址（external_url）+ 附加入口表。
//
// 只返回名称与 URL，不含任何管理信息。
func (s *Server) publicEndpoints(app *models.App) []models.PublicEndpoint {
	out := make([]models.PublicEndpoint, 0, 3)
	add := func(name, url string, newTab bool) {
		if u := strings.TrimSpace(url); u != "" {
			out = append(out, models.PublicEndpoint{Name: name, URL: u, OpenNewTab: newTab})
		}
	}
	if app.Scope == models.ScopeExternal {
		add("访问", app.ExternalURL, true)
	} else {
		add("内网", app.InternalURL, false)
		add("公网", app.ExternalURL, true)
	}
	if s.deps.Endpoints != nil {
		if extras, err := s.deps.Endpoints.ListByApp(app.ID); err == nil {
			for _, e := range extras {
				if !e.Enabled {
					continue
				}
				add(e.Name, e.URL, e.OpenNewTab)
			}
		}
	}
	if len(out) == 0 {
		return []models.PublicEndpoint{}
	}
	return out
}

// buildPublicSummary 概览统计（可选附带系统资源）。
func (s *Server) buildPublicSummary(showResources bool) models.PublicSummary {
	apps := s.buildPublicApps()
	sum := models.PublicSummary{Total: len(apps), ServerUp: true, UpdatedAt: time.Now()}
	for _, a := range apps {
		if a.Type == models.ScopeExternal {
			sum.ExternalApps++
		} else {
			sum.LocalApps++
		}
		switch a.Status {
		case models.StatusOnline:
			sum.Online++
		case models.StatusOffline:
			sum.Offline++
		case models.StatusError:
			sum.Error++
		case models.StatusStarting, models.StatusStopping:
			sum.Starting++
		}
	}
	if sum.Total > 0 {
		sum.OnlineRate = sum.Online * 100 / sum.Total
	}
	if showResources {
		totalMB, usedMB := system.Memory()
		sum.CPU = round2(system.CPUPercent())
		sum.MemTotal = totalMB
		sum.MemUsed = usedMB
	}
	return sum
}

// publicCategories 仅返回前台可见应用实际使用的分类（与后台共用分类数据）。
func (s *Server) publicCategories() []models.Category {
	list, _ := s.deps.Repo.List()
	cats, _ := s.deps.Cats.List()
	catMap := make(map[int64]models.Category, len(cats))
	for _, c := range cats {
		catMap[c.ID] = c
	}
	used := make(map[int64]struct{}, len(cats))
	for i := range list {
		if list[i].Enabled && list[i].PublicVisible {
			if _, ok := catMap[list[i].CategoryID]; ok {
				used[list[i].CategoryID] = struct{}{}
			}
		}
	}
	out := make([]models.Category, 0, len(cats))
	for _, c := range cats {
		if _, ok := used[c.ID]; ok {
			out = append(out, c)
		}
	}
	return out
}

// handlePublicConfig 前台全局配置（始终可访问，便于前台判断是否启用）。
func (s *Server) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	OK(w, r, s.publicConfigFrom())
}

// handlePublicApps 公开应用列表（只读）。
func (s *Server) handlePublicApps(w http.ResponseWriter, r *http.Request) {
	if s.publicDisabled(w, r) {
		return
	}
	OK(w, r, s.buildPublicApps())
}

// handlePublicCategories 公开分类列表。
func (s *Server) handlePublicCategories(w http.ResponseWriter, r *http.Request) {
	if s.publicDisabled(w, r) {
		return
	}
	OK(w, r, s.publicCategories())
}

// handlePublicSummary 概览统计。
func (s *Server) handlePublicSummary(w http.ResponseWriter, r *http.Request) {
	if s.publicDisabled(w, r) {
		return
	}
	OK(w, r, s.buildPublicSummary(s.publicConfigFrom().ShowResources))
}

// handlePublicOverview 一次请求返回前台所需的全部数据（推荐前台轮询使用，降低请求数）。
func (s *Server) handlePublicOverview(w http.ResponseWriter, r *http.Request) {
	cfg := s.publicConfigFrom()
	if !cfg.Enabled {
		writeJSON(w, r, http.StatusForbidden, ErrorPayload{Code: "PUBLIC_DISABLED", Message: "前台导航未启用"}, "前台导航未启用")
		return
	}
	OK(w, r, map[string]any{
		"config":     cfg,
		"summary":    s.buildPublicSummary(cfg.ShowResources),
		"apps":       s.buildPublicApps(),
		"categories": s.publicCategories(),
	})
}
