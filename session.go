package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// 内置管理员账号（密码在启动时哈希，不明文比较）
const adminUsername = "alisleepy"

// adminPasswordHash 为 "xiaoyouzi!@#" 的 bcrypt 哈希，启动时生成
var adminPasswordHash []byte

const (
	sessionCookieName = "session_id"
	sessionTTL        = 12 * time.Hour
)

type session struct {
	username  string
	expiresAt time.Time
}

// SessionManager 内存会话管理器
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]session
}

// NewSessionManager 创建会话管理器并初始化管理员密码哈希
func NewSessionManager() (*SessionManager, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte("xiaoyouzi!@#"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	adminPasswordHash = hash
	sm := &SessionManager{sessions: make(map[string]session)}
	go sm.gcLoop()
	return sm, nil
}

// gcLoop 定期清理过期会话
func (sm *SessionManager) gcLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		sm.mu.Lock()
		for id, s := range sm.sessions {
			if now.After(s.expiresAt) {
				delete(sm.sessions, id)
			}
		}
		sm.mu.Unlock()
	}
}

// VerifyCredentials 校验账号密码
func VerifyCredentials(username, password string) bool {
	if username != adminUsername {
		return false
	}
	return bcrypt.CompareHashAndPassword(adminPasswordHash, []byte(password)) == nil
}

// Create 创建新会话并返回 session id
func (sm *SessionManager) Create(username string) (string, time.Time) {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	exp := time.Now().Add(sessionTTL)
	sm.mu.Lock()
	sm.sessions[id] = session{username: username, expiresAt: exp}
	sm.mu.Unlock()
	return id, exp
}

// Get 返回会话对应的用户名，第二个返回值表示会话是否有效
func (sm *SessionManager) Get(id string) (string, bool) {
	sm.mu.RLock()
	s, ok := sm.sessions[id]
	sm.mu.RUnlock()
	if !ok || time.Now().After(s.expiresAt) {
		if ok {
			sm.Delete(id)
		}
		return "", false
	}
	return s.username, true
}

// Delete 删除会话
func (sm *SessionManager) Delete(id string) {
	sm.mu.Lock()
	delete(sm.sessions, id)
	sm.mu.Unlock()
}

// userFromRequest 从请求 Cookie 中解析登录用户
func (sm *SessionManager) userFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	return sm.Get(c.Value)
}

// setSessionCookie 写入会话 Cookie
func setSessionCookie(w http.ResponseWriter, id string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie 清除会话 Cookie
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
