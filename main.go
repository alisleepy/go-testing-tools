// 测试工具集管理面板 - 单二进制服务
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/static
var staticFiles embed.FS

func main() {
	var (
		addr   string
		dbPath string
	)
	flag.StringVar(&addr, "addr", envOr("ADDR", ":8080"), "监听地址，如 :8080")
	flag.StringVar(&dbPath, "db", envOr("DB_PATH", defaultDBPath()), "SQLite 数据库文件路径")
	flag.Parse()

	store, err := OpenStore(dbPath)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer store.Close()

	sessions, err := NewSessionManager()
	if err != nil {
		log.Fatalf("会话管理初始化失败: %v", err)
	}

	staticFS, err := fs.Sub(staticFiles, "web/static")
	if err != nil {
		log.Fatalf("静态资源加载失败: %v", err)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequests(NewServer(store, sessions, staticFS, indexHTML).Routes()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("测试工具集管理面板已启动: http://localhost%s", addr)
		log.Printf("数据库文件: %s", dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("关闭异常: %v", err)
	}
	log.Println("服务已停止")
}

// logRequests 简单的访问日志中间件
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// defaultDBPath 默认在可执行文件同级目录生成 tools.db
func defaultDBPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "tools.db"
	}
	return filepath.Join(filepath.Dir(exe), "tools.db")
}
