// The phone menu. It is a <details>, so it opens and closes with no script
// at all; this only adds what a menu should also do: close once a link is
// chosen (an in-page anchor would otherwise leave it covering the section
// it just scrolled to), close on Escape or a tap outside, and mark the
// current page, which matters on /drop because it shares send.html.

for (const menu of document.querySelectorAll('.nav-menu')) {
  const summary = menu.querySelector('summary');

  for (const a of menu.querySelectorAll('a')) {
    if (a.getAttribute('href') === location.pathname) a.setAttribute('aria-current', 'page');
    else a.removeAttribute('aria-current');
  }

  menu.addEventListener('click', (e) => {
    if (e.target.closest('a')) menu.open = false;
  });
  document.addEventListener('click', (e) => {
    if (menu.open && !menu.contains(e.target)) menu.open = false;
  });
  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape' || !menu.open) return;
    menu.open = false;
    summary.focus();
  });
}
