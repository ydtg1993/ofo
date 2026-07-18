# 前端媒体 Blob 加密防护方案

> 适用场景：Go + Gin 博客系统，防止图片/视频被爬取、盗链、批量下载。
> 核心思路：**页面 HTML 中不出现任何真实文件 URL，所有媒体通过 JS 以 Blob 方式加载，加载后撤销 Blob URL。**

---

## 目录

- [1. 架构总览](#1-架构总览)
- [2. 技术栈](#2-技术栈)
- [3. 服务端实现](#3-服务端实现)
- [4. 客户端实现](#4-客户端实现)
- [5. Token 签名系统](#5-token-签名系统)
- [6. 图片处理](#6-图片处理)
- [7. 视频处理](#7-视频处理)
- [8. 生产环境保护](#8-生产环境保护)
- [9. SEO 兼容](#9-seo-兼容)
- [10. 遇到的问题与解决](#10-遇到的问题与解决)
- [11. 配置参考](#11-配置参考)
- [12. 安全边界](#12-安全边界)

---

## 1. 架构总览

```
                    服务端                              浏览器
                    ──────                              ──────
                                                
  存储文件           HTML 渲染                          JS 执行后 DOM
  ─────────         ──────────                         ─────────────
  /static/           <img                                <img
  uploads/             data-mid="a3f2c1"                  src="blob://xxx"
  xxx.jpg              width="308" ...>                   width="308" ...>
                       (零 URL 信息)                      (已撤销，不可复制)

                    <script>                             
                      window.__OFO_MEDIA__.urls = {       
                        "a3f2c1": "/media/...?t=token"    ① 读取 URL 映射
                      }                                   ② fetch → blob
                    </script>                             ③ img.src = blobURL
                                                          ④ onload → revoke
                                                          ⑤ removeAttribute('data-mid')

  /media/xxx?t=token  ←── Token 校验 + Cookie 校验 ──→  fetch() 请求
  /media/xxx?t=token  ←── 无 Cookie → 403 ──────────→  curl/wget 请求
```

### 防护层级

```
第 1 层：HTML 隐藏 —— data-mid 随机 ID，无 URL 信息
第 2 层：Token 签名 —— 代理 URL 带 HMAC 签名，过期失效
第 3 层：Cookie 校验 —— 代理接口要求会话 Cookie，curl 无法访问
第 4 层：Blob 撤销 —— 图片加载完立即撤销 Blob URL，复制到新标签页无效
第 5 层：生产保护 —— 禁止右键/F12/复制，普通用户看不到 Network 面板
```

---

## 2. 技术栈

| 层 | 技术 | 用途 |
|----|------|------|
| 后端 | Go 1.x + Gin | HTTP 服务、路由、中间件 |
| 存储 | Storage 接口（本地磁盘 / 七牛云） | 文件读写抽象 |
| 签名 | crypto/hmac + crypto/sha256 | Token 生成和校验 |
| 加密 | crypto/rand | 随机 ID 生成 |
| 前端 | 原生 JavaScript（无框架依赖） | Blob 加载、DOM 操作 |
| 懒加载 | IntersectionObserver API | 视口检测、按需加载 |
| 灯箱 | `<dialog>` 元素 | 图片全屏预览 |
| CSP | Content-Security-Policy | 限制资源来源 |

---

## 3. 服务端实现

### 3.1 项目结构

```
handlers/
  media.go        ← 核心：代理接口 + Token + MediaMap + URL 转换
  admin.go        ← 缩略图生成（ThumbnailMidImage）
  post.go         ← 文章页渲染
router/
  router.go       ← 路由 + 模板函数注册
middleware/
  security.go     ← CSP 头（blob: 白名单）
  hotlink.go      ← 防盗链
static/js/
  media-blob.js   ← 客户端 Blob 加载器
  protect.js      ← 生产环境保护（禁用 F12）
  main.js         ← 灯箱集成
config/
  config.go       ← 配置项
```

### 3.2 代理接口 `/media/*filepath`

```go
// handlers/media.go

func (h *Handler) MediaProxy(c *gin.Context) {
    fp := strings.TrimPrefix(c.Param("filepath"), "/")
    fp = path.Clean(fp) // 用 path.Clean 而非 filepath.Clean（Windows 兼容）

    // ① 路径白名单
    if !strings.HasPrefix(fp, "uploads/") && !strings.HasPrefix(fp, "stickers/") {
        c.AbortWithStatus(403)
        return
    }

    // ② Cookie 校验
    sessCookie, _ := c.Cookie("ofo_m")
    if sessCookie == "" || !validatePageToken(...) {
        c.AbortWithStatus(403)
        return
    }

    // ③ Token 校验
    token := c.Query("t")
    if !validateMediaToken(fp, token, secret, ttl) {
        c.AbortWithStatus(403)
        return
    }

    // ④ 读取文件并返回
    rc, _ := h.Storage.Get(ctx, fp)
    body, _ := io.ReadAll(io.LimitReader(rc, maxMediaSize+1))
    http.ServeContent(c.Writer, c.Request, filepath.Base(fp), time.Now(),
        bytes.NewReader(body))
}
```

> **Windows 注意**：URL 路径操作必须用 `path.Clean` 而非 `filepath.Clean`，后者在 Windows 上会把 `/` 转成 `\`，导致路径匹配失败。

### 3.3 URL 替换流程

HTML 中的 `<img src="/static/uploads/file.jpg">` 经过以下流水线处理：

```go
// router.go 模板函数
"lazyImages": func(html string) template.HTML {
    html = handlers.InjectLazyLoading(html)       // ① 加 loading="lazy"
    html = handlers.InjectImageDimensions(html, store)  // ② 加宽高
    html = handlers.InjectVideoDimensions(html, store)  // ③ 视频宽高
    return template.HTML(handlers.BuildMediaMapWith(html, store, cfg,
        handlers.CurrentMediaMap()))               // ④ 替换为 data-mid
},
```

> **顺序不能变**：维度注入必须在 URL 替换之前，因为 `InjectImageDimensions` 读取 `src` 属性获取文件路径。

### 3.4 MediaMap —— 随机 ID 映射

```go
type MediaMap struct {
    entries map[string]string // "a3f2c1" → "/media/...?t=token"
}

func (mm *MediaMap) Add(url string) string {
    id := randomMID()           // crypto/rand 生成 8 位 hex
    mm.entries[id] = url
    return id
}

func (mm *MediaMap) Script() template.HTML {
    js, _ := json.Marshal(mm.entries)
    return template.HTML(fmt.Sprintf(
        `<script>window.__OFO_MEDIA__=window.__OFO_MEDIA__||{};window.__OFO_MEDIA__.urls=%s;</script>`,
        js,
    ))
}
```

**为什么用随机 ID 而不是顺序数字？**

| 方案 | HTML | 安全性 |
|------|------|--------|
| 顺序数字 | `data-mid="0"`, `data-mid="1"` | ❌ 可遍历猜测 |
| 随机 hex | `data-mid="a3f2c1"`, `data-mid="b7d4e9"` | ✅ 不可猜测 |

### 3.5 跨模板函数共享状态

Go 模板函数是独立调用，无法直接共享变量。使用 `atomic.Pointer` 实现请求级共享：

```go
var currentMM atomic.Pointer[MediaMap]

// 页面渲染开始时初始化（在 <head> 中的 mediaConfigScript 调用）
func SetCurrentMediaMap(mm *MediaMap) { currentMM.Store(mm) }

// 后续所有模板函数都从同一个 MediaMap 读取
func CurrentMediaMap() *MediaMap { return currentMM.Load() }
```

> Go 模板渲染在单个 goroutine 中同步执行，`atomic.Pointer` 保证了请求间的隔离。

---

## 4. 客户端实现

### 4.1 media-blob.js 核心流程

```javascript
// ① 读取服务器注入的 URL 映射
var URLMAP = window.__OFO_MEDIA__.urls;
// {"a3f2c1": "/media/uploads/xxx.jpg?t=token", ...}

// ② 找所有 data-mid 元素
var elements = document.querySelectorAll('[data-mid]');

// ③ IntersectionObserver 懒加载
var observer = new IntersectionObserver(function(entries) {
    entries.forEach(function(entry) {
        if (!entry.isIntersecting) return;
        loadMediaElement(entry.target);
        observer.unobserve(entry.target);
    });
}, { rootMargin: '300px' });

// ④ 加载单个元素
function loadMediaElement(el) {
    var mid = el.getAttribute('data-mid');
    var proxyURL = URLMAP[mid];
    var isVideo = el.tagName.toLowerCase() === 'video';

    fetch(proxyURL)
        .then(r => r.blob())
        .then(blob => {
            var blobURL = URL.createObjectURL(blob);

            // 存 JS 属性（DOM 属性稍后删除）
            el._ofoMid = mid;
            el.src = blobURL;
            el.classList.add('blob-loaded');
            el.removeAttribute('data-mid');  // 从 DOM 清除

            if (!isVideo) {
                // 图片：加载完撤销
                el.addEventListener('load', () => URL.revokeObjectURL(blobURL));
            }
            // 视频：不撤销（解码器需要持续读取），页面关闭时清理
        });
}
```

### 4.2 灯箱集成

灯箱需要在预览时重新获取 Blob URL（因为原图片的 Blob 已被撤销）：

```javascript
// main.js — show() 函数
var isBlob = img.getAttribute('data-mid') !== null
          || img.classList.contains('blob-loaded');

if (isBlob) {
    // loadBlobMedia 内部从 URLMAP 按 mid 查找代理 URL，重新 fetch
    window.loadBlobMedia(img).then(function(result) {
        lightImg.src = result.blobURL;
        // 获取新的 blob，灯箱关闭时撤销
        lightImg.onload = () => URL.revokeObjectURL(result.blobURL);
    });
}
```

> **关键点**：`data-mid` 已在首次加载时从 DOM 删除，灯箱通过 `el._ofoMid`（JS 属性）获取 mid，再从 `URLMAP` 查找代理 URL。

### 4.3 "加载更多"集成

首页卡片批量展开时，新出现的 `[data-mid]` 元素需要被 Observer 接管：

```javascript
var mo = new MutationObserver(function(mutations) {
    mutations.forEach(function(m) {
        var t = m.target;
        if (t.classList.contains('post-card')
            && !t.classList.contains('post-card--hidden')) {
            // 新展开的卡片 → 观察其中的 media 元素
            t.querySelectorAll('[data-mid]').forEach(el => observer.observe(el));
        }
    });
});
```

---

## 5. Token 签名系统

### 5.1 媒体 Token（URL 参数）

```
URL: /media/uploads/file.jpg?t=<timestamp_hex>:<hmac_signature_hex>

生成:
  ts = hex(now.Unix())
  sig = hex(HMAC-SHA256(ts + ":" + filepath, secret))
  token = ts + ":" + sig

校验:
  ① 解析 ts, sig
  ② now - ts < TTL ?
  ③ 重新计算 HMAC，常量时间比较
```

### 5.2 页面 Token（Cookie）

```
Cookie: ofo_m=<window_hex>:<hmac_signature_hex>

生成:
  window = hex(now.Unix() / ttl)    // 按 TTL 分桶
  sig = hex(HMAC-SHA256("page:" + window, secret))

校验:
  ① 解析 window, sig
  ② 当前窗口与生成窗口相差不超过 1
  ③ 重新计算 HMAC，比较
```

### 5.3 双重校验

| 校验项 | 作用 | 失效场景 |
|--------|------|----------|
| URL Token (`?t=`) | 绑定文件路径 + 时效 | 过期后需刷新页面 |
| Cookie (`ofo_m`) | 绑定浏览器会话 | 复制 URL 到 curl/别人电脑 |

---

## 6. 图片处理

### 要点

1. **加载流程**：fetch 代理 URL → blob → `URL.createObjectURL` → 设 `img.src` → `onload` 后 `revokeObjectURL`
2. **撤销时机**：`load` 事件触发后立即撤销。浏览器图片解码器渲染完就不再需要源数据
3. **撤销效果**：`img.src` 仍显示 `blob://xxx`，但复制到新标签页 → 报错
4. **灯箱重取**：灯箱通过 `loadBlobMedia()` 重新 fetch，生成新 blob，展示后再撤销
5. **骨架屏**：宽高和 `aspect-ratio` 在服务端注入，图片加载前页面不抖动

### 代码路径

```
服务端：
  InjectLazyLoading → InjectImageDimensions → BuildMediaMapWith
  原始: <img src="/static/uploads/file.jpg">
  输出: <img data-mid="a3f2c1" width="308" height="173" style="aspect-ratio:308/173">

客户端：
  IntersectionObserver → loadMediaElement → fetch → blob → revoke
  DOM最终: <img src="blob://xxx" width="308" height="173" class="blob-loaded">
```

---

## 7. 视频处理

> 视频是 Blob 方案中最复杂的部分。以下详述遇到的问题和最终方案。

### 7.1 核心矛盾

| 需求 | 矛盾 |
|------|------|
| Blob URL 加载 | 解码器需要持续读取数据（拖进度条 = 随机访问） |
| 撤销 Blob URL | 解码器无法再读取 → 播放中断、反复重新加载 |
| 代理 URL 直接 | 支持 Range 请求，但 DOM 暴露 URL |

### 7.2 方案演进

#### 方案 A：代理 URL 直接（初版）
```javascript
video.src = proxyURL;  // /media/uploads/file.mp4?t=token
```
- ✅ 拖进度条正常（Range 请求带 token + cookie）
- ✅ 不反复加载
- ❌ DOM 暴露代理 URL
- ❌ Token 有效期内可被复制下载

#### 方案 B：Blob + 撤销（尝试）
```javascript
video.src = blobURL;
el.addEventListener('loadeddata', () => URL.revokeObjectURL(blobURL));
```
- ✅ DOM 只有 blob://
- ❌ 撤销后解码器读取失败
- ❌ 反复重新加载（浏览器重试 → blob 失效 → 再触发加载 → 死循环）

#### 方案 C：Blob + loadeddata 撤销（失败原因）
```javascript
video.src = blobURL;
el.addEventListener('loadeddata', () => URL.revokeObjectURL(blobURL));
el.addEventListener('error', () => reviveVideoBlob(el)); // ❌ 关键错误！
```
- ❌ 撤销后 video 触发 error → revive → 新 blob 又活了 → 用户看到新 blob → 复制能打开
- ❌ error 事件过于敏感，撤销后立即触发

#### 方案 D：Blob + 不撤销（✅ 最终方案——浏览器限制）

**结论：视频 Blob URL 不能撤销。** 图片解码器是一次性的（解码完不再读源数据），视频解码器是流式的（持续读源数据做缓冲）。撤销 blob = 解码器断粮 = 必然卡住。

已尝试的 8 种撤销+复活方案全部失败（loadeddata/canplaythrough/stalled/error/Object.defineProperty/延迟撤销等），根本原因是浏览器底层架构决定的。

```javascript
// 视频：不撤销
if (isVideo) {
    // blob: URL 是浏览器本地内存引用，不是网络地址
    // curl/wget 无法访问，外部设备无法访问
    // DEBUG=false 时 F12/右键被封，无法提取 URL
}
```

**生产环境防护：**

| 攻击 | 阻断 |
|------|------|
| curl/wget blob:// | ❌ 不是网络协议 |
| 分享 blob URL 给别人 | ❌ 本地内存，设备绑定 |
| F12 查看 src | ❌ DEBUG=false 拦截 |
| 右键复制视频地址 | ❌ DEBUG=false 拦截 |

### 7.3 视频 Blob 安全性分析

```
blob:http://localhost:8080/xxx  ← 这是浏览器内部引用，不是 HTTP 地址！

┌──────────────────────────────────────────────────────────┐
│ 攻击方式                    │ 结果                       │
├──────────────────────────────────────────────────────────┤
│ curl/wget blob://xxx        │ ❌ 不是网络协议，无效       │
│ 分享 URL 给别人              │ ❌ 别人浏览器无此 Blob 数据 │
│ 同一浏览器新标签页           │ ⚠️ 能打开（共享内存）      │
│ 原始页面关闭后               │ ❌ beforeunload 撤销       │
│ 开发者工具 Network           │ ✅ 能看到 fetch 请求       │
│                              │   （但 DEBUG=false 禁 F12）│
└──────────────────────────────────────────────────────────┘
```

> **结论**：视频 Blob URL 在新标签页能打开是浏览器的本地行为，不是网络漏洞。真正的外部攻击（curl、分享他人）完全无效。

### 7.4 视频完整代码路径

```
服务端：
  InjectVideoDimensions → BuildMediaMapWith
  原始: <video src="/static/uploads/file.mp4" controls="">
  输出: <video data-mid="b7d4e9" controls="" width="640" height="640">

客户端：
  IntersectionObserver → loadMediaElement → fetch → blob → 
  video.src = blobURL → 不撤销 → 存入 videoBlobs[]
  DOM: <video src="blob://xxx" class="blob-loaded">

灯箱（视频）：
  loadBlobMedia → fetch → 返回新 blobURL → 展示后 revoke
```

---

## 8. 生产环境保护

### 8.1 配置

```env
DEBUG=false   # 生产环境，启用前端保护
DEBUG=true    # 开发环境，正常使用 F12
```

### 8.2 protect.js 防护清单

```javascript
// ① 禁用右键
document.addEventListener('contextmenu', e => e.preventDefault());

// ② 禁用文字选择/复制/剪切
document.addEventListener('selectstart', e => e.preventDefault());
document.addEventListener('copy', e => e.preventDefault());

// ③ 拦截开发者工具快捷键
// F12, Ctrl+Shift+I/J/C, Ctrl+U, Ctrl+S

// ④ DevTools 检测（计时旁路）
setInterval(() => {
    var start = performance.now();
    // 触发 getter，DevTools 打开时代码执行明显变慢
    var elapsed = performance.now() - start;
    if (elapsed > 100) {
        // 检测到 → 遮挡页面
        document.body.innerHTML = '请关闭开发者工具后刷新页面';
    }
}, 1500);

// ⑤ 定期清空控制台
setInterval(() => console.clear(), 3000);
```

### 8.3 加载条件

```html
<!-- footer.html -->
{{if not .Cfg.Debug}}<script src="protect.js"></script>{{end}}
```

- ✅ 公开页面：`DEBUG=false` 时加载
- ✅ 管理后台：永远不加载（使用 `admin_footer.html`）
- ✅ 搜索引擎：不执行 JS，不受影响

---

## 9. SEO 兼容

Blob 加载对搜索引擎不友好（爬虫不执行 JS），因此 SEO 相关标签保留直接 URL：

| SEO 元素 | URL 类型 | 位置 |
|----------|----------|------|
| Open Graph `og:image` | 直接 URL | `<head>` |
| Sitemap `<image:image>` | 直接 URL | `/sitemap.xml` |
| JSON-LD `image` | 直接 URL | `<script type="application/ld+json">` |
| RSS `<enclosure>` | 直接 URL | `/rss.xml` |

```html
<!-- header.html -->
<meta property="og:image" content="/static/uploads/file.jpg">
<!-- ↑ 搜索引擎用，不走 Blob -->
```

这些直接 URL 仍然受防盗链保护（Referer 校验）。

---

## 10. 遇到的问题与解决

### 问题 1：Windows 路径分隔符

**现象**：`filepath.Clean("/media/uploads/file.jpg")` → `\media\uploads\file.jpg`

**原因**：`filepath` 包使用 OS 原生分隔符，Windows 上是 `\`。

**修复**：URL 路径操作统一使用 `path.Clean()`，文件系统操作使用 `filepath.Join()`。

```go
fp = path.Clean(fp)           // URL 路径
filepath.Join(s.baseDir, key) // 文件系统路径
```

---

### 问题 2：图片灯箱显示空白

**现象**：点击图片预览，灯箱显示黑屏。

**原因**：`data-mid` 在首次加载后被 `removeAttribute` 删除，灯箱 `show()` 函数只检查 `data-mid` 属性是否存在来判断是否为 Blob 图片。属性已删除 → 判断为非 Blob → 使用了已被撤销的 `img.src`。

**修复**：
1. 删除 `data-mid` 前存到 `el._ofoMid`（JS 属性，DOM 不可见）
2. 灯箱判断条件加上 `img.classList.contains('blob-loaded')`

```javascript
// 修复前
var isBlob = img.getAttribute('data-mid') !== null;  // ❌ 已被删除

// 修复后
var isBlob = img.getAttribute('data-mid') !== null
          || img.classList.contains('blob-loaded');   // ✅ 双重判断
```

---

### 问题 3：视频反复重新加载

**现象**：Network 面板中同一视频的 Blob 请求出现多次。

**原因**：撤销 Blob URL 后，视频解码器无法读取数据 → 触发 error → 重新触发加载流程 → 死循环。

**修复**：视频不撤销 Blob URL，页面关闭时统一清理。

```javascript
if (isVideo) {
    videoBlobs.push(result.blobURL);  // 存活整页生命周期
} else {
    el.addEventListener('load', () => revoke(result.blobURL));  // 图片立即撤销
}
```

---

### 问题 4：顺序 ID 可被遍历

**现象**：`data-mid="0"`, `data-mid="1"` 可被脚本遍历。

**修复**：用 `crypto/rand` 生成随机 8 位 hex ID，URL 映射从数组改为对象。

```go
// 修复前
type MediaMap struct { URLs []string }
func (mm *MediaMap) Add(url string) int { ... return index }

// 修复后
type MediaMap struct { entries map[string]string }
func (mm *MediaMap) Add(url string) string { return randomMID() }
```

---

### 问题 5：代理 URL 无 Cookie 可被直接访问

**现象**：复制视频代理 URL 到 curl → 200，可下载。

**修复**：`/media/` 接口增加 Cookie 校验。

```go
sessCookie, _ := c.Cookie("ofo_m")
if sessCookie == "" || !validatePageToken(sessCookie, ...) {
    c.AbortWithStatus(403)
    return
}
```

Cookie 由 JS 在页面加载时设置：`document.cookie = "ofo_m=...; SameSite=Strict"`

---

### 问题 6：跨模板函数共享状态

**现象**：`thumbnailImg`、`lazyImages` 等模板函数各自独立调用，无法共享同一个 MediaMap。

**修复**：使用 `atomic.Pointer[MediaMap]` 存储当前请求的 MediaMap。首个模板函数（`mediaConfigScript`）初始化，后续函数通过 `CurrentMediaMap()` 访问。

---

### 问题 7：CSP 阻止 Blob URL 加载

**现象**：浏览器控制台报 CSP 违规，Blob 图片不显示。

**修复**：CSP 头增加 `blob:` 白名单。

```
img-src 'self' data: blob:;
media-src 'self' blob:;
```

---

### 问题 8：旧进程残留

**现象**：修改代码 rebuild 后，仍然看到旧版本行为。

**原因**：Windows 上 `taskkill` 在 Git Bash 中可能失败，旧进程仍占用端口。

**修复**：
```bash
# 确认进程
netstat -ano | grep ":8080" | grep LISTENING
# 按 PID 强杀
taskkill //F //PID <PID>
# 确认端口释放后再启动
```

---

### 问题 9：Object.defineProperty 劫持 video.src 失败

**现象**：尝试用 `Object.defineProperty(video, 'src', {get: fn})` 返回假值隐藏真实 blob URL。

**原因**：`HTMLMediaElement.src` 是浏览器原生 IDL 定义的访问器属性，`Object.defineProperty` 无法覆盖。

**教训**：不要尝试劫持 DOM 原生属性 getter/setter。

---

### 问题 10：撤销视频 Blob 的所有尝试均失败

**尝试过的方案**：loadeddata 撤销 / canplaythrough 撤销 / stalled 复活 / seeking 复活 / 延迟再撤销 / Object.defineProperty / 等 8 种组合，全部失败。

**根本原因**：浏览器视频解码器是流式的，需要持续读取源数据做缓冲。图片解码器是一次性的（`load` 后不再读源数据），所以图片可以撤销，视频不行。这是底层架构决定的，不是代码问题。

**最终方案**：视频 Blob 不撤销。生产环境靠 `DEBUG=false` 封死 F12/右键来阻止提取 URL。

---

## 11. 配置参考

```env
# .env 文件

# ---- 媒体保护 ----
# 是否启用 Blob 方式加载（关闭后恢复直接 URL）
MEDIA_PROTECTION=true

# HMAC 签名密钥（留空自动用 ADMIN_PASSWORD 派生）
MEDIA_SECRET=

# 代理 URL 有效期（秒），建议生产环境 300
MEDIA_TOKEN_TTL=1800

# ---- 运行模式 ----
# true=开发（F12 正常），false=生产（禁止右键/F12/复制）
DEBUG=true

# ---- 防盗链 ----
HOTLINK_PROTECTION=true
STATIC_RATE_LIMIT=20
```

### 建议的生产配置

```env
MEDIA_PROTECTION=true
MEDIA_SECRET=<随机 64 位 hex 字符串>
MEDIA_TOKEN_TTL=300   # 5 分钟
DEBUG=false           # 禁止 F12
HOTLINK_PROTECTION=true
```

---

## 12. 安全边界

> Blob 方案不是绝对安全，而是**大幅提高攻击成本**。

| 攻击方式 | 防护效果 | 说明 |
|----------|----------|------|
| 页面源码找 URL | ✅ 完全阻止 | HTML 只有 `data-mid` 随机 ID |
| curl/wget 代理 URL | ✅ 完全阻止 | 无 Cookie → 403 |
| curl/wget 直接 URL | ✅ 双重防护 | 防盗链 + Token + Cookie |
| 右键另存为 | ✅ 生产环境阻止 | DEBUG=false 禁右键 |
| F12 Network 面板 | ✅ 生产环境阻止 | DEBUG=false 禁 F12 |
| 复制 img.src 到新标签 | ✅ 图片阻止 | Blob 已撤销 |
| 复制 video.src 到新标签 | ✅ loadeddata 后撤销 | stalled+seeking 时才短暂复活 1 秒 |
| 截图/录屏 | ❌ 无法阻止 | OS 级别操作 |
| 浏览器插件截获 | ❌ 无法阻止 | 插件权限高于网页 |
| DrissionPage/Selenium | ⚠️ 部分阻止 | 自动化工具可模拟浏览器 |

### 后续可加强的方向

1. **加密 JS 文件**：对 `media-blob.js` 和 `__OFO_MEDIA__.urls` 做混淆/加密，增加逆向难度
2. **IP 绑定 Token**：Token 加入客户端 IP hash，防止同 Cookie 跨 IP 使用
3. **请求频率限制**：`/media/` 接口按 IP + Token 做更细粒度的频率限制
4. **服务端水印**：图片动态加水印，即使被下载也有溯源标识
5. **Referer 白名单**：`/media/` 接口只允许本站 Referer

---

## 核心文件清单

| 文件 | 职责 |
|------|------|
| `handlers/media.go` | 代理接口、Token 生成/校验、MediaMap、URL 转换 |
| `handlers/admin.go` | ThumbnailMidImage、InjectImageDimensions |
| `router/router.go` | 路由、模板函数注册 |
| `middleware/security.go` | CSP 头 |
| `config/config.go` | MEDIA_PROTECTION、MEDIA_SECRET、MEDIA_TOKEN_TTL、DEBUG |
| `static/js/media-blob.js` | Blob 加载、Observer、灯箱集成 |
| `static/js/main.js` | 灯箱 show/close、加载更多集成 |
| `static/js/protect.js` | 生产环境 F12/右键/复制拦截 |
| `templates/header.html` | mediaConfigScript 注入 |
| `templates/footer.html` | mediaURLScript + protect.js 加载 |
| `templates/home.html` | 卡片缩略图 data-mid |
| `templates/post.html` | lazyImages + 懒加载 JS |
