/**
 * 蹬车摸鱼 — main.js
 * 主题切换 / 信息流无限滚动 / 全屏进度 / 摸鱼模式 / 老板键 / 后台编辑器
 */
(function () {
    'use strict';

    // ==========================================
    // 主题切换
    // ==========================================
    var themeKey = 'ofo-theme';
    function getTheme() { return localStorage.getItem(themeKey) || 'light'; }
    function setTheme(t) {
        document.documentElement.setAttribute('data-theme', t);
        localStorage.setItem(themeKey, t);
        var btn = document.getElementById('theme-toggle');
        if (btn) btn.querySelector('.theme-toggle__icon').innerHTML = t === 'dark' ? '&#x263D;' : '&#x2600;';
    }
    setTheme(getTheme());
    var themeBtn = document.getElementById('theme-toggle');
    if (themeBtn) themeBtn.addEventListener('click', function () {
        setTheme(getTheme() === 'dark' ? 'light' : 'dark');
    });

    // ==========================================
    // 移动端导航
    // ==========================================
    var navToggle = document.getElementById('nav-toggle');
    var navLinks = document.getElementById('nav-links');
    if (navToggle && navLinks) {
        navToggle.addEventListener('click', function () { navLinks.classList.toggle('open'); });
        navLinks.querySelectorAll('a').forEach(function (a) {
            a.addEventListener('click', function () { navLinks.classList.remove('open'); });
        });
    }

    // ==========================================
    // 代码复制按钮
    // ==========================================
    document.querySelectorAll('pre').forEach(function (pre) {
        var btn = document.createElement('button');
        btn.className = 'code-copy-btn';
        btn.textContent = '复制';
        btn.addEventListener('click', function () {
            navigator.clipboard.writeText(pre.textContent).then(function () {
                btn.textContent = '已复制!';
                setTimeout(function () { btn.textContent = '复制'; }, 2000);
            });
        });
        pre.appendChild(btn);
    });

    // ==========================================
    // 信息流 — AJAX 无限滚动
    // ==========================================
    (function () {
        var feedEl = document.getElementById('post-feed');
        if (!feedEl) return;

        var currentPage = 1;
        var loading = false;
        var hasMore = true;
        var activeCategory = '';
        var perPage = 15;
        var sentinel = document.getElementById('load-sentinel');
        var loadMoreWrap = document.getElementById('load-more-wrap');
        var loadMoreBtn = document.getElementById('load-more-btn');

        var pathMatch = window.location.pathname.match(/^\/category\/(.+)$/);
        if (pathMatch) activeCategory = pathMatch[1];

        var catEmoji = { 'quick-peek': '⚡', 'bathroom-break': '☕', 'lunch-break': '🍱', 'daily-highlight': '🔥' };
        var readTime = { 'quick-peek': '30秒', 'bathroom-break': '3-5分钟', 'lunch-break': '10-15分钟', 'daily-highlight': '5-10分钟' };

        function renderCard(post) {
            var emoji = catEmoji[post.CategorySlug] || '';
            var time = readTime[post.CategorySlug] || '';
            var dateStr = new Date(post.CreatedAt).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' });
            var tagsHTML = post.Tags ? '<div class="feed-card__tags">' + post.Tags.map(function (t) {
                return '<a href="/tag/' + t.Slug + '" class="tag">' + t.Name + '</a>';
            }).join('') + '</div>' : '';
            var mediaHTML = '';
            if (post.ThumbnailURL) {
                var isV = /\.(mp4|webm|ogg|mov)$/i.test(post.ThumbnailURL) || post.ThumbnailURL.indexOf('/video/') > -1;
                mediaHTML = isV
                    ? '<div class="feed-card__media"><video src="' + post.ThumbnailURL + '" preload="none" controls playsinline class="feed-card__video"></video></div>'
                    : '<div class="feed-card__media"><img src="' + post.ThumbnailURL + '" alt="' + post.Title + '" loading="lazy"></div>';
            }
            return '<article class="feed-card neo-box" data-category="' + (post.CategorySlug || '') + '">' +
                mediaHTML + '<div class="feed-card__body">' +
                '<header class="feed-card__header">' +
                (post.CategoryName ? '<a href="/category/' + post.CategorySlug + '" class="feed-card__category">' + emoji + ' ' + post.CategoryName + (time ? ' · ' + time : '') + '</a>' : '') +
                '<time datetime="' + post.CreatedAt + '">' + dateStr + '</time>' +
                '</header>' +
                '<h2 class="feed-card__title"><a href="/post/' + post.Slug + '">' + post.Title + '</a></h2>' +
                '<div class="feed-card__content">' + (post.ContentHTML || post.Excerpt) + '</div>' +
                tagsHTML + '</div></article>';
        }

        function loadMore() {
            if (loading || !hasMore) return;
            loading = true;
            currentPage++;
            var url = '/api/posts?page=' + currentPage + '&per_page=' + perPage;
            if (activeCategory) url += '&category=' + encodeURIComponent(activeCategory);
            fetch(url).then(function (r) { return r.json(); }).then(function (data) {
                (data.posts || []).forEach(function (p) { feedEl.insertAdjacentHTML('beforeend', renderCard(p)); });
                hasMore = data.page < data.total_pages;
                if (!hasMore && loadMoreWrap) loadMoreWrap.style.display = 'none';
                loading = false;
            }).catch(function () { loading = false; });
        }

        if (sentinel) {
            new IntersectionObserver(function (entries) {
                if (entries[0].isIntersecting && hasMore) loadMore();
            }, { rootMargin: '400px' }).observe(sentinel);
        }
        if (loadMoreBtn) loadMoreBtn.addEventListener('click', loadMore);
    })();

    // ==========================================
    // 全屏刷屏页 (/fullscreen) — 进度计数器 + 导航栏自动隐藏
    // ==========================================
    (function () {
        var feed = document.querySelector('.swipe-feed');
        if (!feed) return;
        var current = document.getElementById('swipe-current');
        var total = document.getElementById('swipe-total');
        var navbar = document.querySelector('.navbar');
        var lastScrollY = 0;

        // 进度计数器
        if (current && total) {
            var observer = new IntersectionObserver(function (entries) {
                entries.forEach(function (entry) {
                    if (entry.isIntersecting && entry.intersectionRatio > 0.5) {
                        var cards = feed.querySelectorAll('.feed-card');
                        var idx = Array.prototype.indexOf.call(cards, entry.target) + 1;
                        if (idx > 0) current.textContent = idx;
                    }
                });
            }, { threshold: 0.5 });
            feed.querySelectorAll('.feed-card').forEach(function (c) { observer.observe(c); });
        }

        // 导航栏改为固定定位 + 下拉消失/上拉出现
        if (navbar) {
            navbar.style.position = 'fixed';
            navbar.style.top = '0';
            navbar.style.left = '0';
            navbar.style.right = '0';
            navbar.style.zIndex = '100';
            navbar.style.transition = 'transform 0.3s ease';

            feed.addEventListener('scroll', function () {
                var sy = feed.scrollTop;
                if (sy > lastScrollY && sy > 60) {
                    navbar.style.transform = 'translateY(-100%)';
                } else if (sy < lastScrollY) {
                    navbar.style.transform = 'translateY(0)';
                }
                lastScrollY = sy;
            }, { passive: true });
        }
    })();

    // ==========================================
    // 后台编辑器 — 预览 + 上传 + 标签
    // ==========================================
    var previewTabs = document.querySelectorAll('.preview-tab');
    if (previewTabs.length) {
        previewTabs.forEach(function (tab) {
            tab.addEventListener('click', function () {
                previewTabs.forEach(function (t) { t.classList.remove('active'); });
                tab.classList.add('active');
                updatePreview();
            });
        });
    }

    var fileInput = document.getElementById('file-upload');
    var dropZone = document.getElementById('editor-drop-zone');
    var uploadQueue = [];

    function addToQueue(file) { uploadQueue.push({ file: file, uploaded: false }); renderQueue(); }
    function removeFromQueue(idx) { uploadQueue.splice(idx, 1); renderQueue(); }
    function renderQueue() {
        var list = document.getElementById('editor-queue-list');
        if (!list) return;
        list.innerHTML = '';
        uploadQueue.forEach(function (item, i) {
            var d = document.createElement('div');
            d.className = 'upload-item';
            d.innerHTML = '<span>' + item.file.name + '</span>' + (item.uploaded
                ? '<span style="color:green;">✓</span>'
                : '<button class="btn btn--small btn--danger queue-remove-btn" data-idx="' + i + '">×</button>');
            list.appendChild(d);
        });
        list.querySelectorAll('.queue-remove-btn').forEach(function (b) {
            b.addEventListener('click', function () { removeFromQueue(parseInt(this.dataset.idx)); });
        });
    }

    if (fileInput) {
        fileInput.addEventListener('change', function () {
            var files = this.files; if (files && files.length) { for (var i = 0; i < files.length; i++) addToQueue(files[i]); }
            this.value = '';
        });
    }
    if (dropZone) {
        dropZone.addEventListener('click', function () { if (fileInput) fileInput.click(); });
        dropZone.addEventListener('dragover', function (e) { e.preventDefault(); dropZone.classList.add('editor-drop-zone--drag-over'); });
        dropZone.addEventListener('dragleave', function () { dropZone.classList.remove('editor-drop-zone--drag-over'); });
        dropZone.addEventListener('drop', function (e) {
            e.preventDefault(); dropZone.classList.remove('editor-drop-zone--drag-over');
            var files = e.dataTransfer.files;
            if (files && files.length) { for (var i = 0; i < files.length; i++) addToQueue(files[i]); }
        });
    }

    window.uploadEditorBatch = function () {
        var pending = uploadQueue.filter(function (item) { return !item.uploaded; });
        if (!pending.length) return;
        (function next(idx) {
            if (idx >= pending.length) return;
            var item = pending[idx];
            var fd = new FormData(); fd.append('file', item.file);
            var xhr = new XMLHttpRequest(); xhr.open('POST', '/admin/upload');
            xhr.onload = function () {
                if (xhr.status === 200) {
                    item.uploaded = true;
                    var resp = JSON.parse(xhr.responseText);
                    var ta = document.getElementById('content');
                    var imgUrl = document.getElementById('image_url');
                    if (imgUrl) { imgUrl.value = resp.url; }
                    else if (ta) {
                        var ext = item.file.name.split('.').pop().toLowerCase();
                        var isV = ['mp4', 'webm', 'ogg', 'mov'].indexOf(ext) >= 0;
                        var tag = isV ? '<video src="' + resp.url + '" controls></video>' : '![](' + resp.url + ')';
                        ta.value = ta.value.substring(0, ta.selectionStart) + tag + '\n' + ta.value.substring(ta.selectionEnd);
                    }
                    renderQueue();
                }
                next(idx + 1);
            };
            xhr.onerror = function () { next(idx + 1); };
            xhr.send(fd);
        })(0);
    };

    window.cancelEditor = function () {
        var urls = uploadQueue.filter(function (i) { return i.uploaded; }).map(function (i) { return i.file.name; });
        if (urls.length) navigator.sendBeacon('/admin/upload/cleanup', JSON.stringify({ urls: urls }));
        window.location.href = '/admin';
    };

    function looksLikePureHTML(s) {
        return /^\s*<[a-zA-Z]/.test(s) && !/[#>\-\*\d]/.test(s.replace(/```[\s\S]*?```/g, ''));
    }
    window.updatePreview = function () {
        var ta = document.getElementById('content'), pre = document.getElementById('preview-content');
        if (!ta || !pre) return;
        var md = ta.value;
        var tab = document.querySelector('.preview-tab.active');
        var mode = tab ? tab.dataset.preview : 'detail';
        if (mode === 'card') {
            var img = (md.match(/!\[.*?\]\((.*?)\)/) || [])[1] || '';
            var title = (document.getElementById('title') || {}).value || '标题预览';
            var excerpt = md.replace(/[#*`\[\]()!_]/g, '').replace(/\s+/g, ' ').substring(0, 150);
            pre.innerHTML = '<article class="post-card neo-box post-card--has-thumb">' +
                (img ? '<div class="post-card__thumb"><img src="' + img + '" alt=""></div>' : '') +
                '<div class="post-card__body"><h2 class="post-card__title">' + title + '</h2>' +
                '<p class="post-card__excerpt">' + excerpt + '</p></div></article>';
        } else {
            var html = looksLikePureHTML(md) ? md : marked.parse(md);
            pre.innerHTML = '<div class="neo-box post-full__body">' + html + '</div>';
        }
    };

    if (document.getElementById('content')) {
        document.getElementById('content').addEventListener('input', updatePreview);
        updatePreview();
    }

    window.addTag = function (btn) {
        var ta = document.getElementById('tags'); if (!ta) return;
        var tag = btn.dataset.tag;
        var tags = ta.value.split('\n').map(function (s) { return s.trim(); }).filter(Boolean);
        if (tags.indexOf(tag) < 0) tags.push(tag);
        ta.value = tags.join('\n');
    };

    // ==========================================
    // 表情包管理
    // ==========================================
    document.addEventListener('click', function (e) {
        var btn = e.target.closest('.sticker-copy-btn');
        if (!btn) return;
        var url = btn.dataset.url;
        if (navigator.clipboard) {
            navigator.clipboard.writeText(url).then(function () {
                btn.textContent = '已复制'; setTimeout(function () { btn.textContent = '复制链接'; }, 1500);
            });
        }
    });

    var stickerDrop = document.getElementById('sticker-drop-zone');
    if (stickerDrop) {
        stickerDrop.addEventListener('dragover', function (e) { e.preventDefault(); stickerDrop.classList.add('sticker-drop-zone--drag-over'); });
        stickerDrop.addEventListener('dragleave', function () { stickerDrop.classList.remove('sticker-drop-zone--drag-over'); });
        stickerDrop.addEventListener('drop', function (e) {
            e.preventDefault(); stickerDrop.classList.remove('sticker-drop-zone--drag-over');
            var files = e.dataTransfer.files;
            if (files && files.length) {
                for (var i = 0; i < files.length; i++) {
                    var fd = new FormData(); fd.append('file', files[i]);
                    fetch('/admin/stickers', { method: 'POST', body: fd, headers: { 'X-Requested-With': 'XMLHttpRequest' } })
                        .then(function (r) { return r.json(); }).then(function () { window.location.reload(); });
                }
            }
        });
    }

    window.previewSticker = function (url, isVideo) {
        var m = document.getElementById('sticker-modal'), c = document.getElementById('sticker-modal-content');
        if (!m || !c) return;
        c.innerHTML = isVideo ? '<video src="' + url + '" controls autoplay style="max-width:90vw;max-height:80vh;"></video>'
            : '<img src="' + url + '" style="max-width:90vw;max-height:80vh;">';
        m.style.display = 'flex';
    };
    window.closeStickerPreview = function () {
        var m = document.getElementById('sticker-modal'); if (m) m.style.display = 'none';
    };

    // ==========================================
    // 图片懒加载 + Lightbox
    // ==========================================
    (function () {
        if ('IntersectionObserver' in window) {
            var obs = new IntersectionObserver(function (entries) {
                entries.forEach(function (entry) {
                    if (!entry.isIntersecting) return;
                    var src = entry.target.getAttribute('data-src');
                    if (src) { entry.target.src = src; entry.target.removeAttribute('data-src'); }
                    entry.target.classList.add('img-loaded');
                    obs.unobserve(entry.target);
                });
            }, { rootMargin: '300px' });
            document.querySelectorAll('img[data-src]').forEach(function (img) { obs.observe(img); });
        }
    })();

    (function () {
        var imgs = document.querySelectorAll('.post-full__body img, .feed-card__content img');
        imgs = Array.prototype.filter.call(imgs, function (img) { return img.src && !img.src.startsWith('data:'); });
        if (!imgs.length) return;
        var dialog = document.createElement('dialog');
        dialog.className = 'img-lightbox';
        dialog.innerHTML = '<div class="img-lightbox__inner">' +
            '<button class="img-lightbox__close">&times;</button>' +
            '<button class="img-lightbox__prev">&lsaquo;</button>' +
            '<img class="img-lightbox__img" src="" alt="">' +
            '<button class="img-lightbox__next">&rsaquo;</button>' +
            '<div class="img-lightbox__counter"></div></div>';
        document.body.appendChild(dialog);
        var idx = 0, imgEl = dialog.querySelector('.img-lightbox__img'), counter = dialog.querySelector('.img-lightbox__counter');
        function show(i) {
            idx = (i + imgs.length) % imgs.length;
            imgEl.src = imgs[idx].src; counter.textContent = (idx + 1) + ' / ' + imgs.length;
        }
        imgs.forEach(function (img, i) { img.style.cursor = 'zoom-in'; img.addEventListener('click', function (e) { e.preventDefault(); show(i); dialog.showModal(); }); });
        dialog.querySelector('.img-lightbox__close').addEventListener('click', function () { dialog.close(); });
        dialog.querySelector('.img-lightbox__next').addEventListener('click', function () { show(idx + 1); });
        dialog.querySelector('.img-lightbox__prev').addEventListener('click', function () { show(idx - 1); });
        dialog.addEventListener('click', function (e) { if (e.target === dialog) dialog.close(); });
        dialog.addEventListener('keydown', function (e) {
            if (e.key === 'ArrowLeft') { e.preventDefault(); show(idx - 1); }
            if (e.key === 'ArrowRight') { e.preventDefault(); show(idx + 1); }
        });
    })();

    // ==========================================
    // 视频播放
    // ==========================================
    document.addEventListener('click', function (e) {
        var player = e.target.closest('.video-player');
        if (!player) return;
        var video = player.querySelector('video'); if (!video) return;
        if (video.paused) { video.play(); player.classList.add('playing'); }
        else { video.pause(); player.classList.remove('playing'); }
    });

    // ==========================================
    // 刷屏模式快捷键 — Ctrl+Z 切换
    // ==========================================
    document.addEventListener('keydown', function (e) {
        if (e.ctrlKey && e.key === 'z' && !e.target.closest('input,textarea,[contenteditable]')) {
            e.preventDefault();
            if (window.location.pathname === '/fullscreen') {
                window.history.back();
            } else {
                window.location.href = '/fullscreen';
            }
        }
    });

    // 切换刷屏的通用函数（按钮也用）
    window.toggleFullscreen = function () {
        if (window.location.pathname === '/fullscreen') {
            window.history.back();
        } else {
            window.location.href = '/fullscreen';
        }
    };

    // ==========================================
    // 摸鱼模式 (Fish Mode) — Ctrl+B
    // ==========================================
    (function () {
        var active = false, origTitle = document.title;
        var fishTitle = window._ofoFishModeTitle || '工作周报 - 2024';
        var toggle = document.getElementById('fish-mode-toggle');

        function toast(msg) {
            var t = document.createElement('div');
            t.textContent = msg;
            t.style.cssText = 'position:fixed;top:80px;left:50%;transform:translateX(-50%);z-index:9999;' +
                'background:#000;color:#FFD740;padding:0.5rem 1.5rem;border-radius:5px;font-weight:700;font-size:0.95rem;pointer-events:none;';
            document.body.appendChild(t);
            setTimeout(function () { t.style.opacity = '0'; }, 1500);
            setTimeout(function () { t.remove(); }, 2000);
        }

        function on() {
            active = true; document.title = fishTitle;
            document.body.classList.add('fish-mode');
            document.documentElement.setAttribute('data-fish-mode', 'true');
            localStorage.setItem('ofo-fish-mode', 'true');
            if (toggle) toggle.querySelector('.fish-mode-toggle__icon').textContent = '✓';
            toast('摸鱼模式已开启');
        }
        function off() {
            active = false; document.title = origTitle;
            document.body.classList.remove('fish-mode');
            document.documentElement.setAttribute('data-fish-mode', 'false');
            localStorage.setItem('ofo-fish-mode', 'false');
            if (toggle) toggle.querySelector('.fish-mode-toggle__icon').textContent = '🐟';
            toast('摸鱼模式已关闭');
        }

        if (toggle) toggle.addEventListener('click', function () { active ? off() : on(); });
        document.addEventListener('keydown', function (e) {
            if (e.ctrlKey && e.key === 'x') { e.preventDefault(); active ? off() : on(); }
        });
        if (localStorage.getItem('ofo-fish-mode') === 'true') on();
    })();

    // ==========================================
    // 老板键 (Boss Key) — Ctrl+Shift+H
    // ==========================================
    (function () {
        var active = false, overlay = null;
        function create() {
            overlay = document.createElement('div');
            overlay.id = 'boss-overlay';
            overlay.style.cssText = 'position:fixed;inset:0;z-index:999999;background:var(--bg-secondary,#fff);' +
                'display:flex;align-items:center;justify-content:center;font-family:var(--font-body);';
            overlay.innerHTML = '<div style="max-width:600px;padding:2rem;text-align:center;">' +
                '<h1 style="font-size:1.5rem;margin-bottom:1rem;">工作周报 - 2024年第30周</h1>' +
                '<table style="width:100%;border-collapse:collapse;margin:1rem 0;font-size:0.9rem;">' +
                '<tr style="border-bottom:1px solid #ddd;"><th style="text-align:left;padding:0.5rem;">项目</th><th>进度</th></tr>' +
                '<tr><td style="padding:0.5rem;">系统优化</td><td style="text-align:center;color:green;">85%</td></tr>' +
                '<tr><td style="padding:0.5rem;">接口开发</td><td style="text-align:center;color:green;">90%</td></tr>' +
                '<tr><td style="padding:0.5rem;">文档更新</td><td style="text-align:center;color:orange;">60%</td></tr>' +
                '</table><p style="margin-top:2rem;font-size:0.85rem;">按 <kbd>Esc</kbd> 返回</p></div>';
            document.body.appendChild(overlay);
        }
        function on() { if (!overlay) create(); active = true; overlay.style.display = 'flex'; document.title = '工作周报 - 2024'; }
        function off() { active = false; overlay.style.display = 'none'; document.title = document.body.classList.contains('fish-mode') ? '工作周报 - 2024' : '蹬车摸鱼'; }
        document.addEventListener('keydown', function (e) {
            if (e.ctrlKey && e.key === 'q') { e.preventDefault(); active ? off() : on(); }
            if (active && e.key === 'Escape') { e.preventDefault(); off(); }
        });
        var btn = document.getElementById('boss-key-btn');
        if (btn) btn.addEventListener('click', function () { active ? off() : on(); });
    })();
})();
