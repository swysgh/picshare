# 部署指南

## 架构概览

```
浏览器 (HTTPS)
   │
   ▼
Nginx (TLS + 静态直出 + 反代)
   ├── /photos/        → 直接读文件系统（零开销）
   ├── /thumbs/        → 直接读缓存（可选）
   └── 其余             → Go :8080
                          │
                          ├── 目录扫描 / 封面识别
                          ├── 缩略图懒生成
                          └── Basic Auth 后台
```

## 1. 编译

在任意 Linux 机器上：

```bash
make build
# 产出：picshare（~10MB 静态二进制）
file picshare  # 应显示 ELF 64-bit LSB executable, statically linked
```

如需 ARM64 NAS：

```bash
GOOS=linux GOARCH=arm64 make build
```

## 2. 在 NAS 上安装

### 2.1 准备目录

```bash
sudo useradd -r -s /usr/sbin/nologin picshare
sudo mkdir -p /var/lib/picshare/{photos,thumbs}
sudo chown -R picshare:picshare /var/lib/picshare
```

### 2.2 安装二进制

```bash
sudo install -m 755 picshare /usr/local/bin/picshare
```

### 2.3 配置

```bash
sudo tee /etc/picshare.json <<EOF
{
  "listen": "127.0.0.1:8080",
  "photos_dir": "/var/lib/picshare/photos",
  "thumbs_dir": "/var/lib/picshare/thumbs",
  "admin_user": "admin",
  "admin_pass": "$(openssl rand -base64 16)",
  "site_title": "Product Gallery",
  "default_lang": "zh",
  "grid_thumb_size": 480,
  "lightbox_size": 1600,
  "enable_exif_rotation": true,
  "web_path_prefix": "/photos"
}
EOF
sudo chmod 600 /etc/picshare.json
```

**请保存 `admin_pass`！** 这是后台登录密码。

### 2.4 systemd

```bash
sudo cp contrib/picshare.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now picshare
sudo systemctl status picshare
```

## 3. Nginx 配置

### 3.1 基础（HTTP）

```bash
sudo cp contrib/nginx.conf.example /etc/nginx/sites-available/picshare.conf
sudo ln -s /etc/nginx/sites-available/picshare.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

### 3.2 加 HTTPS（推荐）

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d gallery.example.com
```

certbot 会自动修改配置。

### 3.3 调整性能（如需）

```nginx
# 处理大量缩略图请求时
location /thumb {
    proxy_pass http://127.0.0.1:8080;
    proxy_cache_valid 200 30d;
    add_header X-Cache-Status $upstream_cache_status;
}
# （需要在 http 块先定义 proxy_cache_path）
```

## 4. 防火墙

```bash
# 仅暴露 80/443
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
# 注意：8080 不应暴露，仅供 Nginx 访问
```

## 5. 日常使用

### 5.1 上传图片

**方式 1：后台（推荐）**
1. 访问 `https://gallery.example.com/admin/`
2. 浏览器弹窗登录（输入 admin_user / admin_pass）
3. 点击【+ 新建相册】
4. 进入相册，拖拽图片到上传区

**方式 2：scp/cp**
```bash
# 直接传到相册目录
scp *.jpg nas:/var/lib/picshare/photos/主力产品/
# 刷新网页即可看到
```

### 5.2 设置封面

将图片命名为 `cover.jpg`（或 `cover.png` / `cover.webp`）放入相册目录：

```bash
cp best-photo.jpg /var/lib/picshare/photos/主力产品/cover.jpg
```

或后台：hover 图片 → 【设为封面】（会自动生成 cover.jpg 副本）。

### 5.3 添加相册描述

```bash
echo "本产品线包括 5 款" > /var/lib/picshare/photos/主力产品/description.txt
```

### 5.4 排序

目录名加数字前缀：

```
01.主力产品
02.新品上市
03.历史归档
```

按自然顺序排序（`10.……` 排在 `2.……` 之前）。

### 5.5 清除缩略图缓存

```bash
sudo systemctl stop picshare
sudo rm -rf /var/lib/picshare/thumbs/*
sudo systemctl start picshare
# 首次访问会重新生成
```

## 6. 备份

只需备份 `photos/` 目录：

```bash
sudo tar czf photos-$(date +%Y%m%d).tar.gz -C /var/lib/picshare photos/
```

`thumbs/` 是缓存，可随时重建。

## 7. 升级

```bash
# 1. 备份
sudo systemctl stop picshare
sudo cp /usr/local/bin/picshare /usr/local/bin/picshare.bak

# 2. 部署新版本
sudo install -m 755 picshare /usr/local/bin/picshare
sudo systemctl start picshare

# 3. 回滚（如需）
sudo cp /usr/local/bin/picshare.bak /usr/local/bin/picshare
sudo systemctl restart picshare
```

## 8. 故障排查

### 看不到图片？

```bash
# 1. 检查目录权限
ls -la /var/lib/picshare/photos/

# 2. 检查 picshare 进程
sudo journalctl -u picshare -f

# 3. 直接 curl 测试
curl -i http://127.0.0.1:8080/api/albums
```

### 上传 500 错误？

检查磁盘空间和目录权限：

```bash
df -h /var/lib/picshare
ls -ld /var/lib/picshare/photos
```

### 缩略图 404？

首次访问会实时生成，**第一次访问慢是正常的**。之后走 `30d` 缓存。

### Nginx 反代 502？

```bash
# picshare 是否在跑？
sudo systemctl status picshare
# 8080 是否监听？
ss -tlnp | grep 8080
```
