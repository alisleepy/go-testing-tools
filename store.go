package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Tool 对应数据库 tools 表的记录
type Tool struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Purpose   string `json:"purpose"`
	URL       string `json:"url"`
	Remark    string `json:"remark"`
	SortOrder int64  `json:"sort_order"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ErrNotFound 表示指定工具不存在
var ErrNotFound = errors.New("工具不存在")

// Store 封装数据库访问
type Store struct {
	db *sql.DB
}

// OpenStore 打开（或创建）SQLite 数据库并初始化表结构
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 建议限制并发写连接
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

// init 创建表结构（首次启动自动建表）
func (s *Store) init() error {
	schema := `
CREATE TABLE IF NOT EXISTS tools (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    purpose    TEXT NOT NULL,
    url        TEXT NOT NULL,
    remark     TEXT DEFAULT '',
    sort_order INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_tools_sort ON tools(sort_order);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化表结构失败: %w", err)
	}
	return nil
}

// Close 关闭数据库连接
func (s *Store) Close() error { return s.db.Close() }

// ListTools 返回所有工具，按 sort_order 升序，其次按 id 升序
func (s *Store) ListTools() ([]Tool, error) {
	rows, err := s.db.Query(`SELECT id, name, purpose, url, remark, sort_order, created_at, updated_at
		FROM tools ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tools := make([]Tool, 0)
	for rows.Next() {
		var t Tool
		if err := rows.Scan(&t.ID, &t.Name, &t.Purpose, &t.URL, &t.Remark, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, rows.Err()
}

// GetTool 按 id 查询单条记录
func (s *Store) GetTool(id int64) (*Tool, error) {
	var t Tool
	err := s.db.QueryRow(`SELECT id, name, purpose, url, remark, sort_order, created_at, updated_at
		FROM tools WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.Purpose, &t.URL, &t.Remark, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateTool 新增工具，sort_order 自动排到末尾
func (s *Store) CreateTool(t *Tool) (*Tool, error) {
	var maxOrder sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(sort_order) FROM tools`).Scan(&maxOrder); err != nil {
		return nil, err
	}
	nextOrder := int64(0)
	if maxOrder.Valid {
		nextOrder = maxOrder.Int64 + 1
	}
	res, err := s.db.Exec(`INSERT INTO tools (name, purpose, url, remark, sort_order)
		VALUES (?, ?, ?, ?, ?)`, t.Name, t.Purpose, t.URL, t.Remark, nextOrder)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetTool(id)
}

// UpdateTool 更新指定工具的可编辑字段
func (s *Store) UpdateTool(id int64, t *Tool) (*Tool, error) {
	res, err := s.db.Exec(`UPDATE tools SET name = ?, purpose = ?, url = ?, remark = ?, updated_at = ?
		WHERE id = ?`, t.Name, t.Purpose, t.URL, t.Remark, time.Now().Format("2006-01-02 15:04:05"), id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.GetTool(id)
}

// DeleteTool 删除指定工具
func (s *Store) DeleteTool(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tools WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateSort 按给定的 id 顺序批量更新 sort_order（事务保证一致性）
func (s *Store) UpdateSort(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE tools SET sort_order = ?, updated_at = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Format("2006-01-02 15:04:05")
	for order, id := range ids {
		if _, err := stmt.Exec(order, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
