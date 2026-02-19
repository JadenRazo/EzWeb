// EzWeb client-side utilities

// ── Modal system (native <dialog> API) ──────────────────────────────
var EzModal = (function() {
    function open(id) {
        var el = document.getElementById('modal-' + id);
        if (!el || typeof el.showModal !== 'function') return;
        if (!el.open) el.showModal();
    }

    function close() {
        var openDialog = document.querySelector('dialog[open]');
        if (openDialog) openDialog.close();
    }

    // Delegated click: open triggers + backdrop click-to-close + overlay close
    document.addEventListener('click', function(e) {
        var openBtn = e.target.closest('[data-modal-open]');
        if (openBtn) {
            var id = openBtn.getAttribute('data-modal-open');
            if (id) open(id);
            return;
        }

        var overlayClose = e.target.closest('[data-close-overlay]');
        if (overlayClose) {
            var oid = overlayClose.getAttribute('data-close-overlay');
            var oel = document.getElementById(oid);
            if (oel) {
                oel.classList.remove('is-visible');
                if (oid === 'shortcuts-overlay') localStorage.removeItem('shortcuts-visible');
            }
        }
    });

    // Close on backdrop click (clicking the <dialog> element itself, not its children)
    document.addEventListener('click', function(e) {
        if (e.target.tagName === 'DIALOG' && e.target.open) {
            var rect = e.target.querySelector('.bg-white');
            if (rect) {
                var r = rect.getBoundingClientRect();
                if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) {
                    e.target.close();
                }
            }
        }
    });

    return { open: open, close: close };
})();


// ── Toast notification system ───────────────────────────────────────
(function() {
    var container = document.createElement('div');
    container.id = 'toast-container';
    container.className = 'fixed top-4 right-4 z-[60] flex flex-col gap-2';
    container.style.cssText = 'pointer-events: none;';
    document.body.appendChild(container);

    window.showToast = function(message, type) {
        type = type || 'info';
        var colors = {
            success: 'bg-green-600',
            error: 'bg-red-600',
            warning: 'bg-yellow-500 text-black',
            info: 'bg-blue-600'
        };

        var toast = document.createElement('div');
        toast.className = 'toast-item px-4 py-3 rounded-lg text-white shadow-lg text-sm max-w-sm pointer-events-auto ' + (colors[type] || colors.info);
        toast.textContent = message;
        container.appendChild(toast);

        setTimeout(function() {
            toast.style.transition = 'opacity 0.4s, transform 0.4s';
            toast.style.opacity = '0';
            toast.style.transform = 'translateX(100%)';
            setTimeout(function() { toast.remove(); }, 400);
        }, 4000);
    };
})();

// ── Auto-dismiss alerts ─────────────────────────────────────────────
document.addEventListener('htmx:afterSwap', function(event) {
    var alerts = event.detail.target.querySelectorAll('[data-auto-dismiss]');
    alerts.forEach(function(alert) {
        setTimeout(function() {
            alert.style.transition = 'opacity 0.5s';
            alert.style.opacity = '0';
            setTimeout(function() { alert.remove(); }, 500);
        }, 5000);
    });
});

// ── Global HTMX error handlers ─────────────────────────────────────
document.addEventListener('htmx:responseError', function(event) {
    var status = event.detail.xhr ? event.detail.xhr.status : 0;
    if (status === 401) {
        window.location.href = '/login';
        return;
    }
    var msg = event.detail.xhr ? event.detail.xhr.responseText : 'Request failed';
    if (msg.length > 100) msg = 'An error occurred';
    showToast(msg, 'error');
});

document.addEventListener('htmx:sendError', function() {
    showToast('Network error — check your connection', 'error');
});

// ── Dark mode toggle ────────────────────────────────────────────────
(function() {
    var stored = localStorage.getItem('theme');
    if (stored === 'dark' || (!stored && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
        document.documentElement.classList.add('dark');
    }

    window.toggleDarkMode = function() {
        var isDark = document.documentElement.classList.toggle('dark');
        localStorage.setItem('theme', isDark ? 'dark' : 'light');
    };
})();

// ── Keyboard shortcuts ──────────────────────────────────────────────
(function() {
    var pendingKey = null;

    function showOverlay() {
        var overlay = document.getElementById('shortcuts-overlay');
        if (overlay) {
            overlay.classList.add('is-visible');
            localStorage.setItem('shortcuts-visible', '1');
        }
    }

    function hideOverlay() {
        var overlay = document.getElementById('shortcuts-overlay');
        if (overlay) {
            overlay.classList.remove('is-visible');
            localStorage.removeItem('shortcuts-visible');
        }
    }

    // Restore overlay state on load and after HTMX swaps
    function restoreOverlay() {
        if (localStorage.getItem('shortcuts-visible') === '1') {
            var overlay = document.getElementById('shortcuts-overlay');
            if (overlay) overlay.classList.add('is-visible');
        }
    }

    restoreOverlay();
    document.addEventListener('htmx:afterSettle', restoreOverlay);

    document.addEventListener('keydown', function(e) {
        // Don't trigger when typing in inputs
        var tag = e.target.tagName;
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || e.target.isContentEditable) {
            return;
        }

        // Ctrl+K or Cmd+K — focus search (if exists)
        if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
            e.preventDefault();
            var search = document.querySelector('[data-search]');
            if (search) search.focus();
            return;
        }

        // ? — toggle shortcuts help
        if (e.key === '?') {
            var overlay = document.getElementById('shortcuts-overlay');
            if (overlay && overlay.classList.contains('is-visible')) {
                hideOverlay();
            } else {
                showOverlay();
            }
            return;
        }

        // Escape — close modals/overlays
        if (e.key === 'Escape') {
            var overlay = document.getElementById('shortcuts-overlay');
            if (overlay && overlay.classList.contains('is-visible')) {
                hideOverlay();
                return;
            }
            EzModal.close();
            return;
        }

        // Two-key sequences: g + <key>
        if (pendingKey === 'g') {
            pendingKey = null;
            switch (e.key) {
                case 'd': htmx.ajax('GET', '/dashboard', {target:'body', swap:'innerHTML'}); break;
                case 's': htmx.ajax('GET', '/sites', {target:'body', swap:'innerHTML'}); break;
                case 'c': htmx.ajax('GET', '/customers', {target:'body', swap:'innerHTML'}); break;
                case 'p': htmx.ajax('GET', '/payments', {target:'body', swap:'innerHTML'}); break;
                case 'v': htmx.ajax('GET', '/servers', {target:'body', swap:'innerHTML'}); break;
                case 'b': htmx.ajax('GET', '/backups', {target:'body', swap:'innerHTML'}); break;
            }
            return;
        }

        if (e.key === 'g') {
            pendingKey = 'g';
            setTimeout(function() { pendingKey = null; }, 800);
            return;
        }
    });
})();

// ── CSV export helper ───────────────────────────────────────────────
window.exportCSV = function(url) {
    window.location.href = url;
};
