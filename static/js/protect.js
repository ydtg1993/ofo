/**
 * ofo Production Protection
 *
 * Disables right-click, F12/devtools, and text selection/copying
 * in production mode (DEBUG=false).
 * Admin pages do NOT load this script.
 */
(function () {
    'use strict';

    // ---- Right-click ----
    document.addEventListener('contextmenu', function (e) {
        e.preventDefault();
        return false;
    });

    // ---- Text selection / copy ----
    document.addEventListener('selectstart', function (e) {
        e.preventDefault();
        return false;
    });
    document.addEventListener('copy', function (e) {
        e.preventDefault();
        return false;
    });
    document.addEventListener('cut', function (e) {
        e.preventDefault();
        return false;
    });

    // CSS backup: user-select none on body
    document.documentElement.style.userSelect = 'none';
    document.documentElement.style.webkitUserSelect = 'none';

    // ---- F12 / DevTools keyboard shortcuts ----
    document.addEventListener('keydown', function (e) {
        // F12
        if (e.key === 'F12' || e.keyCode === 123) {
            e.preventDefault();
            return false;
        }
        // Ctrl+Shift+I / Ctrl+Shift+J / Ctrl+Shift+C
        if (e.ctrlKey && e.shiftKey) {
            var k = e.keyCode || e.key;
            if (k === 73 || k === 74 || k === 67 || k === 'I' || k === 'J' || k === 'C') {
                e.preventDefault();
                return false;
            }
        }
        // Ctrl+U (view source)
        if (e.ctrlKey && (e.keyCode === 85 || e.key === 'U' || e.key === 'u')) {
            e.preventDefault();
            return false;
        }
        // Ctrl+S (save page)
        if (e.ctrlKey && (e.keyCode === 83 || e.key === 'S' || e.key === 's')) {
            e.preventDefault();
            return false;
        }
    });

    // ---- DevTools detection (timing-based) ----
    var threshold = 100; // ms — if console.log takes > this, devtools is open
    var checkTimer = null;

    function detectDevTools() {
        var start = performance.now();
        // Using the debugger trick: in some browsers this creates a
        // measurable delay when devtools is open
        var obj = {};
        Object.defineProperty(obj, 'x', {
            get: function () {
                // Intentionally empty — timing side-channel
            }
        });
        obj.x; // trigger getter
        var elapsed = performance.now() - start;

        if (elapsed > threshold) {
            // DevTools detected — aggressively block
            document.body.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100vh;font-size:1.5rem;color:#999;">请关闭开发者工具后刷新页面</div>';
            if (checkTimer) clearInterval(checkTimer);
        }
    }

    // Run detection periodically
    checkTimer = setInterval(detectDevTools, 1500);

    // ---- Console clearing ----
    // Make it harder to use console
    if (typeof console !== 'undefined') {
        var noop = function () {};
        // Periodically clear console
        setInterval(function () {
            if (typeof console.clear === 'function') {
                console.clear();
            }
        }, 3000);
    }

})();
