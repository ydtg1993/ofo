# JavaScript 端加密技术详解

> 本文聚焦前端 JS 侧的加密/解密实现，是 [Blob 媒体保护方案](blob-media-protection.md) 的 JS 技术补充篇。
> 核心问题：**服务端注入的 URL 映射表如何在 HTML 中传输而不被直接读取？**

---

## 目录

- [1. 问题背景](#1-问题背景)
- [2. 加密方案选型](#2-加密方案选型)
- [3. 整体数据流](#3-整体数据流)
- [4. 服务端加密](#4-服务端加密)
- [5. 客户端解密](#5-客户端解密)
- [6. Web Crypto API 使用要点](#6-web-crypto-api-使用要点)
- [7. Service Worker 协同防护](#7-service-worker-协同防护)
- [8. 密钥生命周期](#8-密钥生命周期)
- [9. 安全分析](#9-安全分析)
- [10. 踩坑记录](#10-踩坑记录)
- [11. 关键代码片段](#11-关键代码片段)

---

## 1. 问题背景

在 Blob 媒体保护方案中，页面 HTML 里不出现真实文件 URL，取而代之的是 `data-mid="a3f2c1"` 这样的随机 ID。JS 需要一份映射表来将 ID 解析为代理 URL：

```javascript
// JS 需要的映射表
{"a3f2c1": "/media/uploads/photo.jpg?t=1a2b3c:def456...", ...}
```

**如果这份映射表以明文注入 HTML**：

```html
<script>
window.__OFO_MEDIA__.urls = {"a3f2c1": "/media/uploads/photo.jpg?t=..."};
</script>
```

任何人查看页面源码就能拿到所有代理 URL，Token 有效期内可批量下载。这削弱了第 1 层防护（HTML 隐藏）。

**解决方案**：对映射表做 **AES-256-GCM 加密**，密钥通过独立渠道注入（另一个 `<script>` 标签），解密在浏览器中用 **Web Crypto API** 完成。

---

## 2. 加密方案选型

### 为什么是 AES-256-GCM？

| 候选方案 | 优点 | 缺点 | 结论 |
|----------|------|------|------|
| **明文 JSON** | 简单 | 源码可见，等于没防 | ❌ |
| Base64 编码 | 不可读 | 不是加密，秒解 | ❌ |
| 自定义 XOR | 轻量 | 无认证，可被篡改 | ❌ |
| **AES-256-GCM** | 认证加密、浏览器原生支持 | 需要 Web Crypto API | ✅ |
| AES-256-CBC | 兼容性好 | 无内置认证，需额外 HMAC | ⚠️ |
| ChaCha20-Poly1305 | 移动端快 | Web Crypto 支持不如 AES 广 | ⚠️ |

**选择 AES-256-GCM 的核心理由**：

1. **认证加密（AEAD）**：解密时自动验证完整性，篡改密文 → 解密失败
2. **浏览器原生支持**：`crypto.subtle.decrypt({name: 'AES-GCM'}, ...)` 无需引入任何第三方库
3. **服务端 Go 对应**：`crypto/aes` + `crypto/cipher` 标准库完美支持
4. **性能足够**：URL 映射表通常几百字节到几 KB，AES-GCM 开销可忽略

### 为什么还需要加密？

```
防护层级：
  第 1 层：HTML 中 data-mid 随机 ID 替代 URL     ← 防页面源码
  第 2 层：URL 映射表 AES-256-GCM 加密存储       ← 防脚本提取  ← 本文重点
  第 3 层：代理 URL 带 HMAC Token               ← 防直接访问
  第 4 层：代理接口要求 Cookie                    ← 防 curl/wget
  第 5 层：Blob 加载后撤销 Blob URL              ← 防复制粘贴
```

即使攻击者能执行 JS（如通过浏览器控制台），也需要先找到解密密钥才能还原 URL 映射表。

---

## 3. 整体数据流

```
┌─────────────────────────────────────────────────────────────────────┐
│                              服务端                                  │
│                                                                     │
│  URL 映射表                          AES-256 密钥（每页随机）         │
│  {"a3f2c1":"/media/...", ...}        32 字节 crypto/rand            │
│       │                                    │                        │
│       ▼                                    │                        │
│  aesEncrypt(plain, key)                    │                        │
│       │                                    │                        │
│       ▼                                    │                        │
│  base64(nonce[12B] ||                      │                        │
│         ciphertext ||                      │                        │
│         tag[16B])                          │                        │
│       │                                    │                        │
│       ▼                                    ▼                        │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                        HTML 页面                                ││
│  │                                                                 ││
│  │  <script>                                                       ││
│  │    window.__OFO_MEDIA__ = {                    ← 密钥 + 配置     ││
│  │      enabled: true,                                             ││
│  │      k: "Base64密钥明文",                     ← 攻击面：页面源码可见││
│  │      d: "Base64密文",                         ← 加密的映射表     ││
│  │    };                                                           ││
│  │  </script>                                                      ││
│  └─────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                              浏览器                                  │
│                                                                     │
│  ① 读取 window.__OFO_MEDIA__.k (Base64 Key)                        │
│  ② 读取 window.__OFO_MEDIA__.d (Base64 Ciphertext)                 │
│       │                                    │                        │
│       ▼                                    ▼                        │
│  ③ crypto.subtle.importKey("raw", keyBytes, "AES-GCM", ...)        │
│       │                                                             │
│       ▼                                                             │
│  ④ crypto.subtle.decrypt({name:"AES-GCM", iv:nonce}, key, ct)      │
│       │                                                             │
│       ▼                                                             │
│  ⑤ JSON.parse(plaintext) → URL 映射表                               │
│       │                                                             │
│       ▼                                                             │
│  ⑥ IntersectionObserver → fetch → blob → revoke                    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 4. 服务端加密

### 4.1 Go 侧 aesEncrypt

```go
// handlers/media.go

func aesEncrypt(plain, key []byte) (string, error) {
    block, err := aes.NewCipher(key)       // key 必须是 32 字节（AES-256）
    if err != nil {
        return "", err
    }
    gcm, err := cipher.NewGCM(block)       // GCM 模式
    if err != nil {
        return "", err
    }
    nonce := make([]byte, gcm.NonceSize()) // 12 字节
    if _, err := rand.Read(nonce); err != nil {
        return "", err
    }
    // Seal: nonce || ciphertext || tag (GCM 自动追加 16 字节认证标签)
    out := gcm.Seal(nonce, nonce, plain, nil)
    return base64.StdEncoding.EncodeToString(out), nil
}
```

**GCM Seal 的输出格式**：

```
┌──────────────┬─────────────────────┬──────────────────┐
│  Nonce (12B) │  Ciphertext (变长)   │  Auth Tag (16B)  │
└──────────────┴─────────────────────┴──────────────────┘
                    全部做 Base64 编码后注入 HTML
```

### 4.2 密钥生成

```go
// 每个页面渲染时生成全新随机密钥
aesKey := make([]byte, 32) // AES-256 = 32 字节 = 256 位
rand.Read(aesKey)          // crypto/rand，非 math/rand
```

**为什么每页换密钥？**
- 即使攻击者通过某种方式获取了某一页的密钥，也只能解密该页的 URL 映射表
- 刷新页面后密钥变化，之前的密钥作废（映射表本身也随 Token 过期而失效）

### 4.3 HTML 注入

```go
func BuildMediaConfigScript(cfg *config.Config) string {
    // ...
    aesKey := make([]byte, 32)
    rand.Read(aesKey)
    keyB64 := base64.StdEncoding.EncodeToString(aesKey)
    setPageAESKey(aesKey) // 存到 atomic.Pointer，供后续模板函数使用

    return fmt.Sprintf(
        `<script>document.cookie="ofo_m=%s;path=/;max-age=%d;SameSite=Strict";`+
        `window.__OFO_MEDIA__={enabled:true,ttl:%d,k:%q};</script>`,
        pageToken, cfg.MediaTokenTTL, cfg.MediaTokenTTL, keyB64,
    )
}
```

注入结果（页面源码中）：

```html
<script>document.cookie="ofo_m=67a1b2c3:def456...;path=/;max-age=1800;SameSite=Strict";window.__OFO_MEDIA__={enabled:true,ttl:1800,k:"Base64密钥..."};</script>
<script>window.__OFO_MEDIA__=window.__OFO_MEDIA__||{};window.__OFO_MEDIA__.d="Base64密文...";</script>
```

**注意**：密钥以 Base64 明文出现在页面源码中。这是无法避免的——浏览器必须能读到密钥才能解密。我们的目标是**增加攻击复杂度**，不是实现绝对安全。

---

## 5. 客户端解密

### 5.1 整体流程

```javascript
// static/js/media-blob.js

(function () {
    'use strict';

    var CFG = window.__OFO_MEDIA__;
    if (!CFG || !CFG.enabled) return;

    // ① 注册 Service Worker（拦截 blob: 导航）
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.register('/sw.js', { scope: '/' });
    }

    var URLMAP = {};

    // ② AES-256-GCM 解密
    if (!CFG.d || !CFG.k) return;
    aesDecrypt(CFG.d, CFG.k).then(function (urls) {
        URLMAP = urls;   // 解密成功后才设置 URLMAP
        init();           // 然后启动 Observer
    }).catch(function (err) {
        console.warn('ofo: decrypt failed', err);
    });

    // ③ 后续使用 URLMAP 查找代理 URL...
})();
```

**关键设计**：`init()` 在解密 **成功之后** 才调用。如果解密失败（密钥错误、密文被篡改），整个媒体加载流程不会启动，所有媒体元素保持 `data-mid` 状态，不会加载任何 blob。

### 5.2 aesDecrypt 实现

```javascript
function aesDecrypt(b64Data, b64Key) {
    // Step 1: Base64 → Uint8Array
    var raw = Uint8Array.from(atob(b64Data), function (c) {
        return c.charCodeAt(0);
    });
    var keyBytes = Uint8Array.from(atob(b64Key), function (c) {
        return c.charCodeAt(0);
    });

    // Step 2: 分离 nonce 和 ciphertext
    var nonce = raw.slice(0, 12);   // GCM nonce = 12 字节
    var ct = raw.slice(12);         // 剩余 = ciphertext || tag

    // Step 3: 导入密钥
    return crypto.subtle.importKey(
        'raw',                       // 原始字节格式
        keyBytes,                    // 32 字节密钥
        { name: 'AES-GCM' },         // 算法标识
        false,                       // 不可导出
        ['decrypt']                  // 只用于解密
    ).then(function (k) {
        // Step 4: 解密
        return crypto.subtle.decrypt(
            { name: 'AES-GCM', iv: nonce },
            k,
            ct
        );
    }).then(function (plain) {
        // Step 5: 解码为 JSON
        return JSON.parse(new TextDecoder().decode(plain));
    });
}
```

---

## 6. Web Crypto API 使用要点

### 6.1 API 调用链

```
Base64 字符串
    │
    ▼ atob()
Uint8Array (原始字节)
    │
    ├── raw.slice(0, 12)  → nonce
    └── raw.slice(12)     → ciphertext + tag
    │
    ▼ crypto.subtle.importKey("raw", keyBytes, "AES-GCM", false, ["decrypt"])
CryptoKey 对象（不可提取）
    │
    ▼ crypto.subtle.decrypt({name: "AES-GCM", iv: nonce}, key, ct)
ArrayBuffer（解密后的明文）
    │
    ▼ new TextDecoder().decode()
字符串
    │
    ▼ JSON.parse()
URL 映射对象
```

### 6.2 `importKey` 参数说明

```javascript
crypto.subtle.importKey(
    'raw',           // format: 原始字节，非 JWK/PKCS8
    keyBytes,        // keyData: 32 字节 Uint8Array
    { name: 'AES-GCM' }, // algorithm
    false,           // extractable: false → 密钥不可被 JS 导出
    ['decrypt']      // usages: 只能解密，不能加密
)
```

- `extractable: false` 是关键安全措施：导入了密钥后，无法通过 `crypto.subtle.exportKey` 将密钥导回原始字节。JS 代码无法获取密钥内容。
- `usages: ['decrypt']`：即使攻击者在控制台调用 `importKey` 也无法将此密钥用于加密（假造数据）。

### 6.3 GCM 认证标签

AES-GCM 解密时会自动验证末尾 16 字节的认证标签。如果密文被篡改（哪怕一个 bit），`crypto.subtle.decrypt` 会抛出 `OperationError`。

这意味着：
- 攻击者无法通过修改密文来注入伪造的 URL
- 任何密文篡改都会导致整个映射表解密失败，图片/视频全部不加载

### 6.4 浏览器兼容性

| 浏览器 | Web Crypto API | AES-GCM |
|--------|---------------|---------|
| Chrome 37+ | ✅ | ✅ |
| Firefox 34+ | ✅ | ✅ |
| Safari 11+ | ✅ | ✅ |
| Edge 79+ | ✅ | ✅ |
| IE 11 | ❌ | ❌ |

如果浏览器不支持 Web Crypto API（如 IE11），解密失败 → `catch` 捕获 → 打印警告 → 媒体不加载。页面其余功能不受影响。

---

## 7. Service Worker 协同防护

### 7.1 为什么需要 Service Worker？

Blob URL 有一个特性：同一浏览器的新标签页可以打开同一个 blob URL。即使用户复制了 `blob:http://localhost:8080/xxx` 地址到新标签页，也能看到图片。

Service Worker 充当网关，拦截对 `blob:` URL 的导航请求：

```javascript
// static/js/sw.js
self.addEventListener('fetch', function (event) {
    var url = event.request.url;
    // 只拦截 blob: 的导航请求（地址栏输入），不影响 <img>/<video> 的内部加载
    if (url.startsWith('blob:') && event.request.mode === 'navigate') {
        event.respondWith(new Response(
            '<!DOCTYPE html>...⛔ Blocked...</html>',
            { status: 403, headers: { 'Content-Type': 'text/html; charset=utf-8' } }
        ));
    }
});
```

### 7.2 `request.mode` 区分

| mode | 含义 | SW 行为 |
|------|------|---------|
| `navigate` | 地址栏导航、链接点击 | **拦截**，返回 403 |
| `no-cors` / `cors` | `<img src>`、`<video src>` 等子资源加载 | **放行**，不经过 SW 的 fetch 事件 |
| `same-origin` | 同源子资源 | **放行** |

> **重要**：`<img>` 和 `<video>` 的 blob URL 加载不经过 Service Worker 的 `fetch` 事件。它们由浏览器内部的 blob 解析器处理，直接从内存读取。所以 SW 只拦截地址栏输入，不影响正常页面渲染。

### 7.3 注册时机

SW 在页面加载时立即注册，不等待解密完成：

```javascript
if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js', { scope: '/' });
}
```

---

## 8. 密钥生命周期

```
┌──────────────────────────────────────────────────────────────────┐
│                        密钥生命周期                               │
│                                                                  │
│  服务端                                                          │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ ① 请求到达 → Gin 开始渲染页面                              │  │
│  │ ② BuildMediaConfigScript() 生成 32 字节随机 AES Key       │  │
│  │ ③ setPageAESKey(key) → 存 atomic.Pointer                 │  │
│  │ ④ 注入 <script> window.__OFO_MEDIA__.k = base64(key)     │  │
│  │ ⑤ 后续模板函数通过 getPageAESKey() 获取 key               │  │
│  │ ⑥ MediaMap.Script(key) → aesEncrypt + 注入 d=...         │  │
│  │ ⑦ 页面 HTML 返回，key 在此请求生命周期后不再使用           │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  浏览器                                                          │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ ① 页面加载，读取 window.__OFO_MEDIA__.k                    │  │
│  │ ② atob(k) → keyBytes → crypto.subtle.importKey(...)      │  │
│  │ ③ 导入成功后，CryptoKey 对象存在于浏览器内存              │  │
│  │ ④ 解密完成后，原始 keyBytes 可被 GC 回收                  │  │
│  │ ⑤ CryptoKey 对象随页面关闭/刷新销毁                        │  │
│  │                                                             │  │
│  │ 注意：keyBytes 以 Uint8Array 形式短暂存在于 JS 堆内存。    │  │
│  │ 有 F12 权限的攻击者可以在 importKey 之前断点读取。         │  │
│  │ 这就是为什么生产环境要封 F12。                              │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

---

## 9. 安全分析

### 9.1 威胁模型

| 攻击者能力 | 能否获取原图？ | 攻击路径 |
|------------|:---:|------|
| 查看页面源码 | ❌ | HTML 只有 `data-mid` 随机 ID 和 Base64 密文 |
| 从源码获取 Base64 密文和密钥 | ⚠️ | 密钥在源码中是明文 Base64。能拿到，但需要进一步操作 |
| 在控制台解密 | ⚠️ | 需要手动写解密代码。生产环境封 F12 增加门槛 |
| 复制 blob: URL 到新标签 | ❌ | Service Worker 拦截导航请求 |
| curl/wget 代理 URL | ❌ | 无 Cookie → 403 |
| 中间人截获 HTML | ⚠️ | HTTPS 下无法截获；HTTP 下可获取密钥和密文 |
| 浏览器插件 | ⚠️ | 插件可读取 DOM 和 JS 变量，无法防御 |

### 9.2 密钥为何以明文传输？

这是必须做出的取舍。浏览器必须能解密，所以密钥必须出现在浏览器能读到的地方。我们把密钥放在 `<script>` 标签中，和加密数据在同一个页面上——这不是「把钥匙和锁放在一起」的愚蠢设计，而是：

1. **提高自动爬虫的门槛**：爬虫一般只解析 HTML，不执行 JS，更不会调用 Web Crypto API
2. **降低批量提取效率**：人工查看页面源码无法直接拿到 URL 列表，必须写解密脚本
3. **每页密钥不同**：即使写了解密脚本，也只能解当前页面的映射表

### 9.3 可进一步加强的方向

1. **JS 混淆**：对 `media-blob.js` 做变量名混淆、控制流平坦化，增加逆向难度
2. **密钥分段注入**：密钥拆分成多段，在不同位置注入，JS 中拼接（对抗简单正则提取）
3. **WASM 解密**：将解密逻辑编译为 WASM，进一步增加逆向成本
4. **短期密钥 + 服务端时间绑定**：密钥包含当前时间窗口 hash，过期后即使拿到密钥也无法解密新数据

---

## 10. 踩坑记录

### 坑 1：Base64 编解码不一致

**现象**：Go 加密、JS 解密偶尔失败。

**原因**：Go 的 `base64.StdEncoding` 使用标准 Base64（`+` `/` `=`），JS 的 `atob()` 也使用标准 Base64。但如果混淆了 URL-safe Base64（`-` `_` 无 `=`），解码会失败。

**解决**：服务端和客户端统一使用标准 Base64（`StdEncoding`）。

### 坑 2：GCM nonce 长度

**现象**：解密报错 `OperationError`。

**原因**：GCM 的 nonce 标准长度是 12 字节（96 位），但 `cipher.NewGCM` 的 `NonceSize()` 返回值可能在某些平台上不是 12。如果用错 nonce 长度，浏览器端 `crypto.subtle.decrypt` 会拒绝。

**解决**：始终用 `gcm.NonceSize()` 生成 nonce，而不是硬编码 12。Go 和浏览器的 GCM 实现都遵循 NIST SP 800-38D，nonce 为 12 字节。

### 坑 3：`atob` 不支持 Unicode

**现象**：解密后的 JSON 中文文件名乱码。

**原因**：`atob()` 只能处理 Latin-1 字符。不过本项目 URL 映射表全是 ASCII（路径 + hex），不受此限。如果是更通用的场景，需要用 `TextEncoder`/`TextDecoder`。

**解决**：本项目使用 `Uint8Array.from(atob(s), c => c.charCodeAt(0))`，将 Base64 解码为字节数组，再用 `TextDecoder` 解码为字符串，绕过了 `atob` 的 Unicode 限制。

### 坑 4：`crypto.subtle` 只在安全上下文可用

**现象**：`http://localhost` 上能解密，但部署到 `http://192.168.x.x` 上 `crypto.subtle` 为 `undefined`。

**原因**：Web Crypto API 的 `crypto.subtle` 只在安全上下文（HTTPS 或 localhost）中可用。HTTP 下 `crypto.subtle` 为 `undefined`。

**解决**：生产环境必须 HTTPS，或使用 `localhost` 开发。

### 坑 5：解密失败后没有回退

**现象**：解密失败后，图片完全不显示，没有任何提示。

**原因**：代码设计中，解密失败的 `catch` 只有 `console.warn`。如果密钥/密文有问题，用户看到的是没有图片的页面。

**解决**：这是有意为之——静默失败避免向攻击者暴露信息。生产环境应该在部署前验证加密解密链路正常。

---

## 11. 关键代码片段

### 11.1 完整解密调用链（一览）

```javascript
// 从 window 读取配置（服务端注入）
var CFG = window.__OFO_MEDIA__;  // { enabled:true, k:"...", d:"..." }

// 解密
aesDecrypt(CFG.d, CFG.k).then(function (urls) {
    URLMAP = urls;   // {"a3f2c1": "/media/uploads/photo.jpg?t=token", ...}
    init();           // 启动 IntersectionObserver
});

function aesDecrypt(b64Data, b64Key) {
    var raw = Uint8Array.from(atob(b64Data), c => c.charCodeAt(0));
    var keyBytes = Uint8Array.from(atob(b64Key), c => c.charCodeAt(0));
    var nonce = raw.slice(0, 12);
    var ct = raw.slice(12);

    return crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, ['decrypt'])
        .then(k => crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce }, k, ct))
        .then(plain => JSON.parse(new TextDecoder().decode(plain)));
}
```

### 11.2 Go 加密对照

```go
func aesEncrypt(plain, key []byte) (string, error) {
    block, _ := aes.NewCipher(key)         // key = 32 bytes
    gcm, _ := cipher.NewGCM(block)         // GCM mode
    nonce := make([]byte, gcm.NonceSize()) // 12 bytes
    rand.Read(nonce)
    out := gcm.Seal(nonce, nonce, plain, nil)
    return base64.StdEncoding.EncodeToString(out), nil
}
```

### 11.3 防篡改验证

```javascript
// AES-GCM 的认证标签在 crypto.subtle.decrypt 内部自动验证。
// 如果密文被篡改，会抛出 OperationError，天然防篡改。
aesDecrypt(CFG.d, CFG.k).catch(function (err) {
    // 密文被篡改时会走到这里
    console.warn('ofo: decrypt failed', err);
    // URLMAP 保持为 {}，所有媒体不会加载
});
```

---

## 相关文件

| 文件 | 职责 |
|------|------|
| `handlers/media.go` | Go 侧 AES-256-GCM 加密、密钥生成、HTML 注入 |
| `static/js/media-blob.js` | JS 侧 AES-256-GCM 解密、Blob 加载 |
| `static/js/sw.js` | Service Worker：拦截 blob: URL 导航 |
| `static/js/protect.js` | 生产环境保护：禁止 F12/右键/复制 |
| `router/router.go` | 模板函数注册、`BuildMediaConfigScript` 调用 |
| `docs/blob-media-protection.md` | Blob 媒体保护总体架构文档 |

---

> **总结**：JS 加密的核心思路是用 AES-256-GCM 认证加密保护 URL 映射表，让页面源码中不出现可读的 URL 列表。配合每页随机密钥、Web Crypto API 的 `extractable: false`、Service Worker 的 blob 导航拦截、以及生产环境的 F12 禁用，构成多层次的媒体保护体系。
