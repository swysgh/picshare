# 着陆页（Landing Page）实施方案

| 项 | 值 |
|---|---|
| 状态 | **已实现**（2026-10-18，见文末「实现备注」） |
| 日期 | 2026-10-18 |
| 来源 | 客户需求：进入首页先显示一个"霸气"的着陆页，参考 Lychee，但不要那么复杂 |
| 参考实现 | Lychee v8 的 `classic` 布局（`resources/js/v8/views/landing/LandingClassic.vue`） |

执行前先读仓库根目录的 `AGENTS.md`。本文只写"这次要做什么"，项目结构说明不在本文重复。

---

## 0. 目标与一句话方案

根路径 `/` 变成全屏着陆页，点"进入图库"后跳到 `/gallery`；cookie 记住已进入，30 天内再访问 `/` 直接 302 到相册列表。

**唯一比 Lychee 多做的一件事**：Lychee 没有 remember 机制，靠"进入后 URL 变成 `/gallery`"避免重复曝光——客户如果把根域名存书签，每次打开还是会看到着陆页。客户明确要求"不要每次打开都显示"，所以加 cookie。

---

## 1. 方案边界：照抄 Lychee 哪些、丢哪些

| Lychee 的做法 | 我们 | 理由 |
|---|---|---|
| `/` 就是着陆页，进入后 URL 变 `/gallery` | ✅ 照抄 | 客户要"进首页显示" |
| 客户端 SPA 路由分发 | ❌ 改服务端判定 | picshare 是多页静态资源，服务端判 cookie 更简单、无 FOUC |
| `LandingPageResource` 40+ 字段整形接口 | ❌ | 配置直接注入 HTML 占位符，不加 JSON 接口 |
| 4 种布局 + SE 授权分级 | ❌ | 只做 `classic` |
| CTA 9 个位置 + 像素偏移 + 方向 | ❌ | 固定偏下居中 |
| `LandingLink` / `LandingFeaturedItem` 两张表 + 后台 CRUD | ❌ | 无 |
| `intro_screen_enabled` 双层首屏（先播 logo 动画再进 hero） | ❌ | 首屏时间翻倍 |
| 横/竖两套背景图 | ✅ 照抄 | 竖屏手机用横图会被 `object-fit: cover` 裁成中间一条 |
| CTA 文案可配 | ✅ 照抄 | 客户可能想写"查看产品" |
| 遮罩透明度可配 | ✅ 照抄 | 换背景图时唯一需要调的旋钮 |
| 背景解析失败回落默认图、绝不白屏 | ✅ 照抄 | 我们回落纯色渐变（项目没有内置素材图） |
| 无 cookie / 无 remember | ⚠️ 唯一偏离 | 见第 0 节 |

---

## 2. 动手前必须知道的 5 个项目边界

这几条是读代码才会踩到的，跟本需求直接相关：

1. **`config.site_title` 是死配置。** 页面文案全部来自 `web/js/i18n.js` 的字典，`web/web.go` 只替换 `<!--PICSHARE_DEFAULT_LANG-->` 一个占位符。着陆页是唯一必须显示站名的地方，本次要顺带把 `site_title` 真正注入 HTML。
2. **`/thumb` 的 `w` 参数只认 `grid_thumb_size`(480) 和 `lightbox_size`(1600)**，其他值静默回落 480。要背景大图就用 `w=1600`，别指望随便传数值。
3. **路由映射硬编码在 `main.go` 的 `spaHandler`**：`/` → `index.html`、`/album/*` → `index.html`、`/admin*` → `admin.html`，且 `/api/`、`/photos/` 前缀先返回 404。新增路径必须同时检查这几个分支，否则要么 404、要么静默落到 index。
4. **新增静态文件必须落在 `web/web.go` 的 `go:embed` pattern 内**（`*.html`、`css/*.css`、`js/*.js`、`vendor/photoswipe/*`）。文件在仓库里存在、运行时却 404，就是这个原因。
5. **数据层不用动**：`/api/albums` 已返回 `cover_url` / `cover_thumb` / `description` / `photo_count` / `children`。`.order`、`.hidden`、`description.txt`、`cover.*` 是约定文件名，别破坏。

---

## 3. 路由与行为

