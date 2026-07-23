/**
 * RetroUI Blog — Client-side JavaScript
 * Theme toggle + mobile nav + code copy + editor preview + file upload
 */
(function () {
    'use strict';

    // --- Theme Toggle ---
    function getTheme() {
        return localStorage.getItem('ofo-theme') || 'light';
    }

    function setTheme(theme) {
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem('ofo-theme', theme);
        updateToggleIcon(theme);
    }

    function updateToggleIcon(theme) {
        var icon = document.querySelector('.theme-toggle__icon');
        if (icon) {
            icon.innerHTML = theme === 'dark' ? '&#9789;' : '&#9788;';
        }
    }

    setTheme(getTheme());

    document.addEventListener('DOMContentLoaded', function () {
        // Theme toggle button
        var btn = document.getElementById('theme-toggle');
        if (btn) {
            btn.addEventListener('click', function () {
                var current = document.documentElement.getAttribute('data-theme');
                var next = current === 'light' ? 'dark' : 'light';
                setTheme(next);
            });
        }

        // Mobile nav toggle
        var navToggle = document.getElementById('nav-toggle');
        var navLinks = document.getElementById('nav-links');
        if (navToggle && navLinks) {
            navToggle.addEventListener('click', function () {
                navLinks.classList.toggle('open');
            });
            navLinks.querySelectorAll('a').forEach(function (link) {
                link.addEventListener('click', function () {
                    navLinks.classList.remove('open');
                });
            });
        }

        // Code copy buttons
        document.querySelectorAll('pre').forEach(function (block) {
            var btn = document.createElement('button');
            btn.className = 'code-copy-btn';
            btn.textContent = '复制';
            btn.setAttribute('aria-label', '复制代码到剪贴板');
            btn.addEventListener('click', function () {
                var code = block.querySelector('code');
                var text = code ? code.textContent : block.textContent;
                navigator.clipboard.writeText(text).then(function () {
                    btn.textContent = '已复制!';
                    setTimeout(function () { btn.textContent = '复制'; }, 2000);
                }).catch(function () {
                    btn.textContent = '失败';
                    setTimeout(function () { btn.textContent = '复制'; }, 2000);
                });
            });
            block.style.position = 'relative';
            block.appendChild(btn);
        });

        // --- Preview Panel Tabs ---
        var previewTabs = document.querySelectorAll('.preview-tab');
        previewTabs.forEach(function (tab) {
            tab.addEventListener('click', function () {
                previewTabs.forEach(function (t) { t.classList.remove('active'); });
                tab.classList.add('active');
                updatePreview();
            });
        });

        // Initial preview
        updatePreview();

        // ==========================================
        // 信息流：AJAX 无限滚动 + 分类筛选 + 刷屏模式
        // ==========================================
        (function () {
            var currentPage = 1;
            var loading = false;
            var hasMore = true;
            var activeCategory = '';
            var perPage = 15;
            var feedEl = document.getElementById('post-feed');
            var sentinel = document.getElementById('load-sentinel');
            var loadMoreWrap = document.getElementById('load-more-wrap');
            var loadMoreBtn = document.getElementById('load-more-btn');
            var swipeProgress = document.getElementById('swipe-progress');
            var swipeCurrent = document.getElementById('swipe-current');
            var swipeTotal = document.getElementById('swipe-total');
            var isMobile = window.matchMedia('(max-width: 640px)').matches;

            // ---- 卡片渲染 ----
            function renderCard(post) {
                var catEmoji = { 'quick-peek': '⚡', 'bathroom-break': '☕', 'lunch-break': '🍱', 'daily-highlight': '🔥' };
                var readTime = { 'quick-peek': '30秒', 'bathroom-break': '3-5分钟', 'lunch-break': '10-15分钟', 'daily-highlight': '5-10分钟' };
                var emoji = catEmoji[post.CategorySlug] || '';
                var time = readTime[post.CategorySlug] || '';

                var tagsHTML = '';
                if (post.Tags) {
                    tagsHTML = '<div class="feed-card__tags">' + post.Tags.map(function (t) {
                        return '<a href="/tag/' + t.Slug + '" class="tag">' + t.Name + '</a>';
                    }).join('') + '</div>';
                }

                var mediaHTML = '';
                if (post.ThumbnailURL) {
                    var isVideo = post.ThumbnailURL.match(/\.(mp4|webm|ogg|mov)$/i) || post.ThumbnailURL.indexOf('/video/') > -1;
                    if (isVideo) {
                        mediaHTML = '<div class="feed-card__media"><video src="' + post.ThumbnailURL + '" preload="none" controls playsinline class="feed-card__video"></video></div>';
                    } else {
                        mediaHTML = '<div class="feed-card__media"><img src="' + post.ThumbnailURL + '" alt="' + post.Title + '" loading="lazy"></div>';
                    }
                }

                var dateStr = new Date(post.CreatedAt).toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' });

                return '<article class="feed-card neo-box" data-category="' + (post.CategorySlug || '') + '">' +
                    mediaHTML +
                    '<div class="feed-card__body">' +
                    '<header class="feed-card__header">' +
                    (post.CategoryName ? '<a href="/category/' + post.CategorySlug + '" class="feed-card__category">' + emoji + ' ' + post.CategoryName + (time ? ' · ' + time : '') + '</a>' : '') +
                    '<time datetime="' + post.CreatedAt + '">' + dateStr + '</time>' +
                    '</header>' +
                    '<h2 class="feed-card__title"><a href="/post/' + post.Slug + '">' + post.Title + '</a></h2>' +
                    '<div class="feed-card__content">' + (post.ContentHTML || post.Excerpt) + '</div>' +
                    tagsHTML +
                    '</div></article>';
            }

            // ---- AJAX 加载 ----
            function loadMore() {
                if (loading || !hasMore) return;
                loading = true;
                currentPage++;
                var url = '/api/posts?page=' + currentPage + '&per_page=' + perPage;
                if (activeCategory) url += '&category=' + encodeURIComponent(activeCategory);

                fetch(url).then(function (r) { return r.json(); }).then(function (data) {
                    var posts = data.posts || [];
                    posts.forEach(function (p) {
                        feedEl.insertAdjacentHTML('beforeend', renderCard(p));
                    });
                    hasMore = data.page < data.total_pages;
                    if (!hasMore && loadMoreWrap) loadMoreWrap.style.display = 'none';
                    updateSwipeTotal();
                    observeNewCards(feedEl);
                    loading = false;
                }).catch(function () {
                    loading = false;
                });
            }

            // ---- 哨兵观察（桌面端自动加载） ----
            if (sentinel) {
                var sentinelObserver = new IntersectionObserver(function (entries) {
                    if (entries[0].isIntersecting && hasMore && !isMobile) {
                        loadMore();
                    }
                }, { rootMargin: '400px' });
                sentinelObserver.observe(sentinel);
            }

            // ---- 加载更多按钮（桌面端手动） ----
            if (loadMoreBtn) {
                loadMoreBtn.addEventListener('click', loadMore);
            }

            // 从 URL 检测当前分类（如 /category/quick-peek）
            var pathMatch = window.location.pathname.match(/^\/category\/(.+)$/);
            if (pathMatch) activeCategory = pathMatch[1];

            // ---- 移动端 snap 进度 ----
            function updateSwipeTotal() {
                var total = document.querySelectorAll('.feed-card' + (activeCategory ? '[data-category="' + activeCategory + '"]' : '')).length;
                if (swipeTotal) swipeTotal.textContent = total;
            }

            var snapObserver = null;
            if (isMobile && swipeProgress) {
                swipeProgress.style.display = 'block';
                snapObserver = new IntersectionObserver(function (entries) {
                    entries.forEach(function (entry) {
                        if (entry.isIntersecting && entry.intersectionRatio > 0.5) {
                            var idx = parseInt(entry.target.getAttribute('data-post-index') || '0', 10) + 1;
                            if (swipeCurrent && !isNaN(idx)) swipeCurrent.textContent = idx;
                        }
                    });
                }, { threshold: 0.5 });

                document.querySelectorAll('.feed-card').forEach(function (card) {
                    snapObserver.observe(card);
                });
            }

            // 将新卡片加入 snap 观察器
            function observeNewCards(container) {
                if (!snapObserver) return;
                container.querySelectorAll('.feed-card').forEach(function (card) {
                    snapObserver.observe(card);
                });
            }

            // ---- 刷屏模式切换 ----
            window.toggleSwipeMode = function () {
                document.body.classList.toggle('swipe-mode');
                var isSwipe = document.body.classList.contains('swipe-mode');
                if (isSwipe && swipeProgress) swipeProgress.style.display = 'block';
                if (!isSwipe && !isMobile && swipeProgress) swipeProgress.style.display = 'none';
                if (isSwipe && feedEl) feedEl.scrollTop = 0;
            };
        })();

        // --- File Upload ---
        var fileInput = document.getElementById('file-upload');
        if (fileInput) {
            fileInput.addEventListener('change', function () {
                handleUpload(this);
            });
        }
    });

    // --- Markdown Preview ---

    // looksLikePureHTML: 判断内容是否纯 HTML（无 Markdown 语法），避免 <img> 被套 <p>
    function looksLikePureHTML(s) {
        // 必须以 HTML 标签开头
        if (!/^\s*<[a-zA-Z]/.test(s)) return false;
        // 不含 Markdown 块级语法（标题、引用、列表、代码块）
        if (/^#{1,6}\s|^>\s|^[\-\*\+]\s|^\d+\.\s|^```/m.test(s)) return false;
        // 不含常见内联 Markdown 语法
        if (/\*\*|__|!\[/.test(s)) return false;
        // 不含 [text](url) 链接语法
        if (/\[[^\]]+\]\([^)]+\)/.test(s)) return false;
        return true;
    }

    // preprocessMarkdown: 递归渲染 HTML 容器标签内的 Markdown（前端预览用）
    function preprocessMarkdown(md) {
        // 先把 width="100px" 等属性转为 style
        md = md.replace(/\b(width|height)\s*=\s*"(\d+%?)"/gi, function(m, prop, val) {
            return 'style="' + prop.toLowerCase() + ':' + val + '"';
        });
        md = md.replace(/\b(align)\s*=\s*"(left|center|right)"/gi, function(m, prop, val) {
            return 'style="text-align:' + val.toLowerCase() + '"';
        });

        var tagList = 'div|section|article|figure|figcaption|details|summary|header|footer|nav|aside|main';
        var re = new RegExp('<(' + tagList + ')\\b([^>]*)>(.+?)<\\/(\\w+)>', 'gs');
        for (var i = 0; i < 10; i++) {
            var before = md;
            md = md.replace(re, function(match, openTag, attrs, content, closeTag) {
                if (openTag !== closeTag) return match;
                // 纯 HTML 内容跳过 Markdown 渲染，避免 <img> 等被套上 <p>
                if (looksLikePureHTML(content)) {
                    return '<' + openTag + attrs + '>\n' + content + '\n</' + openTag + '>';
                }
                var rendered;
                try {
                    if (typeof marked !== 'undefined' && typeof marked.parse === 'function') {
                        rendered = marked.parse(content);
                    } else if (typeof marked !== 'undefined' && typeof marked === 'function') {
                        rendered = marked(content);
                    } else {
                        rendered = escapeHtmlForPreview(content);
                    }
                } catch (e) {
                    rendered = escapeHtmlForPreview(content);
                }
                return '<' + openTag + attrs + '>\n' + rendered + '\n</' + openTag + '>';
            });
            if (md === before) break;
        }
        return md;
    }

    window.updatePreview = function () {
        var contentEl = document.getElementById('content');
        var titleEl = document.getElementById('title');
        var thumbEl = document.getElementById('thumbnail_url');
        var previewContent = document.getElementById('preview-content');
        if (!previewContent) return;

        var md = contentEl ? contentEl.value : '';
        var title = titleEl ? (titleEl.value || '文章标题') : '文章标题';
        var thumbURL = thumbEl ? thumbEl.value : '';

        if (!md.trim()) {
            previewContent.innerHTML = '<div class="preview-placeholder">在左侧输入内容即可实时预览…</div>';
            return;
        }

        var activeTab = document.querySelector('.preview-tab.active');
        var mode = activeTab ? activeTab.getAttribute('data-preview') : 'detail';

        var html = '';
        try {
            // 预处理：渲染 HTML 容器内的 Markdown
            var processed = preprocessMarkdown(md);
            if (typeof marked !== 'undefined' && typeof marked.parse === 'function') {
                html = marked.parse(processed);
            } else if (typeof marked !== 'undefined' && typeof marked === 'function') {
                html = marked(processed);
            } else {
                html = escapeHtmlForPreview(processed);
            }
        } catch (e) {
            html = escapeHtmlForPreview(md);
        }

        // Shared elements
        var catEl = document.getElementById('category_id');
        var catName = catEl && catEl.selectedIndex >= 0 ? catEl.options[catEl.selectedIndex].text : '';
        if (catName === '— 无 —') catName = '';

        var tagsEl = document.getElementById('tags');
        var tagNames = [];
        if (tagsEl && tagsEl.value.trim()) {
            tagNames = tagsEl.value.split(/[\r\n]+/).map(function(t) { return t.trim(); }).filter(Boolean);
        }

        // Extract first image for card preview
        var firstImg = extractFirstImage(html);
        var displayThumb = thumbURL || firstImg || '';
        var isVideo = displayThumb && (displayThumb.indexOf('.mp4') > -1 || displayThumb.indexOf('.webm') > -1 ||
            displayThumb.indexOf('.ogg') > -1 || displayThumb.indexOf('.mov') > -1);

        if (mode === 'card') {
            // 主页卡片预览 — 与 layout.html 结构一致
            var excerptEl = document.getElementById('excerpt');
            var excerpt = '';
            if (excerptEl && excerptEl.value.trim()) {
                excerpt = excerptEl.value.trim();
            } else {
                excerpt = stripHtml(html).substring(0, 150) + '...';
            }

            var thumbHTML = '';
            if (displayThumb) {
                if (isVideo) {
                    thumbHTML = '<div class="video-player" onclick="toggleVideoPlayer(this)"><video src="' + displayThumb + '" preload="metadata"></video><div class="video-player__play"></div></div>';
                } else {
                    thumbHTML = '<img src="' + displayThumb + '" alt="' + title + '" loading="lazy">';
                }
            }

            var tagsHTML = '';
            if (tagNames.length > 0) {
                tagsHTML = tagNames.map(function(t) {
                    return '<a href="/tag/' + encodeURIComponent(t) + '" class="tag">' + t + '</a>';
                }).join('\n');
            }

            var html = '';
            html += '<article class="post-card neo-box' + (displayThumb ? ' post-card--has-thumb' : '') + '">\n';
            if (displayThumb) {
                html += '  <div class="post-card__thumb">\n';
                html += '    ' + thumbHTML + '\n';
                html += '  </div>\n';
            }
            html += '  <div class="post-card__body">\n';
            html += '    <h2 class="post-card__title">\n';
            html += '      <a href="#">' + title + '</a>\n';
            html += '    </h2>\n';
            html += '    <div class="post-card__meta">\n';
            html += '      <time datetime="' + new Date().toISOString().slice(0, 10) + '">预览日期</time>\n';
            if (catName) {
                html += '      <span>·</span>\n';
                html += '      <a href="#" class="post-card__category">' + catName + '</a>\n';
            }
            html += '    </div>\n';
            html += '    <p class="post-card__excerpt">' + excerpt + '</p>\n';
            if (tagsHTML) {
                html += '    <div class="post-card__tags">\n';
                html += '      ' + tagsHTML + '\n';
                html += '    </div>\n';
            }
            html += '  </div>\n';
            html += '</article>';
            previewContent.innerHTML = html;
        } else {
            // Detail preview
            var metaHtml = '<div class="post-full__meta">' +
                '<span class="post-full__date">预览日期</span>';
            if (catName) {
                metaHtml += '<span>&middot;</span><span class="post-full__category">' + catName + '</span>';
            }
            metaHtml += '</div>';

            previewContent.innerHTML =
                '<div class="preview-detail">' +
                '<header class="post-full__header">' +
                '<h1 class="post-full__title">' + title + '</h1>' +
                metaHtml +
                '</header>' +
                '<div class="post-full__body">' + html + '</div>' +
                '</div>';
        }
    };

    // --- Editor File Upload (drag-drop + multi-file) ---
    var editorDropZone = document.getElementById('editor-drop-zone');
    var editorFileInput = document.getElementById('editor-file-input');
    var editorQueueDiv = document.getElementById('editor-upload-queue');
    var editorQueueList = document.getElementById('editor-queue-list');
    var editorQueueStatus = document.getElementById('editor-queue-status');
    var editorPendingFiles = [];
    var editorUploadedURLs = [];   // 本次编辑已上传的 URL，取消时用于清理

    function addEditorFiles(files) {
        for (var i = 0; i < files.length; i++) {
            var f = files[i];
            var ext = f.name.split('.').pop().toLowerCase();
            var allowed = ['jpg','jpeg','png','gif','webp','mp4','webm','ogg','mov'];
            if (allowed.indexOf(ext) < 0) continue;
            var idx = editorPendingFiles.length;
            editorPendingFiles.push(f);
            var li = document.createElement('li');
            li.style.display = 'flex';
            li.style.justifyContent = 'space-between';
            li.style.alignItems = 'center';
            li.innerHTML = '<span>' + f.name + ' (' + formatSize(f.size) + ')</span>' +
                '<button type="button" class="queue-remove-btn" data-idx="' + idx + '" data-queue="editor" title="移除">&times;</button>';
            editorQueueList.appendChild(li);
        }
        refreshEditorQueue();
    }

    function refreshEditorQueue() {
        if (editorPendingFiles.length === 0) {
            editorQueueDiv.style.display = 'none';
        } else {
            editorQueueDiv.style.display = 'block';
            editorQueueStatus.textContent = editorPendingFiles.length + ' 个文件待上传';
        }
    }

    // 队列删除按钮（事件委托）
    if (editorQueueList) editorQueueList.addEventListener('click', function (e) {
        var btn = e.target.closest('.queue-remove-btn');
        if (!btn) return;
        var idx = parseInt(btn.getAttribute('data-idx'), 10);
        var queue = btn.getAttribute('data-queue');
        if (queue === 'editor') {
            editorPendingFiles.splice(idx, 1);
            // 重建列表
            editorQueueList.innerHTML = '';
            editorPendingFiles.forEach(function (f, i) {
                var li = document.createElement('li');
                li.style.display = 'flex';
                li.style.justifyContent = 'space-between';
                li.style.alignItems = 'center';
                li.innerHTML = '<span>' + f.name + ' (' + formatSize(f.size) + ')</span>' +
                    '<button type="button" class="queue-remove-btn" data-idx="' + i + '" data-queue="editor" title="移除">&times;</button>';
                editorQueueList.appendChild(li);
            });
            refreshEditorQueue();
        }
    });

    if (editorDropZone && editorFileInput) {
        editorDropZone.addEventListener('click', function (e) {
            if (e.target.tagName !== 'BUTTON') editorFileInput.click();
        });
        editorDropZone.addEventListener('dragover', function (e) {
            e.preventDefault();
            editorDropZone.classList.add('sticker-drop-zone--drag-over');
        });
        editorDropZone.addEventListener('dragleave', function () {
            editorDropZone.classList.remove('sticker-drop-zone--drag-over');
        });
        editorDropZone.addEventListener('drop', function (e) {
            e.preventDefault();
            editorDropZone.classList.remove('sticker-drop-zone--drag-over');
            if (e.dataTransfer.files.length > 0) addEditorFiles(e.dataTransfer.files);
        });
        editorFileInput.addEventListener('change', function () {
            if (editorFileInput.files.length > 0) {
                addEditorFiles(editorFileInput.files);
                editorFileInput.value = '';
            }
        });
    }

    window.uploadEditorBatch = function () {
        if (editorPendingFiles.length === 0) return;

        editorQueueStatus.textContent = '上传中…';
        var total = editorPendingFiles.length;
        var done = 0;
        var failed = 0;

        // 给每个队列项加上进度条骨架
        var allItems = editorQueueList.querySelectorAll('li');
        for (var i = 0; i < editorPendingFiles.length; i++) {
            var f = editorPendingFiles[i];
            var li = allItems[i];
            if (li) {
                li.innerHTML = '<div class="upload-item">' +
                    '<div class="upload-item__info">' +
                        '<span class="upload-item__name">' + escapeHtml(f.name) + '</span>' +
                        '<span class="upload-item__size">' + formatSize(f.size) + '</span>' +
                    '</div>' +
                    '<div class="upload-item__progress">' +
                        '<div class="upload-item__bar" id="editor-bar-' + i + '"></div>' +
                    '</div>' +
                    '<span class="upload-item__pct" id="editor-pct-' + i + '">0%</span>' +
                '</div>';
            }
        }

        function uploadNext(idx) {
            if (idx >= editorPendingFiles.length) {
                editorQueueStatus.textContent = '完成：' + done + ' 成功' + (failed > 0 ? '，' + failed + ' 失败' : '');
                editorPendingFiles = [];
                editorQueueList.innerHTML = '';
                setTimeout(function () {
                    editorQueueDiv.style.display = 'none';
                }, 2000);
                return;
            }

            var file = editorPendingFiles[idx];
            var formData = new FormData();
            formData.append('file', file);

            var xhr = new XMLHttpRequest();
            xhr.open('POST', '/admin/upload');

            // 上传进度
            xhr.upload.onprogress = function (e) {
                if (e.lengthComputable) {
                    var pct = Math.round((e.loaded / e.total) * 100);
                    var bar = document.getElementById('editor-bar-' + idx);
                    var pctEl = document.getElementById('editor-pct-' + idx);
                    if (bar) bar.style.width = pct + '%';
                    if (pctEl) pctEl.textContent = pct + '%';
                }
            };

            xhr.onload = function () {
                try {
                    var data = JSON.parse(xhr.responseText);
                    if (xhr.status === 200 && data.url) {
                        done++;
                        var url = data.url;
                        editorUploadedURLs.push(url);   // 记录以便取消时清理
                        var ext = url.split('.').pop().toLowerCase();
                        var isVideo = ['mp4','webm','ogg','mov'].indexOf(ext) > -1;
                        var insertText = isVideo
                            ? '<video src="' + url + '" controls></video>'
                            : '![' + file.name.replace(/\.[^.]+$/, '') + '](' + url + ')';

                        var textarea = document.getElementById('content');
                        if (textarea) {
                            var start = textarea.selectionStart;
                            var after = textarea.value.substring(textarea.selectionEnd);
                            textarea.value = textarea.value.substring(0, start) + insertText + '\n' + after;
                            textarea.focus();
                            textarea.selectionStart = textarea.selectionEnd = start + insertText.length + 1;
                            updatePreview();
                        }

                        var items = editorQueueList.querySelectorAll('li');
                        if (items[idx]) {
                            items[idx].innerHTML = '<span style="color:var(--success,#22c55e)">✓ ' + escapeHtml(file.name) + ' — 上传成功</span>';
                        }
                    } else {
                        failed++;
                        var items2 = editorQueueList.querySelectorAll('li');
                        if (items2[idx]) {
                            items2[idx].innerHTML = '<span style="color:var(--danger,#ef4444)">✗ ' + escapeHtml(file.name) + ' — ' + escapeHtml(data.error || '上传失败') + '</span>';
                        }
                    }
                } catch (e) {
                    failed++;
                    var items3 = editorQueueList.querySelectorAll('li');
                    if (items3[idx]) {
                        items3[idx].innerHTML = '<span style="color:var(--danger,#ef4444)">✗ ' + escapeHtml(file.name) + ' — 响应异常</span>';
                    }
                }
                editorQueueStatus.textContent = '上传中… ' + (done + failed) + '/' + total;
                uploadNext(idx + 1);
            };

            xhr.onerror = function () {
                failed++;
                var items4 = editorQueueList.querySelectorAll('li');
                if (items4[idx]) {
                    items4[idx].innerHTML = '<span style="color:var(--danger,#ef4444)">✗ ' + escapeHtml(file.name) + ' — 网络错误</span>';
                }
                editorQueueStatus.textContent = '上传中… ' + (done + failed) + '/' + total;
                uploadNext(idx + 1);
            };

            xhr.send(formData);
        }

        uploadNext(0);
    };

    // 取消按钮：使用 sendBeacon 清理已上传但未保存的资源（不阻塞导航）
    window.cancelEditor = function () {
        if (editorUploadedURLs.length > 0) {
            var blob = new Blob(
                [JSON.stringify({ urls: editorUploadedURLs.slice() })],
                { type: 'application/json' }
            );
            navigator.sendBeacon('/admin/upload/cleanup', blob);
            editorUploadedURLs = [];
        }
        // 跳转到管理后台 — sendBeacon 在后台保证送达
        window._ofoCleanupDone = true;
        window.location.href = '/admin';
    };

    // 表单提交时清除追踪，避免 beforeunload 误拦
    var editorForm = document.getElementById('editor-form');
    if (editorForm) {
        editorForm.addEventListener('submit', function () {
            window._ofoCleanupDone = true;
            editorUploadedURLs = [];
            editorPendingFiles = [];
        });
    }

    // 离开页面前提醒（有待上传文件 或 已上传但未保存的文件）
    window.addEventListener('beforeunload', function (e) {
        if (window._ofoCleanupDone) return;
        if (editorPendingFiles.length > 0 || editorUploadedURLs.length > 0) {
            var msg = '你还有未保存的上传文件，离开后将被清理。确定离开吗？';
            e.preventDefault();
            e.returnValue = msg; // Chrome 需要
            return msg;
        }
    });

    // --- Helpers ---

    function escapeHtml(text) {
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(text));
        return div.innerHTML;
    }

    function escapeHtmlForPreview(text) {
        return text
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/\n/g, '<br>');
    }

    function stripHtml(html) {
        var div = document.createElement('div');
        div.innerHTML = html;
        return (div.textContent || div.innerText || '').trim();
    }

    function extractFirstImage(html) {
        var div = document.createElement('div');
        div.innerHTML = html;
        var img = div.querySelector('img');
        if (img && img.src) return img.src;
        var video = div.querySelector('video');
        if (video && video.src) return video.src;
        var source = div.querySelector('source');
        if (source && source.src) return source.src;
        return '';
    }
})();

