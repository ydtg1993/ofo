/**
 * ofo Media Blob Loader
 *
 * Reads proxy URLs from window.__OFO_MEDIA__.urls (injected as a JS array
 * by the server). Matches elements by data-mid index attribute.
 *
 * Images and videos: fetch proxy URL → blob → URL.createObjectURL → revoke.
 * Blob URLs are revoked immediately after load — copying to a new tab fails.
 * data-mid is removed from DOM after load. No URLs in HTML at all.
 */
(function () {
    'use strict';

    var CFG = window.__OFO_MEDIA__;
    if (!CFG || !CFG.enabled) return;
    var URLMAP = CFG.urls; // {randomID: proxyURL, ...}
    if (!URLMAP || !Object.keys(URLMAP).length) return;
    var videoBlobs = []; // video blob URLs to revoke on page unload

    function fetchMediaBlob(proxyURL) {
        return fetch(proxyURL)
            .then(function (res) {
                if (!res.ok) throw new Error('HTTP ' + res.status);
                return res.blob();
            })
            .then(function (blob) {
                return { blobURL: URL.createObjectURL(blob), blob: blob };
            });
    }

    function revoke(url) {
        if (!url || url.indexOf('blob:') !== 0) return;
        URL.revokeObjectURL(url);
    }

    function loadMediaElement(el) {
        var mid = el.getAttribute('data-mid');
        if (mid === null) return Promise.resolve();
        var proxyURL = URLMAP[mid];
        if (!proxyURL) return Promise.resolve();
        if (el.classList.contains('blob-loading') || el.classList.contains('blob-loaded'))
            return Promise.resolve();

        el.classList.add('blob-loading');
        var isVideo = el.tagName.toLowerCase() === 'video';

        return fetchMediaBlob(proxyURL)
            .then(function (result) {
                el._ofoMid = mid;
                el.src = result.blobURL;
                el.classList.add('blob-loaded');
                el.classList.remove('blob-loading');
                el.removeAttribute('data-mid');

                if (isVideo) {
                    // Video: keep blob alive for seeking. Revoke on page unload.
                    videoBlobs.push(result.blobURL);
                } else {
                    // Image: revoke after decode.
                    el.addEventListener('load', function () { revoke(result.blobURL); }, { once: true });
                    el.addEventListener('error', function () { revoke(result.blobURL); }, { once: true });
                }

                el.dispatchEvent(new Event('blob-load', { bubbles: true }));
            })
            .catch(function (err) {
                console.warn('ofo: failed to load media blob:', proxyURL, err);
                el.classList.add('blob-error');
                el.classList.remove('blob-loading');
            });
    }

    /**
     * Re-fetch a fresh blob URL for lightbox use.
     * @param {Element} el - must have data-mid or be blob-loaded
     */
    window.loadBlobMedia = function (el) {
        var mid = el._ofoMid;
        if (mid === undefined) mid = el.getAttribute('data-mid');
        var proxyURL = mid !== null && mid !== undefined ? URLMAP[mid] : null;
        if (!proxyURL) {
            var src = el.getAttribute('src');
            if (src && src.indexOf('blob:') !== 0) {
                return Promise.resolve({ blobURL: src, blob: null });
            }
            return Promise.reject(new Error('No media source'));
        }
        return fetchMediaBlob(proxyURL);
    };

    // ---- IntersectionObserver ----
    if (window.IntersectionObserver) {
        var observer = new IntersectionObserver(
            function (entries) {
                entries.forEach(function (entry) {
                    if (!entry.isIntersecting) return;
                    loadMediaElement(entry.target);
                    observer.unobserve(entry.target);
                });
            },
            { rootMargin: '300px', threshold: 0 }
        );
        collect('[data-mid]').forEach(function (el) { observer.observe(el); });
    } else {
        collect('[data-mid]').forEach(loadMediaElement);
    }

    // ---- Load More integration ----
    var postList = document.querySelector('.post-list');
    if (postList && window.IntersectionObserver) {
        var mo = new MutationObserver(function (mutations) {
            mutations.forEach(function (m) {
                if (m.type === 'attributes' && m.attributeName === 'class') {
                    var t = m.target;
                    if (t.classList.contains('post-card') &&
                        !t.classList.contains('post-card--hidden')) {
                        collect('[data-mid]', t).forEach(function (el) {
                            observer.observe(el);
                        });
                    }
                }
            });
        });
        postList.querySelectorAll('.lazy-post').forEach(function (p) {
            mo.observe(p, { attributes: true, attributeFilter: ['class'] });
        });
    }

    function collect(sel, root) {
        return Array.from((root || document).querySelectorAll(sel));
    }

    // Revoke video blob URLs when leaving the page
    window.addEventListener('beforeunload', function () {
        videoBlobs.forEach(revoke);
        videoBlobs = [];
    });

})();
