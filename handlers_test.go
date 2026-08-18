package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// newTestServer 构建一套基于临时数据库的测试服务
func newTestServer(t *testing.T) (http.Handler, *Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	sm, err := NewSessionManager()
	if err != nil {
		t.Fatalf("初始化会话管理失败: %v", err)
	}
	staticFS, err := fs.Sub(staticFiles, "web/static")
	if err != nil {
		t.Fatalf("加载静态资源失败: %v", err)
	}
	return NewServer(store, sm, staticFS, indexHTML).Routes(), store
}

func doReq(t *testing.T, h http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		buf = bytes.NewReader(b)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// login 登录并返回会话 Cookie
func login(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	rec := doReq(t, h, http.MethodPost, "/api/login",
		map[string]string{"username": adminUsername, "password": "xiaoyouzi!@#"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("登录失败: status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("登录响应未返回会话 Cookie")
	return nil
}

func TestLoginWrongPassword(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doReq(t, h, http.MethodPost, "/api/login",
		map[string]string{"username": adminUsername, "password": "wrong"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，实际 %d", rec.Code)
	}
}

func TestCheckAuthFlow(t *testing.T) {
	h, _ := newTestServer(t)

	rec := doReq(t, h, http.MethodGet, "/api/check-auth", nil, nil)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["logged_in"] != false {
		t.Fatalf("未登录时 logged_in 应为 false，实际 %v", out["logged_in"])
	}

	c := login(t, h)
	rec = doReq(t, h, http.MethodGet, "/api/check-auth", nil, c)
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["logged_in"] != true || out["username"] != adminUsername {
		t.Fatalf("已登录时状态异常: %v", out)
	}

	// 退出后会话失效
	if rec := doReq(t, h, http.MethodPost, "/api/logout", nil, c); rec.Code != http.StatusOK {
		t.Fatalf("退出登录失败: %d", rec.Code)
	}
	rec = doReq(t, h, http.MethodGet, "/api/check-auth", nil, c)
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["logged_in"] != false {
		t.Fatalf("退出后应为未登录状态，实际 %v", out)
	}
}

func TestListToolsPublic(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doReq(t, h, http.MethodGet, "/api/tools", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("工具列表应公开可访问，实际 %d", rec.Code)
	}
	var tools []Tool
	if err := json.Unmarshal(rec.Body.Bytes(), &tools); err != nil {
		t.Fatalf("响应不是 JSON 数组: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("初始应为空列表，实际 %d 条", len(tools))
	}
}

func TestWriteAPIsRequireLogin(t *testing.T) {
	h, _ := newTestServer(t)
	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/tools", map[string]string{"name": "a", "purpose": "b", "url": "https://x.com"}},
		{http.MethodPut, "/api/tools/1", map[string]string{"name": "a", "purpose": "b", "url": "https://x.com"}},
		{http.MethodDelete, "/api/tools/1", nil},
		{http.MethodPut, "/api/tools/sort", map[string][]int64{"ids": {1}}},
		{http.MethodPost, "/api/logout", nil},
	}
	for _, c := range cases {
		rec := doReq(t, h, c.method, c.path, c.body, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s 未登录应返回 401，实际 %d", c.method, c.path, rec.Code)
		}
	}
}

func TestToolCRUD(t *testing.T) {
	h, _ := newTestServer(t)
	c := login(t, h)

	// 新增
	rec := doReq(t, h, http.MethodPost, "/api/tools", map[string]string{
		"name": "Charles", "purpose": "抓包工具", "url": "https://www.charlesproxy.com", "remark": "需配置证书",
	}, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("新增失败: %d %s", rec.Code, rec.Body.String())
	}
	var created Tool
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == 0 || created.Name != "Charles" {
		t.Fatalf("新增返回数据异常: %+v", created)
	}

	// 编辑
	rec = doReq(t, h, http.MethodPut, "/api/tools/"+itoa(created.ID), map[string]string{
		"name": "Charles Proxy", "purpose": "HTTP 抓包", "url": "https://www.charlesproxy.com/download", "remark": "",
	}, c)
	if rec.Code != http.StatusOK {
		t.Fatalf("编辑失败: %d %s", rec.Code, rec.Body.String())
	}
	var updated Tool
	json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Name != "Charles Proxy" || updated.Purpose != "HTTP 抓包" {
		t.Fatalf("编辑未生效: %+v", updated)
	}

	// 删除
	if rec := doReq(t, h, http.MethodDelete, "/api/tools/"+itoa(created.ID), nil, c); rec.Code != http.StatusOK {
		t.Fatalf("删除失败: %d", rec.Code)
	}

	// 删除后再删应 404
	if rec := doReq(t, h, http.MethodDelete, "/api/tools/"+itoa(created.ID), nil, c); rec.Code != http.StatusNotFound {
		t.Fatalf("删除不存在的工具应返回 404，实际 %d", rec.Code)
	}
}

func TestValidation(t *testing.T) {
	h, _ := newTestServer(t)
	c := login(t, h)

	bad := []map[string]string{
		{"name": "", "purpose": "p", "url": "https://a.com"},
		{"name": "n", "purpose": "", "url": "https://a.com"},
		{"name": "n", "purpose": "p", "url": ""},
		{"name": "n", "purpose": "p", "url": "not-a-url"},
		{"name": "n", "purpose": "p", "url": "ftp://a.com"},
	}
	for _, b := range bad {
		rec := doReq(t, h, http.MethodPost, "/api/tools", b, c)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("非法参数 %v 应返回 400，实际 %d", b, rec.Code)
		}
	}
}

func TestUpdateSort(t *testing.T) {
	h, store := newTestServer(t)
	c := login(t, h)

	var ids []int64
	for _, name := range []string{"A", "B", "C"} {
		rec := doReq(t, h, http.MethodPost, "/api/tools", map[string]string{
			"name": name, "purpose": "用途" + name, "url": "https://example.com/" + name,
		}, c)
		if rec.Code != http.StatusOK {
			t.Fatalf("新增 %s 失败: %s", name, rec.Body.String())
		}
		var tool Tool
		json.Unmarshal(rec.Body.Bytes(), &tool)
		ids = append(ids, tool.ID)
	}

	// 反转顺序
	reversed := []int64{ids[2], ids[1], ids[0]}
	if rec := doReq(t, h, http.MethodPut, "/api/tools/sort", map[string][]int64{"ids": reversed}, c); rec.Code != http.StatusOK {
		t.Fatalf("排序失败: %d %s", rec.Code, rec.Body.String())
	}

	tools, err := store.ListTools()
	if err != nil {
		t.Fatalf("读取列表失败: %v", err)
	}
	for i, want := range reversed {
		if tools[i].ID != want {
			t.Fatalf("排序结果不符，位置 %d 期望 id=%d，实际 id=%d", i, want, tools[i].ID)
		}
	}
}

func TestNotFoundTool(t *testing.T) {
	h, _ := newTestServer(t)
	c := login(t, h)
	rec := doReq(t, h, http.MethodPut, "/api/tools/99999", map[string]string{
		"name": "n", "purpose": "p", "url": "https://a.com",
	}, c)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在的工具 ID 应返回 404，实际 %d", rec.Code)
	}
}

func TestStaticAndIndex(t *testing.T) {
	h, _ := newTestServer(t)
	for _, path := range []string{"/", "/static/style.css", "/static/app.js"} {
		if rec := doReq(t, h, http.MethodGet, path, nil, nil); rec.Code != http.StatusOK {
			t.Errorf("%s 应可访问，实际 %d", path, rec.Code)
		}
	}
}

func itoa(v int64) string {
	return json.Number(jsonInt(v)).String()
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
