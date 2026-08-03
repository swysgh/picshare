# picshare

轻量级产品图片展示程序。基于 Go 单二进制，无数据库。

## 特性

- 单二进制，复制即用（`CGO_ENABLED=0` 静态编译，~10MB）
- 无数据库、无 session
- 文件系统即数据：每个子目录 = 一个相册
- 自动识别封面（`cover.{jpg,png,webp}` > `folder.png` > 首张图）
- 缩略图懒生成 + 磁盘缓存（EXIF 自动旋转）
- PhotoSwipe 灯箱（触摸/键盘/全屏）
- 中/韩/英 三语界面
- 后台 HTTP Basic Auth（浏览器弹窗登录）
- 单文件上传、拖拽上传、流式上传（支持 >100MB）
- 简易后台文件管理器：建相册 / 上传 / 删除 / 重命名 / 设封面
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
├── 01.主力产品/                # 目录名前缀数字控制排序
│   ├── cover.jpg                # 封面（约定）
│   ├── description.txt          # 可选，相册描述
│   ├── 01.jpg
│   └── 02.jpg
└── 02.제품B/
    └── cover.jpg

thumbs/                          # 自动生成，可随时删除
config.json                      # 配置
picshare                         # 二进制
```

## URL 约定

- `/` & `/index.html` — 前台首页（相册列表）
- `/album/{name}` — 单相册页（灯箱浏览）
- `/admin/` & `/admin/album/{name}` — 后台文件管理器
- `/api/albums` — JSON: 相册列表
- `/api/albums/{name}` — JSON: 单相册详情
- `/thumb?p=/photos/...&w=480` — 缩略图（懒生成）
- `/photos/{album}/{file}` — 原图（推荐 Nginx 直接服务）

后台 API（Basic Auth）：
- `POST /api/admin/mkdir` — 建相册
- `POST /api/admin/upload?album=...` — 上传（multipart）
- `POST /api/admin/rename` — 重命名
- `POST /api/admin/delete` — 删除
- `POST /api/admin/setcover` — 设封面

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
make build    # 编译
make test     # 跑测试
make run      # 编译并运行（用 config.json）
make init     # 生成默认 config.json
make clean    # 清理二进制 + photos/thumbs
```

## 许可

MIT
