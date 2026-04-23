# Snemc Blog

一个全栈博客项目，使用 `Go + Vue3 + SQLite` 实现。

## Highlights

- 公开站点由 Go 输出页面，文章正文在保存时预编译为安全 HTML
- 后台使用 Vue3，支持单管理员登录、文章编辑、评论审核、分类标签管理
- Markdown 支持代码高亮、公式渲染、Mermaid 按需增强与内置图床
- SQLite 启用 `WAL`，全文搜索基于 `FTS5`
- 匿名访客使用 `visitor_id + 浏览器指纹` 做轻量个性化与风控
- 评论先入库待审核，再异步邮件通知；预留 AI 审核接口

## Run

1. 安装前端依赖并构建

```bash
cd frontend
npm install
npm run build
```

2. 启动服务

```bash
cd ..
go run ./cmd/server
```

默认地址：`http://localhost:8080`

默认后台账号：

- 用户名：`admin`
- 密码：`ChangeMe123!`

## Optional Environment Variables

- `BLOG_ADDR`
- `BLOG_MEDIA_DIR`
- `BLOG_SITE_URL`
- `BLOG_JWT_SECRET`
- `BLOG_ADMIN_USERNAME`
- `BLOG_ADMIN_PASSWORD`
- `BLOG_COMMENT_NOTIFY_TO`
- `BLOG_SMTP_HOST`
- `BLOG_SMTP_PORT`
- `BLOG_SMTP_USERNAME`
- `BLOG_SMTP_PASSWORD`
- `BLOG_SMTP_FROM`

## Structure

- `cmd/server`: Go 入口
- `internal/server`: 路由、页面处理、管理接口
- `internal/store`: SQLite 数据访问和业务主链路
- `internal/render`: Markdown 渲染与清洗
- `frontend`: Vue3 管理端与前台增强组件
- `web/templates`: 公开页面模板
- `web/assets`: 公开站点样式

## Demo Content

首次启动会自动创建：

- 默认管理员账号
- 示例分类、标签和一篇演示 Markdown 能力的文章
