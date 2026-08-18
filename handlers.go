package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Server 组装 HTTP 处理逻辑
type Server struct {
	store    *Store
	sessions *SessionManager
	static   fs.FS
	index    []byte
}

// NewServer 构造 Server
func NewServer(store *Store, sm *SessionManager, static fs.FS, indexHTML []byte) *Server {
	return &Server{store: store, sessions: sm, static: static, index: indexHTML}
}

// Routes 注册路由并返回 Handler
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// 静态资源
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(s.static))))

	// API
	mux.HandleFunc("/api/login", s.methodMux(map[string]http.HandlerFunc{
		http.MethodPost: s.handleLogin,
	}))
	mux.HandleFunc("/api/logout", s.methodMux(map[string]http.HandlerFunc{
		http.MethodPost: s.requireLogin(s.handleLogout),
	}))
	mux.HandleFunc("/api/check-auth", s.methodMux(map[string]http.HandlerFunc{
		http.MethodGet: s.handleCheckAuth,
	}))
	mux.HandleFunc("/api/tools/sort", s.methodMux(map[string]http.HandlerFunc{
		http.MethodPut: s.requireLogin(s.handleUpdateSort),
	}))
	mux.HandleFunc("/api/tools", s.methodMux(map[string]http.HandlerFunc{
		http.MethodGet:  s.handleListTools,
		http.MethodPost: s.requireLogin(s.handleCreateTool),
	}))
	mux.HandleFunc("/api/tools/", s.handleToolByID) // PUT/DELETE /api/tools/{id}

	// 前端页面（其它路径回退到 index.html，支持刷新）
	mux.HandleFunc("/", s.handleIndex)

	return mux
}

// -------- helpers --------

func (s *Server) methodMux(m map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h, ok := m[r.Method]
		if !ok {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "方法不允许"})
			return
		}
		h(w, r)
	}
}

func (s *Server) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.sessions.userFromRequest(r); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "请先登录"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("请求体为空")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// -------- 前端页面 --------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// 非 API/静态资源，一律返回 index.html
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/static/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.index)
}

// -------- 登录 / 会话 --------

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数错误"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "账号和密码不能为空"})
		return
	}
	if !VerifyCredentials(req.Username, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "账号或密码错误"})
		return
	}
	id, exp := s.sessions.Create(req.Username)
	setSessionCookie(w, id, exp)
	writeJSON(w, http.StatusOK, map[string]any{"username": req.Username})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.Delete(c.Value)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "已退出登录"})
}

func (s *Server) handleCheckAuth(w http.ResponseWriter, r *http.Request) {
	user, ok := s.sessions.userFromRequest(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"logged_in": ok,
		"username":  user,
	})
}

// -------- 工具 CRUD --------

type toolRequest struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	URL     string `json:"url"`
	Remark  string `json:"remark"`
}

var urlPattern = regexp.MustCompile(`^https?://`)

func validateToolPayload(p *toolRequest) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Purpose = strings.TrimSpace(p.Purpose)
	p.URL = strings.TrimSpace(p.URL)
	p.Remark = strings.TrimSpace(p.Remark)
	if p.Name == "" {
		return errors.New("工具名称不能为空")
	}
	if p.Purpose == "" {
		return errors.New("工具作用不能为空")
	}
	if p.URL == "" {
		return errors.New("工具地址不能为空")
	}
	if !urlPattern.MatchString(p.URL) {
		return errors.New("请输入有效的URL地址（需以 http:// 或 https:// 开头）")
	}
	if _, err := url.ParseRequestURI(p.URL); err != nil {
		return errors.New("请输入有效的URL地址")
	}
	return nil
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools, err := s.store.ListTools()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "系统异常，请稍后重试"})
		return
	}
	writeJSON(w, http.StatusOK, tools)
}

func (s *Server) handleCreateTool(w http.ResponseWriter, r *http.Request) {
	var req toolRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数错误"})
		return
	}
	if err := validateToolPayload(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.store.CreateTool(&Tool{Name: req.Name, Purpose: req.Purpose, URL: req.URL, Remark: req.Remark})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "系统异常，请稍后重试"})
		return
	}
	writeJSON(w, http.StatusOK, created)
}

// handleToolByID 处理 /api/tools/{id} 的 PUT / DELETE
func (s *Server) handleToolByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tools/")
	if idStr == "" || strings.Contains(idStr, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "路径不存在"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "非法的工具ID"})
		return
	}

	// 需要登录
	if _, ok := s.sessions.userFromRequest(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "请先登录"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.updateTool(w, r, id)
	case http.MethodDelete:
		s.deleteTool(w, r, id)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "方法不允许"})
	}
}

func (s *Server) updateTool(w http.ResponseWriter, r *http.Request, id int64) {
	var req toolRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数错误"})
		return
	}
	if err := validateToolPayload(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, err := s.store.UpdateTool(id, &Tool{Name: req.Name, Purpose: req.Purpose, URL: req.URL, Remark: req.Remark})
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "工具不存在"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "系统异常，请稍后重试"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteTool(w http.ResponseWriter, _ *http.Request, id int64) {
	err := s.store.DeleteTool(id)
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "工具不存在"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "系统异常，请稍后重试"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "删除成功"})
}

// -------- 排序 --------

type sortRequest struct {
	IDs []int64 `json:"ids"`
}

func (s *Server) handleUpdateSort(w http.ResponseWriter, r *http.Request) {
	var req sortRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求参数错误"})
		return
	}
	if len(req.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "排序列表不能为空"})
		return
	}
	if err := s.store.UpdateSort(req.IDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "系统异常，请稍后重试"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "排序已更新"})
}