| 路径 | 行为 |
|---|---|
| `GET /` | cookie `picshare_landing=v<N>` 且 `N == landing_version` → `302 /gallery`；否则渲染 `landing.html` |
| `GET /?landing=1` | 强制渲染着陆页，无视 cookie（客户演示/自己预览用） |
| `GET /enter` | `Set-Cookie` + `Cache-Control: no-store` → `302 /gallery` |
| `GET /gallery` `/gallery/` | `index.html`（原相册列表，逻辑不变） |
| `GET /album/{path}` | `index.html`（不变；直链完全绕过着陆页） |
| `GET /admin` `/admin/album/*` | `admin.html`（不变，Basic Auth 不变） |
| `landing_enabled=false` | `/` 直接 `index.html`，与今天完全一致 |

Cookie：`picshare_landing=v<N>`，`Path=/`、`HttpOnly`、`SameSite=Lax`、`Max-Age=<TTL 秒>`。

`landing_remember=off` 时不写 cookie，`/enter` 仍 302 到 `/gallery`（这样"每次进首页都显示"也成立）。

### Go 侧参考实现

```go
const landingCookie = "picshare_landing"

func landingCookieValue(cfg *config.Config) string {
	return "v" + strconv.Itoa(cfg.LandingVersion)
}

// GET /
func landingGate(cfg *config.Config, web http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.LandingEnabled { // kill switch：与改造前完全一致
			r.URL.Path = "/index.html"
			web.ServeHTTP(w, r)
			return
		}
		if r.URL.Query().Get("landing") == "1" { // 强制预览
			r.URL.Path = "/landing.html"
			web.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(landingCookie); err == nil && c.Value == landingCookieValue(cfg) {
			http.Redirect(w, r, "/gallery", http.StatusFound)
			return
		}
		r.URL.Path = "/landing.html"
		web.ServeHTTP(w, r)
	}
}

// GET /enter
func landingEnter(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.LandingRememberTTL > 0 {
			http.SetCookie(w, &http.Cookie{
				Name:     landingCookie,
				Value:    landingCookieValue(cfg),
				Path:     "/",
				MaxAge:   int(cfg.LandingRememberTTL.Seconds()),
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/gallery", http.StatusFound)
	}
}
```

在 `mux` 上注册时**必须排在 `spaHandler` 之前**：

```go
mux.HandleFunc("GET /enter", landingEnter(cfg))
mux.HandleFunc("GET /{$}", landingGate(cfg, webHandler))   // {$} = 只匹配 "/"，不吞子路径
mux.Handle("/", spaHandler(webHandler))                    // 保持最后
```

`spaHandler` 里把 `case "/": r.URL.Path = "/index.html"` 换成 `/gallery`、`/gallery/` → `/index.html`。

> 这套注册方式已用 `httptest` 实测过：`{$}`（Go 1.22+ 的精确匹配）与 `/` 兜底共存不会 panic，更具体的模式优先；`/gallery`、`/album/*`、`/admin/*` 都不受影响。注意 `spaHandler` 收到的是 `http.Handler`，别把裸函数直接传给 `mux.Handle`。

---

## 4. 配置项

```json
"landing_enabled": true,
"landing_title": "",
"landing_subtitle": "",
"landing_cta_text": "",
"landing_background": "",
"landing_background_portrait": "",
"landing_backdrop_opacity": 45,
"landing_remember": "30d",
"landing_version": 1
```

| 字段 | 语义 |
|---|---|
| `landing_enabled` | 总开关。**默认 true**（合并即生效）；想"合并但不生效"就在部署机配置里显式写 false |
| `landing_title` | 空 → 用 i18n 的 `site_title`（同时用于 `<title>`） |
| `landing_subtitle` | 空 → 用 i18n 的 `landing_subtitle` |
| `landing_cta_text` | 底部提示文案（无按钮 CTA 后仍复用此字段）；空 → 用 i18n 的 `landing_enter_hint` |
| `landing_background` | 横版背景。后台选过背景（`photos/.landing_bg`）时以它为准；都为空 → 自动取第一个有封面相册的 `cover_thumb`（`w=1600`） |
| `landing_background_portrait` | 竖版背景。空 → 回落横版 |
| `landing_backdrop_opacity` | 遮罩不透明度 0–100，越界带中文原因非 0 退出 |
| `landing_remember` | `off` / `12h` / `7d` / `30d`。**Go 的 `time.ParseDuration` 不认 `d`，要自己写解析**；非法值带中文原因非 0 退出 |
| `landing_version` | 改了 = 所有老访客重新看一遍，不用等 TTL、不用教人清缓存 |

实现约定：

- 结构体里 `LandingRemember` 用 `string` 接 JSON，`validate()` 里解析到 `LandingRememberTTL time.Duration \`json:"-"\``，`off` → 0。
- 启动日志补一行：`landing enabled=%v remember=%s bg=%s`（项目惯例：启动打印生效配置）。
- 部署机 `/etc/picshare.json` **不用改**：`config.Load` 从 `Default()` 起 unmarshal，缺的 key 走默认值。

