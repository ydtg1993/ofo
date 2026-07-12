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
            tagNames = tagsEl.value.split(',').map(function(t) { return t.trim(); }).filter(Boolean);
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

    // --- File Upload ---
    window.handleUpload = function (input) {
        var file = input.files[0];
        if (!file) return;

        var statusEl = document.getElementById('upload-status');
        if (statusEl) {
            statusEl.textContent = '上传中…';
        }

        var formData = new FormData();
        formData.append('file', file);

        fetch('/admin/upload', {
            method: 'POST',
            body: formData
        })
        .then(function (res) {
            return res.json().then(function (data) {
                return {ok: res.ok, data: data};
            });
        })
        .then(function (result) {
            if (statusEl) {
                statusEl.textContent = result.ok ? '✓ 上传成功' : '✗ ' + (result.data.error || '失败');
                if (result.ok) {
                    setTimeout(function () { statusEl.textContent = ''; }, 3000);
                }
            }

            if (result.ok && result.data.url) {
                var url = result.data.url;
                var textarea = document.getElementById('content');
                if (!textarea) return;

                var ext = url.split('.').pop().toLowerCase();
                var isVideo = ['mp4', 'webm', 'ogg', 'mov'].indexOf(ext) > -1;

                var insertText;
                if (isVideo) {
                    insertText = '<video src="' + url + '" controls></video>';
                } else {
                    var alt = file.name.replace(/\.[^.]+$/, '');
                    insertText = '![' + alt + '](' + url + ')';
                }

                // Insert at cursor position or append
                var start = textarea.selectionStart;
                var end = textarea.selectionEnd;
                var before = textarea.value.substring(0, start);
                var after = textarea.value.substring(end);
                textarea.value = before + insertText + '\n' + after;
                textarea.focus();
                textarea.selectionStart = textarea.selectionEnd = start + insertText.length + 1;

                updatePreview();
            }
        })
        .catch(function (err) {
            if (statusEl) {
                statusEl.textContent = '✗ 上传失败: ' + err.message;
            }
        });

        // Reset file input
        input.value = '';
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