// 复制表情包 URL（事件委托）
document.addEventListener('click', function (e) {
    var btn = e.target.closest('.sticker-copy-btn');
    if (!btn) return;
    e.preventDefault();
    var url = btn.getAttribute('data-url');
    if (!url) return;

    // 兼容非 HTTPS 环境：先尝试 clipboard API，失败则用 execCommand
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(url).then(function () {
            btn.textContent = '✓ 已复制';
            setTimeout(function () { btn.textContent = '📋 复制'; }, 1500);
        }).catch(function () {
            fallbackCopy(url, btn);
        });
    } else {
        fallbackCopy(url, btn);
    }
});

function fallbackCopy(url, btn) {
    var ta = document.createElement('textarea');
    ta.value = url;
    ta.style.position = 'fixed';
    ta.style.top = '0';
    ta.style.left = '0';
    ta.style.opacity = '0';
    ta.style.pointerEvents = 'none';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    try {
        document.execCommand('copy');
        btn.textContent = '✓ 已复制';
    } catch (err) {
        btn.textContent = '✗ 失败';
    }
    document.body.removeChild(ta);
    setTimeout(function () { btn.textContent = '📋 复制'; }, 1500);
}

// ---- 表情包拖拽上传 ----
(function () {
    var dropZone = document.getElementById('sticker-drop-zone');
    var fileInput = document.getElementById('sticker-file-input');
    var queueDiv = document.getElementById('sticker-upload-queue');
    var queueList = document.getElementById('sticker-queue-list');
    var queueStatus = document.getElementById('sticker-queue-status');
    var pendingFiles = [];

    function addFiles(files) {
        for (var i = 0; i < files.length; i++) {
            var f = files[i];
            var ext = f.name.split('.').pop().toLowerCase();
            var allowed = ['jpg','jpeg','png','gif','webp','mp4','webm','ogg','mov'];
            if (allowed.indexOf(ext) < 0) continue;
            var idx = pendingFiles.length;
            pendingFiles.push(f);
            var li = document.createElement('li');
            li.style.display = 'flex';
            li.style.justifyContent = 'space-between';
            li.style.alignItems = 'center';
            li.innerHTML = '<span>' + f.name + ' (' + formatSize(f.size) + ')</span>' +
                '<button type="button" class="queue-remove-btn" data-idx="' + idx + '" data-queue="sticker" title="移除">&times;</button>';
            queueList.appendChild(li);
        }
        refreshStickerQueue();
    }

    function refreshStickerQueue() {
        if (pendingFiles.length === 0) {
            queueDiv.style.display = 'none';
        } else {
            queueDiv.style.display = 'block';
            queueStatus.textContent = pendingFiles.length + ' 个文件待上传';
        }
    }

    if (queueList) {
        queueList.addEventListener('click', function (e) {
            var btn = e.target.closest('.queue-remove-btn');
            if (!btn) return;
            var idx = parseInt(btn.getAttribute('data-idx'), 10);
            pendingFiles.splice(idx, 1);
            queueList.innerHTML = '';
            pendingFiles.forEach(function (f, i) {
                var li = document.createElement('li');
                li.style.display = 'flex';
                li.style.justifyContent = 'space-between';
                li.style.alignItems = 'center';
                li.innerHTML = '<span>' + f.name + ' (' + formatSize(f.size) + ')</span>' +
                    '<button type="button" class="queue-remove-btn" data-idx="' + i + '" data-queue="sticker" title="移除">&times;</button>';
                queueList.appendChild(li);
            });
            refreshStickerQueue();
        });
    }

    if (dropZone && fileInput) {
        // Click to select
        dropZone.addEventListener('click', function (e) {
            if (e.target.tagName !== 'BUTTON') {
                fileInput.click();
            }
        });

        // Drag events
        dropZone.addEventListener('dragover', function (e) {
            e.preventDefault();
            dropZone.classList.add('sticker-drop-zone--drag-over');
        });
        dropZone.addEventListener('dragleave', function (e) {
            dropZone.classList.remove('sticker-drop-zone--drag-over');
        });
        dropZone.addEventListener('drop', function (e) {
            e.preventDefault();
            dropZone.classList.remove('sticker-drop-zone--drag-over');
            if (e.dataTransfer.files.length > 0) {
                addFiles(e.dataTransfer.files);
            }
        });

        // File input change
        fileInput.addEventListener('change', function () {
            if (fileInput.files.length > 0) {
                addFiles(fileInput.files);
                fileInput.value = '';
            }
        });
    }

    window.uploadStickerBatch = function () {
        if (pendingFiles.length === 0) return;

        queueStatus.textContent = '上传中…';
        var total = pendingFiles.length;
        var done = 0;
        var failed = 0;

        function uploadNext(idx) {
            if (idx >= pendingFiles.length) {
                queueStatus.textContent = '完成：' + done + ' 成功' + (failed > 0 ? '，' + failed + ' 失败' : '');
                pendingFiles = [];
                queueList.innerHTML = '';
                // 刷新页面显示新资源
                setTimeout(function () { location.reload(); }, 800);
                return;
            }

            var file = pendingFiles[idx];
            var formData = new FormData();
            formData.append('file', file);

            fetch('/admin/stickers', {
                method: 'POST',
                body: formData
            }).then(function (res) {
                if (res.ok || res.redirected) {
                    done++;
                } else {
                    failed++;
                }
                queueStatus.textContent = '上传中… ' + (done + failed) + '/' + total;
                // 标记列表项
                var items = queueList.querySelectorAll('li');
                if (items[idx]) {
                    items[idx].textContent = (res.ok || res.redirected ? '✓ ' : '✗ ') + items[idx].textContent;
                }
                uploadNext(idx + 1);
            }).catch(function () {
                failed++;
                queueStatus.textContent = '上传中… ' + (done + failed) + '/' + total;
                uploadNext(idx + 1);
            });
        }

        uploadNext(0);
    };
})();

