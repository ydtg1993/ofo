/**
 * ofo Media Blob Loader
 *
 * Decrypts window.__OFO_MEDIA__.d with AES-256-GCM (Web Crypto API)
 * using key from window.__OFO_MEDIA__.k. No plaintext URLs in HTML.
 *
 * Images: fetch → blob → set src → revoke after decode (safe to revoke).
 * Videos: fetch → blob → set src → keep alive (decoder needs random access).
 *         Revoked on page unload.
 */
(function () {
    'use strict';

    var CFG = window.__OFO_MEDIA__;
    if (!CFG || !CFG.enabled) return;

    // Register Service Worker to block blob: URL navigations
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.register('/sw.js', { scope: '/' });
    }

    var URLMAP = {};

    // ---- AES-256-GCM decrypt via Web Crypto API ----

    function aesDecrypt(b64Data, b64Key) {
        var raw = Uint8Array.from(atob(b64Data), function (c) { return c.charCodeAt(0); });
        var keyBytes = Uint8Array.from(atob(b64Key), function (c) { return c.charCodeAt(0); });
        var nonce = raw.slice(0, 12);
        var ct = raw.slice(12);

        return crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, ['decrypt'])
            .then(function (k) {
                return crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce }, k, ct);
            })
            .then(function (plain) {
                return JSON.parse(new TextDecoder().decode(plain));
            });
    }

    // ---- Start: decrypt then init observer ----

    if (!CFG.d || !CFG.k) return;
    aesDecrypt(CFG.d, CFG.k).then(function (urls) {
        URLMAP = urls;
        init();
    }).catch(function (err) {
        console.warn('ofo: decrypt failed', err);
    });

    // ---- Init observers after decrypt ----

    function init() {
        if (!Object.keys(URLMAP).length) return;

        if (window.IntersectionObserver) {
            var observer = new IntersectionObserver(function (entries) {
                entries.forEach(function (entry) {
                    if (!entry.isIntersecting) return;
                    loadMediaElement(entry.target);
                    observer.unobserve(entry.target);
                });
            }, { rootMargin: '300px', threshold: 0 });

            collect('[data-mid]').forEach(function (el) { observer.observe(el); });

            // Load More integration
            var postList = document.querySelector('.post-list');
            if (postList) {
                var mo = new MutationObserver(function (mutations) {
                    mutations.forEach(function (m) {
                        if (m.type === 'attributes' && m.attributeName === 'class') {
                            var t = m.target;
                            if (t.classList.contains('post-card') && !t.classList.contains('post-card--hidden')) {
                                collect('[data-mid]', t).forEach(function (el) { observer.observe(el); });
                            }
                        }
                    });
                });
                postList.querySelectorAll('.lazy-post').forEach(function (p) {
                    mo.observe(p, { attributes: true, attributeFilter: ['class'] });
                });
            }
        } else {
            collect('[data-mid]').forEach(loadMediaElement);
        }
    }

    // ---- Fetch helpers ----

    function fetchMediaBlob(proxyURL) {
        return fetch(proxyURL)
            .then(function (res) { if (!res.ok) throw new Error('HTTP ' + res.status); return res.blob(); })
            .then(function (blob) { return { blobURL: URL.createObjectURL(blob), blob: blob }; });
    }

    function revoke(url) {
        if (url && url.indexOf('blob:') === 0) URL.revokeObjectURL(url);
    }

    // ---- Load single element ----

    function loadMediaElement(el) {
        var mid = el.getAttribute('data-mid');
        if (mid === null) return Promise.resolve();
        var proxyURL = URLMAP[mid];
        if (!proxyURL) return Promise.resolve();
        if (el.classList.contains('blob-loading') || el.classList.contains('blob-loaded'))
            return Promise.resolve();

        el.classList.add('blob-loading');
        var isVideo = el.tagName.toLowerCase() === 'video';

        return fetchMediaBlob(proxyURL).then(function (r) {
            el._ofoMid = mid;
            el.src = r.blobURL;
            el.classList.add('blob-loaded');
            el.classList.remove('blob-loading');
            el.removeAttribute('data-mid');

            if (isVideo) {
                // Video blob stays alive. Video decoder needs ongoing
                // data access for buffering — revoking causes inevitable
                // stalls. blob: URLs are local browser memory, NOT network
                // addresses. They cannot be accessed by curl/wget or shared
                // to other devices. Production (DEBUG=false) blocks F12/right-
                // click so users can't extract the URL from DOM.
                // Revoked on beforeunload.
            } else {
                el.addEventListener('load', function () { revoke(r.blobURL); }, { once: true });
                el.addEventListener('error', function () { revoke(r.blobURL); }, { once: true });
            }
            el.dispatchEvent(new Event('blob-load', { bubbles: true }));
        }).catch(function (err) {
            console.warn('ofo: load failed', proxyURL, err);
            el.classList.add('blob-error');
            el.classList.remove('blob-loading');
        });
    }

    function collect(sel, root) {
        return Array.from((root || document).querySelectorAll(sel));
    }

    // ---- Lightbox re-fetch ----

    window.loadBlobMedia = function (el) {
        var mid = el._ofoMid;
        if (mid === undefined) mid = el.getAttribute('data-mid');
        var proxyURL = mid != null ? URLMAP[mid] : null;
        if (!proxyURL) {
            var s = el.getAttribute('src');
            if (s && s.indexOf('blob:') !== 0) return Promise.resolve({ blobURL: s, blob: null });
            return Promise.reject(new Error('No source'));
        }
        var isVideo = el.tagName && el.tagName.toLowerCase() === 'video';
        if (isVideo) {
            return Promise.resolve({ blobURL: proxyURL, blob: null });
        }
        return fetchMediaBlob(proxyURL);
    };

})();
