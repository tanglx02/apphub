package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tanglx02/apphub/internal/apps"
	"github.com/tanglx02/apphub/internal/models"
)

// 附加访问入口（附加入口）管理：仅管理员；LOCAL/EXTERNAL 应用均可配置扩展入口。

// handleListEndpoints 应用附加入口列表。
func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	if s.deps.Endpoints == nil {
		OK(w, r, []models.Endpoint{})
		return
	}
	list, err := s.deps.Endpoints.ListByApp(id)
	if err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取入口失败", err))
		return
	}
	OK(w, r, list)
}

type endpointPayload struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Type        string `json:"type"`
	Enabled     *bool  `json:"enabled"`
	OpenNewTab  *bool  `json:"open_new_tab"`
	SortOrder   int    `json:"sort_order"`
	Description string `json:"description"`
}

func (p endpointPayload) toEndpoint(appID int64) (*models.Endpoint, *ErrHTTP) {
	u := strings.TrimSpace(p.URL)
	if u == "" {
		return nil, NewErr(http.StatusBadRequest, CodeBadRequest, "入口地址不能为空")
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return nil, NewErr(http.StatusBadRequest, CodeBadRequest, "入口地址必须以 http:// 或 https:// 开头")
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "入口"
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	newTab := true
	if p.OpenNewTab != nil {
		newTab = *p.OpenNewTab
	}
	t := strings.TrimSpace(p.Type)
	if t == "" {
		t = "WEB"
	}
	return &models.Endpoint{
		AppID:       appID,
		Name:        name,
		URL:         u,
		Type:        t,
		Enabled:     enabled,
		OpenNewTab:  newTab,
		SortOrder:   p.SortOrder,
		Description: strings.TrimSpace(p.Description),
	}, nil
}

// handleCreateEndpoint 新增附加入口。
func (s *Server) handleCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	if _, err := s.deps.Repo.Get(id); err != nil {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "应用不存在")
		return
	}
	var p endpointPayload
	if err := decodeJSON(r, &p); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	e, vErr := p.toEndpoint(int64(id))
	if vErr != nil {
		Fail(w, r, vErr.Status, vErr.Code, vErr.Message)
		return
	}
	if err := s.deps.Endpoints.Create(e); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "保存入口失败", err))
		return
	}
	Created(w, r, e)
}

// handleUpdateEndpoint 更新附加入口。
func (s *Server) handleUpdateEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	eid, err := pathID2(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "入口 ID 非法")
		return
	}
	existing, err := s.deps.Endpoints.Get(eid)
	if err != nil {
		if errors.Is(err, apps.ErrEndpointNotFound) {
			Fail(w, r, http.StatusNotFound, CodeNotFound, "入口不存在")
			return
		}
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "读取入口失败", err))
		return
	}
	if existing.AppID != id {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "入口不存在")
		return
	}
	var p endpointPayload
	if err := decodeJSON(r, &p); err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求参数有误")
		return
	}
	e, vErr := p.toEndpoint(int64(id))
	if vErr != nil {
		Fail(w, r, vErr.Status, vErr.Code, vErr.Message)
		return
	}
	e.ID = eid
	if err := s.deps.Endpoints.Update(e); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "更新入口失败", err))
		return
	}
	OK(w, r, e)
}

// handleDeleteEndpoint 删除附加入口。
func (s *Server) handleDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "应用 ID 非法")
		return
	}
	eid, err := pathID2(r)
	if err != nil {
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "入口 ID 非法")
		return
	}
	existing, err := s.deps.Endpoints.Get(eid)
	if err != nil || existing.AppID != id {
		Fail(w, r, http.StatusNotFound, CodeNotFound, "入口不存在")
		return
	}
	if err := s.deps.Endpoints.Delete(eid); err != nil {
		FailErr(w, r, WrapErr(http.StatusInternalServerError, CodeInternal, "删除入口失败", err))
		return
	}
	OK(w, r, map[string]any{"ok": true})
}

// pathID2 解析路径参数中的入口 ID。
func pathID2(r *http.Request) (int64, error) {
	raw := r.PathValue("eid")
	if raw == "" {
		return 0, errors.New("missing eid")
	}
	return strconv.ParseInt(raw, 10, 64)
}
