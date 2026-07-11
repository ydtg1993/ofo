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

        // Extract first image for card preview
        var firstImg = extractFirstImage(html);
        var displayThumb = thumbURL || firstImg || '';
        var isVideo = displayThumb && (displayThumb.indexOf('.mp4') > -1 || displayThumb.indexOf('.webm') > -1 ||
            displayThumb.indexOf('.ogg') > -1 || displayThumb.indexOf('.mov') > -1);

        if (mode === 'card') {
            // Card preview
            var excerpt = stripHtml(html).substring(0, 150) + '...';
            var thumbBlock = '';
            if (displayThumb) {
                thumbBlock = '<div class="post-card__thumb">' +
                    '<img src="' + displayThumb + '" alt="' + title + '">' +
                    (isVideo ? '<span class="post-card__play-icon">▶</span>' : '') +
                    '</div>';
            }
            previewContent.innerHTML =
                '<article class="post-card neo-box' + (displayThumb ? ' post-card--has-thumb' : '') + '">' +
                thumbBlock +
                '<div class="post-card__body">' +
                '<h2 class="post-card__title"><a href="#">' + title + '</a></h2>' +
                '<div class="post-card__meta"><time>预览日期</time></div>' +
                '<p class="post-card__excerpt">' + excerpt + '</p>' +
                '</div></article>';
        } else {
            // Detail preview
            var tagsEl = document.getElementById('tags');
            var catEl = document.getElementById('category_id');
            var tagsText = tagsEl ? tagsEl.value : '';
            var catName = catEl && catEl.selectedIndex >= 0 ? catEl.options[catEl.selectedIndex].text : '';
            if (catName === '— 无 —') catName = '';

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