// 文件大小格式化（编辑器 & 表情包共用）
function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / 1048576).toFixed(1) + ' MB';
}

// 点击已有标签，添加到 textarea
window.addTag = function (btn) {
    var tagName = btn.getAttribute('data-tag');
    var textarea = document.getElementById('tags');
    if (!textarea || !tagName) return;

    var lines = textarea.value.split(/[\r\n]+/).filter(function (t) { return t.trim() !== ''; });
    // 如果已存在则不重复添加
    if (lines.indexOf(tagName) >= 0) return;

    if (textarea.value.trim() === '') {
        textarea.value = tagName;
    } else {
        // 确保末尾有换行
        textarea.value = textarea.value.replace(/[\r\n]*$/, '\n') + tagName;
    }
    updatePreview();
};

// 全局视频播放器切换（模板 onclick 调用）
function toggleVideoPlayer(container) {
    var video = container.querySelector('video');
    if (!video) return;
    if (video.paused) {
        video.play();
        container.classList.add('playing');
    } else {
        video.pause();
        container.classList.remove('playing');
    }
}

// 表情包预览弹窗
function previewSticker(url, isVideo) {
    var modal = document.getElementById('sticker-modal');
    var content = document.getElementById('sticker-modal-content');
    if (!modal || !content) return;

    if (isVideo) {
        content.innerHTML = '<video src="' + url + '" controls autoplay style="max-width:80vw;max-height:70vh;"></video>';
    } else {
        content.innerHTML = '<img src="' + url + '" style="max-width:80vw;max-height:70vh;object-fit:contain;">';
    }
    modal.style.display = 'flex';
}

