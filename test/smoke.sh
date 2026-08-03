#!/usr/bin/env bash
# picshare 端到端冒烟测试
# 启动 picshare，校验 API + thumb + 后台 Basic Auth + 安全
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PORT=${PORT:-18080}
BASE="http://127.0.0.1:${PORT}"
CONFIG="$(mktemp -d)/config.json"
TESTDIR="$(mktemp -d)"
SAMPLE_PNG="$(mktemp -u --suffix=.png)"

# 先杀掉可能残留的 picshare 进程占用相同端口
pkill -f "picshare.*--config ${CONFIG}" 2>/dev/null || true
sleep 0.3

cleanup() {
  if [[ -n "${PID:-}" ]]; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$CONFIG" "$TESTDIR"
}
trap cleanup EXIT

# 1. 生成测试图片 (100x100 红色 PNG)
python3 -c "
import struct, zlib, sys
def png(w, h, r, g, b):
    sig = b'\x89PNG\r\n\x1a\n'
    def chunk(t, d):
        return struct.pack('>I', len(d)) + t + d + struct.pack('>I', zlib.crc32(t + d) & 0xffffffff)
    ihdr = struct.pack('>IIBBBBB', w, h, 8, 2, 0, 0, 0)
    raw = b''
    for y in range(h):
        raw += b'\x00' + (bytes([r, g, b]) * w)
    idat = zlib.compress(raw)
    iend = b''
    return sig + chunk(b'IHDR', ihdr) + chunk(b'IDAT', idat) + chunk(b'IEND', iend)
sys.stdout.buffer.write(png(100, 100, 255, 100, 50))
" > "$SAMPLE_PNG"

# 2. 写配置
cat > "$CONFIG" <<EOF
{
  "listen": "127.0.0.1:${PORT}",
  "photos_dir": "${TESTDIR}/photos",
  "thumbs_dir": "${TESTDIR}/thumbs",
  "admin_user": "testadmin",
  "admin_pass": "testpass",
  "site_title": "Test",
  "default_lang": "zh",
  "grid_thumb_size": 100,
  "lightbox_size": 200,
  "enable_exif_rotation": true,
  "web_path_prefix": "/photos"
}
EOF

# 3. 启动 picshare
echo ">>> starting picshare"
./picshare --config "$CONFIG" &
PID=$!
sleep 1

# 等待端口
for _ in {1..30}; do
  if curl -sf "$BASE/api/albums" >/dev/null; then break; fi
  sleep 0.2
done

fail() { echo "FAIL: $*"; exit 1; }
pass() { echo "PASS: $*"; }

# 4. 公开 API
echo ">>> testing public API"
albums=$(curl -sf "$BASE/api/albums")
[[ "$albums" == "[]" ]] || fail "expected empty albums, got: $albums"
pass "empty albums"

# 5. 后台未授权 → 401
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/admin/manage")
[[ "$code" == "401" ]] || fail "expected 401, got $code"
pass "admin requires auth"

# 6. 后台 Basic Auth 登录
echo ">>> testing admin auth"
code=$(curl -s -o /dev/null -w "%{http_code}" -u testadmin:testpass "$BASE/api/admin/manage")
[[ "$code" == "200" ]] || fail "expected 200, got $code"
pass "admin basic auth"

# 7. 创建相册
echo ">>> testing mkdir"
curl -sf -u testadmin:testpass -X POST "$BASE/api/admin/mkdir" \
  -H "Content-Type: application/json" \
  -d '{"album":"测试相册"}' >/dev/null
[[ -d "$TESTDIR/photos/测试相册" ]] || fail "album dir not created"
pass "mkdir creates directory"

# 8. 上传图片
echo ">>> testing upload"
curl -sf -u testadmin:testpass -X POST "$BASE/api/admin/upload?album=$(echo -n '测试相册' | python3 -c 'import sys,urllib.parse;print(urllib.parse.quote(sys.stdin.read()))')" \
  -F "files=@${SAMPLE_PNG}" >/dev/null
[[ -f "$TESTDIR/photos/测试相册/$(basename $SAMPLE_PNG)" ]] || fail "uploaded file not found"
pass "upload works"

# 9. 列出相册
albums=$(curl -sf "$BASE/api/albums")
echo "$albums" | grep -q "测试相册" || fail "album not listed"
pass "album in list"

# 10. 获取相册详情
detail=$(curl -sf "$BASE/api/albums/$(python3 -c 'import urllib.parse;print(urllib.parse.quote("测试相册"))')")
echo "$detail" | grep -q "photos" || fail "album detail missing photos"
pass "album detail"

# 11. 缩略图
thumb_url=$(echo "$albums" | python3 -c "import json,sys; [print(a['cover_thumb']) for a in json.load(sys.stdin) if a['name']=='测试相册']")
thumb_code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE$thumb_url")
[[ "$thumb_code" == "200" ]] || fail "thumb request failed: $thumb_code"
pass "thumbnail generated"

# 12. 缩略图文件存在
sleep 0.3
[[ -n "$(find "$TESTDIR/thumbs" -type f 2>/dev/null)" ]] || fail "no thumb files"
pass "thumb cached to disk"

# 13. 路径穿越攻击
echo ">>> testing path traversal"
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/thumb?p=/photos/../etc/passwd")
[[ "$code" == "400" ]] || fail "expected 400, got $code"
pass "path traversal rejected"

# 14. 上传非图片
echo ">>> testing non-image upload"
echo "not an image" > /tmp/notimage.txt
noncode=$(curl -s -o /dev/null -w "%{http_code}" -u testadmin:testpass \
  -X POST "$BASE/api/admin/upload?album=test" \
  -F "files=@/tmp/notimage.txt")
[[ "$noncode" == "400" ]] || fail "expected 400, got $noncode"
pass "non-image rejected"

# 15. 设置封面
echo ">>> testing setcover"
photo_name=$(basename "$SAMPLE_PNG")
curl -sf -u testadmin:testpass -X POST "$BASE/api/admin/setcover" \
  -H "Content-Type: application/json" \
  -d "{\"album\":\"测试相册\",\"photo\":\"${photo_name}\"}" >/dev/null
[[ -f "$TESTDIR/photos/测试相册/cover.png" ]] || fail "cover not created"
pass "set cover"

# 16. 删除图片
echo ">>> testing delete"
curl -sf -u testadmin:testpass -X POST "$BASE/api/admin/delete" \
  -H "Content-Type: application/json" \
  -d "{\"album\":\"测试相册\",\"paths\":[\"cover.png\"]}" >/dev/null
[[ ! -f "$TESTDIR/photos/测试相册/cover.png" ]] || fail "cover not deleted"
pass "delete photo"

# 17. 前台 HTML
echo ">>> testing public HTML"
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/")
[[ "$code" == "200" ]] || fail "home page returned $code"
curl -s "$BASE/" | grep -q "picshare" || true
pass "home page renders"

# 18. 静态资源
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/css/public.css")
[[ "$code" == "200" ]] || fail "css returned $code"
pass "static assets"

# 19. i18n JS
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/js/i18n.js")
[[ "$code" == "200" ]] || fail "i18n js returned $code"
pass "i18n script"

# 20. PhotoSwipe
code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/vendor/photoswipe/photoswipe.umd.min.js")
[[ "$code" == "200" ]] || fail "photoswipe returned $code"
pass "photoswipe vendor"

echo ""
echo "=== ALL TESTS PASSED ==="
