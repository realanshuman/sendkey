// Theme switcher. Loaded synchronously in <head> on purpose: the stored (or
// system) theme must land on <html> before first paint, or the page flashes
// the wrong colors. The toggle itself is wired once the DOM exists.
(function () {
  var KEY = 'sendkey-theme';
  var mq = window.matchMedia('(prefers-color-scheme: dark)');

  function stored() {
    try { return localStorage.getItem(KEY); } catch (e) { return null; }
  }

  function apply(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute('content', theme === 'dark' ? '#121210' : '#f2efe9');
    var btn = document.getElementById('themeBtn');
    if (btn) btn.setAttribute('aria-pressed', theme === 'dark' ? 'true' : 'false');
  }

  apply(stored() || (mq.matches ? 'dark' : 'light'));

  // Follow the system for users who never chose explicitly.
  mq.addEventListener('change', function (e) {
    if (!stored()) apply(e.matches ? 'dark' : 'light');
  });

  document.addEventListener('DOMContentLoaded', function () {
    var btn = document.getElementById('themeBtn');
    if (!btn) return;
    apply(document.documentElement.getAttribute('data-theme') || 'light');
    btn.addEventListener('click', function () {
      var next = document.documentElement.getAttribute('data-theme') === 'dark'
        ? 'light' : 'dark';
      // Transitions exist only during the flip, so the page stays snappy at
      // rest and the switch still reads as one smooth move.
      document.documentElement.classList.add('theme-anim');
      apply(next);
      try { localStorage.setItem(KEY, next); } catch (e) { /* private mode */ }
      setTimeout(function () {
        document.documentElement.classList.remove('theme-anim');
      }, 300);
    });
  });
})();