---

## 5. 代码改动清单

| 文件 | 改什么 | 量 |
|---|---|---|
| `internal/config/config.go` | 9 个字段 + `ParseRemember` + validate | ~70 行 |
| `internal/config/config_test.go` | `landing_remember` 合法/非法值表驱动测试 | ~40 行 |
| `web/web.go` | `NewHandler(defaultLang)` → `NewHandler(opts Options)`；占位符从 1 个扩成 7 个 | ~35 行 |
| `web/landing.html` | 新文件：`<picture>` 双源背景 + 遮罩 + 站名 + 副标题 + CTA + 语言切换 | ~60 行 |
| `web/css/landing.css` | 新文件（`css/*.css` 的 embed pattern 已覆盖） | ~110 行 |
| `web/js/i18n.js` | 加 `landing_subtitle` / `landing_enter` / `landing_enter_hint` × 中韩英；把 `renderLangSwitch` 从 `public.js` 挪进来并导出 | ~25 行 |
| `web/js/public.js` | 删掉本地 `renderLangSwitch`，改调 `window.i18n.renderLangSwitch()`；面包屑"首页" `/` → `/gallery`（第 75 行） | ~4 行 |
| `web/index.html` | `<h1>` 里的 `<a href="/">` → `/gallery`（第 12 行） | 1 行 |
| `main.go` | `/enter`、`/` 门禁、`spaHandler` 加 `/gallery` 映射 | ~55 行 |
| `README.md` / `DEPLOY.md` | 配置表补 9 个字段；URL 约定表补 `/gallery`、`/enter`、`?landing=1` | ~25 行 |

### 占位符注入

沿用现有 `<!--PICSHARE_DEFAULT_LANG-->` 的机制，在 `web.go` 里扩成：

```
<!--PICSHARE_DEFAULT_LANG-->          → "zh" / "ko" / "en"
<!--PICSHARE_SITE_TITLE-->            → cfg.SiteTitle
<!--PICSHARE_LANDING_TITLE-->         → landing_title 或 site_title
<!--PICSHARE_LANDING_SUBTITLE-->      → landing_subtitle 或空（留空则由 i18n.js 填）
<!--PICSHARE_LANDING_CTA-->           → landing_cta_text 或空（同上）
<!--PICSHARE_LANDING_BG-->            → 横版背景 URL
<!--PICSHARE_LANDING_BG_PORTRAIT-->   → 竖版背景 URL
<!--PICSHARE_LANDING_BACKDROP-->      → "0.45" 这样的数值（注入到 style 里）
```

**注意**：注入值必须 `html.EscapeString` 处理——缩略图 URL 带 `&w=1600&v=...`，不转义会破坏属性。空值不要输出字面空串以外的占位符，让前端 `data-i18n` 兜底。

---

## 6. 视觉规格

```
<main class="landing">                    全屏 100dvh，overflow hidden
  <picture>
    <source media="(orientation: portrait)" srcset="竖版">
    <img>                                 横版，object-fit: cover; object-position: center
  <div class="veil">                      线性渐变（顶部/底部加深），opacity 由配置控制
  <div class="hero">
    <h1>                                  clamp(2rem, 7vw, 4.5rem)，letter-spacing .08em，uppercase
    <p>                                   副标题，clamp(.9rem, 2.2vw, 1.15rem)
    <a href="/enter">                     大号、宽字距、hover scale(1.05)，末尾 "→" 做 pulse
  <div class="lang">                      右下角，复用现有 .lang-switch 样式
```

- 0.4s fade-in；`@media (prefers-reduced-motion: reduce)` 下全部关掉
- 不引外部字体/图标库，箭头用字符 `→`
- 深色配色沿用 `public.css` 已有的 `--bg` 系列变量，不新开一套色板
- `landing.html` **不要引 `public.js`**（它会按 `location.pathname === '/'` 判成 home 去拉 `/api/albums`）

---

## 7. 实施顺序（每步可独立验证）

1. `config.go` + 单测 → `make test`
2. `web.go` 改 Options 与占位符（此时 `landing.html` 还不存在，行为不变）→ `make build` 起服务确认老页面没坏
3. `landing.html` + `landing.css` + i18n，先用 `photos/尺寸测试/横图.png` 当背景在浏览器里调
4. `main.go` 三条路由接上，跑第 8 节的 curl
5. `index.html` / `public.js` 链接改 `/gallery`
6. `make vet && make test && make build`，中文 commit，**不打 tag**

