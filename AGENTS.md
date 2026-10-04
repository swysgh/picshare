# AGENTS.md

## 项目简介

picshare：轻量级产品图片展示程序，Go 单二进制 + 文件系统作为数据源，无数据库。详见 [README.md](README.md)。

## 协作偏好

- **Commit message 使用中文**。已推送的历史保持原样，新提交一律用中文。
- Commit 风格：`<类型>: <一句话概述>`，必要时附正文 bullet 说明。常用类型：`feat` / `fix` / `ci` / `docs` / `refactor` / `test` / `chore`。
- 版本发布通过打 `v*` tag 触发 GitHub Actions 自动编译和 Release，无需手工上传二进制。

## 构建与测试

```bash
make build   # 编译（版本号自动从最近 git tag 注入）
make test    # 跑测试
make vet     # go vet
```

## 目录结构

- `main.go` — 入口、路由、SPA handler
- `internal/config` — 配置加载与校验
- `internal/gallery` — 文件系统扫描、相册/照片模型、`.order` 手动排序
- `internal/thumb` — 缩略图懒生成与磁盘缓存
- `internal/api` — 公开 API + 后台 API（Basic Auth）
- `internal/auth` — HTTP Basic Auth 中间件
- `web/` — 嵌入式静态资源（HTML/CSS/JS、PhotoSwipe）
- `photos/` / `thumbs/` — 运行期数据目录（不入库）

## 注意事项

- `.order` 文件存放子文件夹手动排序，每行一个名字；由 `POST /api/admin/reorder` 写入。
- `.hidden` 文件存放本相册内被隐藏的照片名；由 `POST /api/admin/sethidden` 写入。公开 API 自动过滤，后台可见，仍可作封面。
- 照片展示顺序按文件 mtime 倒序（最新上传优先）。
- 缩略图 URL 带 `&v=<mtime-ns>` 参数作为缓存破坏，源文件更新后浏览器会重新拉取。
- `web/web.go` 使用 `go:embed`，新增静态文件需要被 embed pattern 覆盖。