function closeStickerPreview(e) {
    if (!e || e.target === document.getElementById('sticker-modal') || e.target.className === 'sticker-modal__close') {
        var modal = document.getElementById('sticker-modal');
        var content = document.getElementById('sticker-modal-content');
        if (modal) modal.style.display = 'none';
        if (content) content.innerHTML = '';
    }
}

// ---- 严格懒加载：IntersectionObserver ----
(function () {
    // 如果浏览器不支持 IntersectionObserver，直接恢复所有图片
    if (!window.IntersectionObserver) {
        document.querySelectorAll('.post-full__body img[data-src]').forEach(function (img) {
            img.src = img.getAttribute('data-src');
            img.removeAttribute('data-src');
            img.classList.remove('img-pending');
        });
        return;
    }

    var observer = new IntersectionObserver(function (entries) {
        entries.forEach(function (entry) {
            if (!entry.isIntersecting) return;
            var img = entry.target;
            var src = img.getAttribute('data-src');
            if (!src) return;
            // 开始加载
            img.src = src;
            img.removeAttribute('data-src');
            img.classList.remove('img-pending');
            // 加载完成后的处理
            img.addEventListener('load', function () {
                img.classList.add('img-loaded');
            }, { once: true });
            img.addEventListener('error', function () {
                img.classList.add('img-error');
            }, { once: true });
            observer.unobserve(img);
        });
    }, {
        rootMargin: '300px',   // 提前 300px 开始加载（滑到附近就开始，避免用户等待）
        threshold: 0
    });

    // 观察所有带 data-src 的图片
    document.querySelectorAll('.post-full__body img[data-src]').forEach(function (img) {
        observer.observe(img);
    });
})();