---

## 8. 验收

```bash
make vet && make test && make build
./picshare --config config.json          # listen 是 0.0.0.0:8080

curl -si localhost:8080/ | head -1                                    # 200 + landing.html
curl -si localhost:8080/enter | grep -i 'set-cookie\|location'        # cookie + 302 /gallery
curl -si -H 'Cookie: picshare_landing=v1' localhost:8080/ | head -1   # 302 /gallery
curl -si -H 'Cookie: picshare_landing=v0' localhost:8080/ | head -1   # 200（版本不符）
curl -si -H 'Cookie: picshare_landing=v1' 'localhost:8080/?landing=1' | head -1  # 200（强制）
curl -s localhost:8080/gallery | grep -c album-grid                   # 1
curl -s localhost:8080/album/尺寸测试 | grep -c photo-grid            # 1
# landing_remember=off 时 /enter 不应返回 Set-Cookie
```

浏览器侧还要看一眼：横屏/竖屏背景裁切、刷新不闪、点站名不回弹到着陆页、`/admin/` 不受影响。

---

## 9. 风险与回滚

- **回滚**：`landing_enabled: false` 重启即恢复原样。无数据迁移、无 URL 永久失效。
- **唯一不可逆点**：`/` 从相册列表变成着陆页，老的根域名书签会多一次点击——这正是客户要的效果，上线前跟他确认一句。
- 背景图解析失败 → 回落纯色渐变（不白屏），日志 warning 一次。

---

## 10. 不做

后台上传背景按钮、布局切换、动画预设、导航/页脚自定义链接、精选内容、访问统计、og:image 分享卡、视频/轮播背景。

---

## 11. 交付约定

- 构建：`make build` / `make test` / `make vet`（版本号由最近 git tag 注入）
- Commit 用中文：`<类型>: <一句话概述>`，类型 `feat`/`fix`/`docs`/`refactor`/`test`/`chore`
- 只加功能**不要顺手打 `v*` tag**（打 tag 会触发 GitHub Actions 发 Release）
- 部署机是 rich37：`/hhdisk/rich37/picshare` + `/etc/picshare.json` + systemd + nginx 直出 `/photos`、`/thumbs`。只改前端时替换二进制即可；改了配置字段要同步那份 JSON（本次不需要，缺省值即可）
- `config.json` 含真实后台密码，已在 `.gitignore` 里，不要提交；不要动 `photos/`、`thumbs/`

---

## 12. 实现备注（2026-10-18）

与本文的两处差异，均为 CSP 要求所致：

1. **着陆页脚本是外联文件 `web/js/landing.js`**，不是 inline `<script>`。CSP 未声明 `script-src` 时会回落到 `default-src 'self'`，inline 脚本本来就会被挡；因此 CSP 显式补上 `script-src 'self'`（不加 `unsafe-inline`），着陆页的 i18n 兜底、语言切换、Enter 快捷进入都放进 `landing.js`。
2. **`default_lang` 改为注入 `data-default-lang` 属性**（`index.html` / `landing.html` 的 `<html>`），`i18n.js` 的 `detectLang()` 优先读它。原来 `index.html` 里那句 `window.PICSHARE_DEFAULT_LANG = ...` 的 inline 脚本在现有 CSP 下一直是失效的，顺手去掉。

另外：

- 背景图 `<img>` 只用 `srcset`、不写 `src`：空值时浏览器不会去请求当前文档（空 `src` 会被当成页面 URL），也就不用为“没有背景”造占位图。
- `landing_remember=off` 时连已存在的旧 cookie 也不认，保证“每次进首页都显示”。
- `landing.html` 走 `Cache-Control: no-store`，避免缓存里的 200 盖住 cookie 判定或留下过期的背景 URL。

## 13. 实现备注（2026-10-18 第二轮：客户反馈 4 项）

