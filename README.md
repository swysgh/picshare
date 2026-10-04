# picshare

轻量级产品图片展示程序。基于 Go 单二进制，无数据库。

## 特性

- 单二进制，复制即用（`CGO_ENABLED=0` 静态编译，~7MB）
- 无数据库、无 session
- 文件系统即数据：每个子目录 = 一个相册，支持多级嵌套文件夹分类
- 自动识别封面（`cover.{jpg,png,webp}` > `folder.png` > 首张图）
- 缩略图懒生成 + 磁盘缓存（EXIF 自动旋转；源文件更新自动失效）
- PhotoSwipe 灯箱（触摸/键盘/全屏）
- 中/韩/英 三语界面
- 后台 HTTP Basic Auth（浏览器弹窗登录）
- 单文件上传、拖拽上传、流式上传（支持 >100MB）
- 后台文件管理器：建相册 / 上传 / 删除 / 重命名 / 设封面 / 拖拽排序 / 隐藏照片
- 相册手动排序：在后台拖拽子文件夹即可调整顺序，持久化到 `.order` 文件
- 照片隐藏：后台可将某张照片标记为隐藏，公开页不展示但仍可作封面
- 中文/韩文/日文文件名与 URL 全链路 UTF-8

## Lychee 对比

| 维度 | Lychee | picshare |
|---|---|---|
| 运行时 | PHP + MariaDB | Go 单二进制 |
| 内存 | ~200MB | ~20MB |
| 部署 | 2 服务 + 配置 | 1 JSON 文件 |
| 后台复杂度 | 用户/共享/标签/相册 | 浏览 + 上传 + 文件管理 |

## 快速开始

```bash
# 1. 编译
make build

# 2. 初始化配置
./picshare --config config.json --init

# 3. 编辑配置（至少修改 admin_pass）
vim config.json

# 4. 创建相册目录
mkdir -p photos/主力产品

# 5. 拖入图片 + 命名 cover.jpg 作为封面
#    photos/主力产品/cover.jpg
#    photos/主力产品/01.jpg
#    photos/主力产品/02.jpg

# 6. 启动
./picshare --config config.json

# 7. 访问
#    前台 http://localhost:8080/
#    后台 http://localhost:8080/admin/  （浏览器弹窗登录）
```

## 文件结构

```
photos/                          # 配置中的 photos_dir
├── 主力产品/
│   ├── cover.jpg                # 封面（约定）
│   ├── description.txt          # 可选，相册描述
│   ├── .order                   # 可选，子文件夹手动排序（后台拖拽生成）
│   ├── .hidden                  # 可选，本相册内被隐藏的照片名（后台切换）
│   ├── 手机/                    # 多级嵌套：子相册/子分类
│   │   ├── cover.jpg
│   │   └── 01.jpg
│   └── 平板/
│       └── cover.jpg
└── 제품B/
    └── cover.jpg

thumbs/                          # 自动生成，可随时删除
config.json                      # 配置
picshare                         # 二进制
```

每个目录既是相册（可含图片）也是分类（可含子目录），可无限嵌套。

## 排序规则

- **子文件夹（相册）**：默认按名称自然排序（数字感知，如 `01` 在 `1` 之前等同于 `1`）。在后台用拖拽调整顺序后，顺序写入该目录的 `.order` 文件，每行一个子文件夹名，之后前台按此顺序展示。
- **照片**：按文件 mtime **倒序**（最新上传的在前），mtime 相同的回退到自然排序。
- 想恢复默认排序：删除对应目录下的 `.order` 文件即可。

## 隐藏照片

- 后台照片卡片有"隐藏"按钮，点击后该照片写入相册目录的 `.hidden` 文件。
- 公开页面不再展示该照片，`photo_count` 也不再计入。
- 后台仍能看到（半透明 + "已隐藏"角标），可取消隐藏。
- **隐藏的照片仍可作为封面**：封面文件是 `cover.jpg`（独立文件），不受 `.hidden` 影响。

## URL 约定

- `/` & `/index.html` — 前台首页（顶层相册/分类列表）
- `/album/{path}` — 单相册页（路径可为多级，如 `/album/主力产品/手机`）
- `/admin/` & `/admin/album/{path}` — 后台文件管理器
- `/api/albums` — JSON: 顶层相册列表（含 `children` 递归子分类）
- `/api/albums/{path}` — JSON: 单相册详情（多级路径）
- `/thumb?p=/photos/...&w=480` — 缩略图（懒生成；带 `v` 参数做缓存破坏）
- `/photos/{path}/{file}` — 原图（推荐 Nginx 直接服务）

后台 API（Basic Auth）：
- `POST /api/admin/mkdir` — 建相册（`album` 传完整路径）
- `POST /api/admin/upload?album={path}` — 上传（multipart）
- `POST /api/admin/rename` — 重命名（文件或文件夹）
- `POST /api/admin/delete` — 删除（文件/子目录；`paths:["."]` 删除整个相册）
- `POST /api/admin/setcover` — 设封面
- `POST /api/admin/sethidden` — 切换照片隐藏状态（`{album, photo, hidden}`）
- `POST /api/admin/reorder` — 保存子文件夹手动顺序（`{album, names:[...]}`）

## 配置说明

```json
{
  "listen": "127.0.0.1:8080",       // 监听地址（建议走 Nginx 反代）
  "photos_dir": "./photos",         // 原始图片目录
  "thumbs_dir": "./thumbs",         // 缩略图缓存
  "admin_user": "admin",            // 后台账号
  "admin_pass": "change-me",        // 后台密码
  "site_title": "Product Gallery",  // 站名
  "default_lang": "zh",             // 初始语言 zh / ko / en
  "grid_thumb_size": 480,           // 网格缩略图最大边
  "lightbox_size": 1600,            // 灯箱图最大边
  "enable_exif_rotation": true,     // 按 EXIF 自动旋转
  "web_path_prefix": "/photos"      // 原图 URL 前缀（与 Nginx 配置一致）
}
```

## 支持的格式

JPEG / PNG / WebP / GIF / BMP / TIFF

**HEIC/HEIF 不支持**（需要 CGO，破坏单二进制）。如需支持请先用系统工具转 JPEG。

## 部署

参见 [DEPLOY.md](DEPLOY.md)。

## 命令

```bash
make build    # 编译（版本号自动从最近的 git tag 注入）
make test     # 跑测试
make run      # 编译并运行（用 config.json）
make init     # 生成默认 config.json
make clean    # 清理二进制 + photos/thumbs
```

## 发布

打 `v*` 开头的 tag 并 push，GitHub Actions 会自动跑测试、编译 linux/amd64 二进制并创建 Release：

```bash
git tag v0.0.2
git push origin v0.0.2
```

## 许可

MIT
