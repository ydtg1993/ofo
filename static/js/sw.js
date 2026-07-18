/**
 * ofo Service Worker
 *
 * Blocks navigation to blob: URLs. When someone copies a blob URL
 * and opens it in a new tab (address bar navigation), the SW returns 403.
 *
 * Video/Image elements loading blob: URLs internally are NOT affected —
 * those go through the browser's blob resolver, not the fetch API.
 */
self.addEventListener('fetch', function (event) {
    var url = event.request.url;
    // Only block blob: URL navigations (address bar), not subresource loads
    if (url.startsWith('blob:') && event.request.mode === 'navigate') {
        event.respondWith(new Response(
            '<!DOCTYPE html><html><head><meta charset="utf-8"></head>' +
            '<body style="background:#000;display:flex;align-items:center;' +
            'justify-content:center;height:100vh;margin:0;' +
            'font-family:system-ui;color:#f44;font-size:1.5rem;">' +
            '⛔ Blocked</body></html>',
            { status: 403, headers: { 'Content-Type': 'text/html; charset=utf-8' } }
        ));
        return;
    }
});