1. **CTA 去按钮化**：`<a class="cta">` 已删除，改为底部 `.enter-hint`（SVG 线框鼠标 + 小字），整页由 `landing.js` 的 `initWholePageClick()` 负责跳 `/enter`；语言切换按钮的点击被跳过。`main.landing` 加 `cursor: pointer` 提示整页可点。小字仍可由 `landing_cta_text` 覆盖，空值走 i18n `landing_enter_hint`。
2. **渐入动画**：背景图 0.8s/延迟 0.2s、遮罩 0.6s/延迟 0.1s、提示区 0.5s/延迟 0.6s，文字仍是 0.4s。背景与遮罩用只改 `opacity` 的关键帧（`landing-fade-in` / `landing-veil-in`）：`landing-fade` 带 `translateY`，用在整屏图上会露出边缘；遮罩的终点值取自 `--veil-opacity`（由 `landing_backdrop_opacity` 注入），否则动画的 `both` 会覆盖配置值。`prefers-reduced-motion: reduce` 时全部 `animation: none`。
3. **后台可选背景**：照片 hover 菜单新增「设为着陆页背景」→ `POST /api/admin/setlandingbg` → 原子写入 `photos/.landing_bg`（一行相对路径，不碰 `config.json`）。`main.go` 每次渲染着陆页时读该文件并解析成 `w=lightbox_size` 的 `/thumb` URL；照片被删/移走则 log 一次 warning 并回落到 `landing_background` / 默认封面。`GET /landing.html` 也走同一渲染路径，避免直链拿到旧背景。
4. **`landing_remember=off`**：代码零改动，仅部署机配置加 `"landing_remember": "off"`。

## 14. 实现备注（2026-10-18 第三轮：后台图形化配置）

客户反馈后台改不了标题和着陆页文案，且英/韩文案写死在 `web/js/i18n.js`。本轮新增后台设置页。

### 14.1 接口与持久化

- `GET /api/admin/settings` → `internal/config.Settings`（白名单：`site_title`、`landing_enabled`、`landing_i18n`、`landing_backdrop_opacity`、`landing_remember`、`max_upload_mb`）。响应**不含** `admin_pass`/`admin_user`/`listen`/目录路径。
- `POST /api/admin/settings` → 全字段必填；未知字段、缺字段、越界值一律 400 + 中文原因（JSON `{"error": "..."}`，便于前端 toast 原样展示）。`landing_i18n` 只接受 `title/subtitle/cta` × `zh/ko/en`。
- 落盘走 `config.SaveSettings`：读原文 → `map[string]json.RawMessage` → 只覆盖白名单 key → `MarshalIndent` → 同目录临时文件 + `fsync` + `rename`，权限 0600。`admin_pass` 与未知字段字节级保留，所以设置页永远不可能丢掉或覆盖密钥。
- 保存即生效：`main.liveCfg`（RWMutex）先写文件再换内存指针（写失败则内存不动），着陆页渲染、`/gallery` 的标题、上传上限都按请求读当前值。`index.html` 因此从 `max-age=3600` 改成 `no-store`。

### 14.2 三语言覆盖与回退链

```json
"landing_i18n": {
  "title":    {"zh": "", "ko": "", "en": ""},
  "subtitle": {"zh": "", "ko": "", "en": ""},
  "cta":      {"zh": "", "ko": "", "en": ""}
}
```

旧的 `landing_title`/`landing_subtitle`/`landing_cta_text` 保留作回退，**不需要迁移现有配置**：某语言为空 → 旧单语言字段 → 仍为空 → `i18n.js` 字典兜底。标题的回退链最后落到 `site_title`（它同时是 `<title>` 与 `<h1>`，不能为空）。

服务端把整份文案（已折叠旧字段）以 HTML 转义后的 JSON 注入 `<main data-landing-texts="...">`；marker `PICSHARE_LANDING_TITLE/SUBTITLE/CTA` 只提供 default_lang 的首屏文案（无 JS 时也合理）。`landing.js` 按 `i18n.getLang()` 覆盖 `<h1>`、副标题、进入提示与 `document.title`：副标题/提示即使取到空值也写入元素，再交给 `fillFallbacks()` 走字典——否则切到英/韩时会残留服务端渲染的中文。

### 14.3 site_title 老 bug

`index.html` 的 `<h1 data-i18n="site_title">` 一直会被 `i18n.js applyLang()` 用字典值（产品图库/제품 갤러리/Product Gallery）覆盖，配置里的 `site_title` 实际无效。现在服务端注入 `<html data-site-title="...">`，`t('site_title')` 优先读它，非空才回退字典。

### 14.4 UI

后台 header 新增「设置」按钮 → modal（复用 `#modal-backdrop`），三段：站点 / 着陆页（启用开关、大标题·副标题·进入提示各三语言输入框、遮罩透明度 0-100、记住进入 off/12h/7d/30d）/ 上传（单文件上限 MB，0 = 不限制）。表单 `novalidate`，越界值由前端给中文 toast（与服务端同规则），保存成功 toast「设置已保存」。配色沿用 `admin.css` 深色变量。