// ---- 正文图片全屏灯箱（dialog） ----
(function () {
    var body = document.querySelector('.post-full__body');
    if (!body) return;

    // 收集正文中的所有图片
    var images = Array.from(body.querySelectorAll('img'));
    if (images.length === 0) return;

    // 创建 dialog
    var dialog = document.createElement('dialog');
    dialog.className = 'img-lightbox';
    dialog.innerHTML =
        '<div class="img-lightbox__inner">' +
        '  <button class="img-lightbox__close" aria-label="关闭" title="关闭 (Esc)">&times;</button>' +
        '  <button class="img-lightbox__prev" aria-label="上一张" title="上一张 (←)">&#8249;</button>' +
        '  <button class="img-lightbox__next" aria-label="下一张" title="下一张 (→)">&#8250;</button>' +
        '  <img class="img-lightbox__img" src="" alt="">' +
        '  <span class="img-lightbox__counter"></span>' +
        '</div>';
    document.body.appendChild(dialog);

    var inner      = dialog.querySelector('.img-lightbox__inner');
    var lightImg   = dialog.querySelector('.img-lightbox__img');
    var closeBtn   = dialog.querySelector('.img-lightbox__close');
    var prevBtn    = dialog.querySelector('.img-lightbox__prev');
    var nextBtn    = dialog.querySelector('.img-lightbox__next');
    var counterEl  = dialog.querySelector('.img-lightbox__counter');

    var currentIdx = 0;
    var lightboxBlobURL = null; // track blob URL for cleanup

    function show(idx) {
        idx = Math.max(0, Math.min(idx, images.length - 1));
        currentIdx = idx;
        var img = images[idx];
        var alt = img.getAttribute('alt') || '';

        // Revoke previous lightbox blob URL
        if (lightboxBlobURL) {
            URL.revokeObjectURL(lightboxBlobURL);
            lightboxBlobURL = null;
        }

        // If this image uses blob loading, always re-fetch a fresh blob URL
        // (the original was revoked after page load to prevent new-tab access)
        // data-mid may have been removed after load; also check blob-loaded class
        var isBlob = img.getAttribute('data-mid') !== null || img.classList.contains('blob-loaded');
        if (isBlob && typeof window.loadBlobMedia === 'function') {
            window.loadBlobMedia(img).then(function (result) {
                lightboxBlobURL = result.blobURL;
                lightImg.src = result.blobURL;
                lightImg.alt = alt;
                // Only revoke actual blob URLs (videos use proxy URL directly)
                if (result.blobURL && result.blobURL.indexOf('blob:') === 0) {
                    lightImg.onload = function () {
                        URL.revokeObjectURL(result.blobURL);
                        lightboxBlobURL = null;
                    };
                    lightImg.onerror = function () {
                        URL.revokeObjectURL(result.blobURL);
                        lightboxBlobURL = null;
                    };
                }
            }).catch(function () {
                lightImg.src = img.getAttribute('src') || '';
                lightImg.alt = alt;
            });
            // Show loading state: use current src (might be revoked, so show alt text)
            var currentSrc = img.getAttribute('src');
            if (currentSrc && currentSrc.indexOf('blob:') === 0) {
                // Don't set a revoked blob URL; image will update when fetch completes
            } else if (currentSrc) {
                lightImg.src = currentSrc;
            }
        } else {
            // Non-blob image: use data-src or src directly
            var src = img.getAttribute('data-src') || img.getAttribute('src');
            lightImg.src = src || '';
            lightImg.alt = alt;
        }

        // 导航按钮显隐
        prevBtn.hidden = (images.length <= 1);
        nextBtn.hidden = (images.length <= 1);

        // 计数器
        counterEl.textContent = (idx + 1) + ' / ' + images.length;
    }

    function open(idx) {
        show(idx);
        if (!dialog.open) dialog.showModal();
    }

    function close() {
        dialog.close();
        // 关闭后释放图片内存（避免大图滞留）
        setTimeout(function () {
            if (lightboxBlobURL && lightboxBlobURL.indexOf('blob:') === 0) {
                URL.revokeObjectURL(lightboxBlobURL);
                lightboxBlobURL = null;
            }
            lightImg.src = '';
        }, 200);
    }

    function next() { show(currentIdx + 1); }
    function prev() { show(currentIdx - 1); }

    // ---- 事件绑定 ----
    // 点击正文图片
    images.forEach(function (img, i) {
        // 用闭包保存索引，避免 addEventListener 的 i 被覆盖
        img.addEventListener('click', function (e) {
            e.preventDefault();
            open(i);
        });
    });

    // 关闭按钮
    closeBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        close();
    });

    // 点击背景
    dialog.addEventListener('click', function (e) {
        if (e.target === dialog) close();
    });

    // 点击大图本身 → 关闭（zoom-out 体验）
    lightImg.addEventListener('click', function (e) {
        e.stopPropagation();
        close();
    });

    // 导航按钮
    prevBtn.addEventListener('click', function (e) { e.stopPropagation(); prev(); });
    nextBtn.addEventListener('click', function (e) { e.stopPropagation(); next(); });

    // 键盘
    document.addEventListener('keydown', function (e) {
        if (!dialog.open) return;
        switch (e.key) {
            case 'Escape':   break;           // dialog 原生处理
            case 'ArrowLeft':  e.preventDefault(); prev(); break;
            case 'ArrowRight': e.preventDefault(); next(); break;
        }
    });

    // 触摸滑动（移动端）
    var touchStartX = 0;
    inner.addEventListener('touchstart', function (e) {
        touchStartX = e.touches[0].clientX;
    }, { passive: true });
    inner.addEventListener('touchend', function (e) {
        var dx = e.changedTouches[0].clientX - touchStartX;
        if (Math.abs(dx) > 60) {
            dx > 0 ? prev() : next();
        }
    });
})();

