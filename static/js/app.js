// EzWeb client-side utilities
document.addEventListener('htmx:afterSwap', function(event) {
    // Auto-dismiss alerts after 5 seconds
    var alerts = event.detail.target.querySelectorAll('[data-auto-dismiss]');
    alerts.forEach(function(alert) {
        setTimeout(function() {
            alert.style.transition = 'opacity 0.5s';
            alert.style.opacity = '0';
            setTimeout(function() { alert.remove(); }, 500);
        }, 5000);
    });
});
