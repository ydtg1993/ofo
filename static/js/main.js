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

        // --- Lazy Load Posts ---
        var POSTS_PER_BATCH = 10;
        var allPosts = document.querySelectorAll('.lazy-post');
        var loadMoreWrap = document.getElementById('load-more-wrap');
        var loadMoreBtn = document.getElementById('load-more-btn');
        var loadMoreRemaining = document.getElementById('load-more-remaining');
        var visibleCount = POSTS_PER_BATCH;

        function updateLoadMore() {
            var hiddenPosts = document.querySelectorAll('.lazy-post.post-card--hidden');
            if (hiddenPosts.length === 0) {
                if (loadMoreWrap) loadMoreWrap.style.display = 'none';
            } else {
                if (loadMoreWrap) loadMoreWrap.style.display = 'block';
                if (loadMoreRemaining) {
                    loadMoreRemaining.textContent = '（还有 ' + hiddenPosts.length + ' 篇）';
                }
            }
        }

        // Initially show only first batch
        allPosts.forEach(function (post, i) {
            if (i < POSTS_PER_BATCH) {
                post.classList.remove('post-card--hidden');
            }
        });
        updateLoadMore();

        if (loadMoreBtn) {
            loadMoreBtn.addEventListener('click', function () {
                var hidden = document.querySelectorAll('.lazy-post.post-card--hidden');
                var toShow = Math.min(POSTS_PER_BATCH, hidden.length);
                for (var i = 0; i < toShow; i++) {
                    hidden[i].classList.remove('post-card--hidden');
                }
                updateLoadMore();
            });
        }

        // --- File Upload ---
        var fileInput = document.getElementById('file-upload');
        if (fileInput) {
            fileInput.addEventListener('change', function () {
                handleUpload(this);
            });
        }
    });

    // --- Markdown Preview ---
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
            // Use marked if available, otherwise escape
            if (typeof marked !== 'undefined' && typeof marked.parse === 'function') {
                html = marked.parse(md);
            } else if (typeof marked !== 'undefined' && typeof marked === 'function') {
                html = marked(md);
            } else {
                html = escapeHtmlForPreview(md);
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

    function addEditorFiles(files) {
        for (var i = 0; i < files.length; i++) {
            var f = files[i];
            var ext = f.name.split('.').pop().toLowerCase();
            var allowed = ['jpg','jpeg','png','gif','webp','mp4','webm','ogg','mov'];
            if (allowed.indexOf(ext) < 0) continue;
            editorPendingFiles.push(f);
            var li = document.createElement('li');
            li.textContent = f.name + ' (' + formatSize(f.size) + ')';
            editorQueueList.appendChild(li);
        }
        if (editorPendingFiles.length > 0) {
            editorQueueDiv.style.display = 'block';
            editorQueueStatus.textContent = editorPendingFiles.length + ' 个文件待上传';
        }
    }

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

            fetch('/admin/upload', {
                method: 'POST',
                body: formData
            })
            .then(function (res) { return res.json().then(function (data) { return {ok: res.ok, data: data}; }); })
            .then(function (result) {
                if (result.ok && result.data.url) {
                    done++;
                    var url = result.data.url;
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
                    if (items[idx]) items[idx].textContent = '✓ ' + file.name;
                } else {
                    failed++;
                    var items2 = editorQueueList.querySelectorAll('li');
                    if (items2[idx]) items2[idx].textContent = '✗ ' + file.name;
                }
                editorQueueStatus.textContent = '上传中… ' + (done + failed) + '/' + total;
                uploadNext(idx + 1);
            })
            .catch(function () {
                failed++;
                editorQueueStatus.textContent = '上传中… ' + (done + failed) + '/' + total;
                uploadNext(idx + 1);
            });
        }

        uploadNext(0);
    };

    // --- Helpers ---

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
            // validate type
            var ext = f.name.split('.').pop().toLowerCase();
            var allowed = ['jpg','jpeg','png','gif','webp','mp4','webm','ogg','mov'];
            if (allowed.indexOf(ext) < 0) continue;
            pendingFiles.push(f);
            var li = document.createElement('li');
            li.textContent = f.name + ' (' + formatSize(f.size) + ')';
            queueList.appendChild(li);
        }
        if (pendingFiles.length > 0) {
            queueDiv.style.display = 'block';
            queueStatus.textContent = pendingFiles.length + ' 个文件待上传';
        }
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