// ==========================================
// 摸鱼模式 (Fish Mode) — Ctrl+B 切换
// ==========================================
(function () {
	'use strict';

	var fishModeActive = false;
	var originalTitle = document.title;
	var fishModeTitle = window._ofoFishModeTitle || '工作周报 - 2024';

	function showToast(msg) {
		var t = document.createElement('div');
		t.textContent = msg;
		t.style.cssText = 'position:fixed;top:80px;left:50%;transform:translateX(-50%);z-index:9999;' +
			'background:#000;color:#FFD740;padding:0.5rem 1.5rem;border-radius:5px;font-weight:700;' +
			'font-size:0.95rem;transition:opacity 0.3s;pointer-events:none;';
		document.body.appendChild(t);
		setTimeout(function () { t.style.opacity = '0'; }, 1500);
		setTimeout(function () { t.remove(); }, 2000);
	}

	function enableFishMode() {
		fishModeActive = true;
		document.title = fishModeTitle;
		document.body.classList.add('fish-mode');
		document.documentElement.setAttribute('data-fish-mode', 'true');
		localStorage.setItem('ofo-fish-mode', 'true');
		var btn = document.getElementById('fish-mode-toggle');
		if (btn) btn.querySelector('.fish-mode-toggle__icon').textContent = '\u2713';
		showToast('\u6478\u9C7C\u6A21\u5F0F\u5DF2\u5F00\u542F \u2014 \u6807\u9898\u680F\u5DF2\u4F2A\u88C5');
	}

	function disableFishMode() {
		fishModeActive = false;
		document.title = originalTitle;
		document.body.classList.remove('fish-mode');
		document.documentElement.setAttribute('data-fish-mode', 'false');
		localStorage.setItem('ofo-fish-mode', 'false');
		var btn = document.getElementById('fish-mode-toggle');
		if (btn) btn.querySelector('.fish-mode-toggle__icon').textContent = '\uD83D\uDC1F';
		showToast('\u6478\u9C7C\u6A21\u5F0F\u5DF2\u5173\u95ED');
	}

	var fishToggle = document.getElementById('fish-mode-toggle');
	if (fishToggle) {
		fishToggle.addEventListener('click', function () {
			fishModeActive ? disableFishMode() : enableFishMode();
		});
	}

	document.addEventListener('keydown', function (e) {
		if (e.ctrlKey && e.key === 'b') {
			e.preventDefault();
			fishModeActive ? disableFishMode() : enableFishMode();
		}
	});

	if (localStorage.getItem('ofo-fish-mode') === 'true') {
		enableFishMode();
	}

	// ==========================================
	// 老板键 (Boss Key) — Ctrl+Shift+H 一键伪装
	// ==========================================
	var bossModeActive = false;
	var bossOverlay = null;

	function createBossOverlay() {
		bossOverlay = document.createElement('div');
		bossOverlay.id = 'boss-overlay';
		bossOverlay.style.cssText =
			'position:fixed;inset:0;z-index:999999;background:var(--bg-secondary,#fff);' +
			'display:flex;flex-direction:column;align-items:center;justify-content:center;font-family:var(--font-body);';
		bossOverlay.innerHTML =
			'<div style="max-width:600px;padding:2rem;text-align:center;">' +
			'<h1 style="font-size:1.5rem;margin-bottom:1rem;color:var(--text-primary);">工作周报 - 2024年第30周</h1>' +
			'<table style="width:100%;border-collapse:collapse;margin:1rem 0;font-size:0.9rem;color:var(--text-primary);">' +
			'<tr style="border-bottom:1px solid #ddd;"><th style="text-align:left;padding:0.5rem;">项目</th><th style="padding:0.5rem;">进度</th></tr>' +
			'<tr><td style="padding:0.5rem;">系统优化</td><td style="padding:0.5rem;text-align:center;color:green;">85%</td></tr>' +
			'<tr><td style="padding:0.5rem;">接口开发</td><td style="padding:0.5rem;text-align:center;color:green;">90%</td></tr>' +
			'<tr><td style="padding:0.5rem;">文档更新</td><td style="padding:0.5rem;text-align:center;color:orange;">60%</td></tr>' +
			'</table>' +
			'<p style="color:var(--text-muted);margin-top:2rem;font-size:0.85rem;">按 <kbd>Esc</kbd> 返回摸鱼</p>' +
			'</div>';
		document.body.appendChild(bossOverlay);
	}

	function activateBossKey() {
		if (!bossOverlay) createBossOverlay();
		bossModeActive = true;
		bossOverlay.style.display = 'flex';
		document.title = '工作周报 - 2024';
	}

	function deactivateBossKey() {
		bossModeActive = false;
		bossOverlay.style.display = 'none';
		document.title = fishModeActive ? fishModeTitle : originalTitle;
	}

	document.addEventListener('keydown', function (e) {
		if (e.ctrlKey && e.shiftKey && e.key === 'H') {
			e.preventDefault();
			bossModeActive ? deactivateBossKey() : activateBossKey();
		}
		if (e.key === 'Escape' && bossModeActive) {
			e.preventDefault();
			deactivateBossKey();
		}
	});

	var bossKeyBtn = document.getElementById('boss-key-btn');
	if (bossKeyBtn) {
		bossKeyBtn.addEventListener('click', function () {
			bossModeActive ? deactivateBossKey() : activateBossKey();
		});
	}
})();
