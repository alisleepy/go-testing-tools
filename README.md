# 测试工具集管理面板

一个面向 APP 测试工程师的轻量级工具管理面板：把散落在书签、文档、聊天记录里的测试工具地址统一收纳、增删改查、拖动排序。

- 后端：Go + `net/http` + SQLite（`modernc.org/sqlite`，纯 Go 免 CGO）
- 前端：HTML + CSS + 原生 JS 单页面
- 部署：`embed` 打包，**单二进制文件**，无外部依赖

---

## 目录结构

```
go-testing-tools/
├── main.go                 # 入口，参数解析、embed 静态资源、优雅退出
├── store.go                # SQLite 存储层（Tool 模型 + CRUD + 排序）
├── session.go              # 内置管理员账户、bcrypt、内存 Session
├── handlers.go             # HTTP 路由与 Handler
├── handlers_test.go        # API 集成测试
├── web/
│   ├── index.html          # 单页应用主模板
│   └── static/
│       ├── style.css       # 样式
│       └── app.js          # 前端逻辑（Tab、CRUD、拖动排序、登录）
├── go.mod / go.sum
└── dist/                   # 交叉编译产物（README 中说明）
```

---

## 快速开始

### 1. 从源码启动

```bash
# 首次构建
go build -o go-testing-tools .

# 运行（默认监听 :8080，数据库文件生成在可执行文件同级目录）
./go-testing-tools
```

浏览器访问：<http://localhost:8080>

### 2. 使用预编译二进制

`dist/` 目录下提供三平台预编译产物：

| 平台 | 文件 |
|------|------|
| Linux (amd64) | `go-testing-tools-linux-amd64` |
| macOS (Intel) | `go-testing-tools-darwin-amd64` |
| macOS (Apple Silicon) | `go-testing-tools-darwin-arm64` |
| Windows (amd64) | `go-testing-tools-windows-amd64.exe` |

直接执行即可，例如：

```bash
./dist/go-testing-tools-linux-amd64
```

---

## 启动参数

| 参数 | 环境变量 | 默认值 | 说明 |
|------|----------|--------|------|
| `-addr` | `ADDR` | `:8080` | HTTP 监听地址 |
| `-db`   | `DB_PATH` | 可执行文件同级目录下的 `tools.db` | SQLite 数据库文件路径 |

示例：

```bash
./go-testing-tools -addr :9000 -db /var/lib/tools.db
# 或使用环境变量
ADDR=:9000 DB_PATH=/var/lib/tools.db ./go-testing-tools
```

---

## 登录信息

面板内置**唯一管理员账户**，不支持注册：

| 字段 | 值 |
|------|----|
| 账号 | `alisleepy` |
| 密码 | `xiaoyouzi!@#` |

- 密码在进程内以 **bcrypt** 哈希存储，不落库、不明文比较。
- 登录成功后颁发 `HttpOnly` Session Cookie（有效期 12 小时），刷新页面保持登录。
- 页面右上角提供「退出登录」。

---

## 功能一览

### 工具集列表（公开只读）
- 表格展示：序号、工具名称、工具作用、工具地址（可点击）、备注。
- 无需登录，任何人可访问。

### 工具管理（登录可见）
- ✅ 新增工具（弹窗表单，含 URL 格式前后端双校验）
- ✅ 编辑工具（弹窗预填当前数据）
- ✅ 删除工具（二次确认，提示工具名称）
- ✅ 拖动 ID 列上下排序，松开自动持久化 `sort_order`
- ✅ 401 会话过期自动感知，提示「会话已过期，请重新登录」

---

## 数据模型

数据库首次启动时会自动建表：

```sql
CREATE TABLE tools (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    purpose    TEXT NOT NULL,
    url        TEXT NOT NULL,
    remark     TEXT DEFAULT '',
    sort_order INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_tools_sort ON tools(sort_order);
```

---

## API 一览

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| POST | `/api/login` | 公开 | 登录，body: `{username,password}` |
| POST | `/api/logout` | 需登录 | 退出登录 |
| GET | `/api/check-auth` | - | 返回 `{logged_in,username}` |
| GET | `/api/tools` | 公开 | 列表，按 `sort_order` 升序 |
| POST | `/api/tools` | 需登录 | 新增，body: `{name,purpose,url,remark?}` |
| PUT | `/api/tools/{id}` | 需登录 | 编辑 |
| DELETE | `/api/tools/{id}` | 需登录 | 删除 |
| PUT | `/api/tools/sort` | 需登录 | 批量排序，body: `{ids:[id1,id2,...]}` |

错误响应统一为 `{"error":"...错误信息..."}`。

---

## 开发与测试

```bash
# 单元/集成测试（用临时 SQLite 数据库，不会污染真实数据）
go test ./...

# 静态检查
go vet ./...
```

覆盖场景：登录/退出/会话检查、公开列表、写操作鉴权、CRUD、参数校验、排序、404、静态资源。

---

## 交叉编译

```bash
mkdir -p dist
GOOS=linux   GOARCH=amd64 go build -o dist/go-testing-tools-linux-amd64        .
GOOS=darwin  GOARCH=arm64 go build -o dist/go-testing-tools-darwin-arm64      .
GOOS=darwin  GOARCH=amd64 go build -o dist/go-testing-tools-darwin-amd64      .
GOOS=windows GOARCH=amd64 go build -o dist/go-testing-tools-windows-amd64.exe .
```

因 SQLite 使用纯 Go 的 `modernc.org/sqlite`，**无需 CGO、无需交叉编译器**。

---

## 常见问题

**Q: 数据库文件在哪里？**
A: 默认与可执行文件同目录下的 `tools.db`；可用 `-db` 参数或 `DB_PATH` 环境变量指定。

**Q: 忘记管理员密码怎么办？**
A: 账号密码是硬编码在源码中（`session.go` 中 `xiaoyouzi!@#`）。如需修改，直接改代码后重新编译。

**Q: 想部署到内网服务器？**
A: 把对应平台的二进制文件传上去，`nohup ./go-testing-tools -addr :8080 &` 即可，不需要装任何依赖。

---

## 联系

使用问题请联系 **wangkaikai01**。
