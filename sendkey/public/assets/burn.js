// Inline burn demo for the landing page. The page says "it opens once" in
// prose several times; this shows it instead. A staged link opens, the
// plaintext appears, then the link fades out and comes back dead. Pure
// theater on the client: no secret is created and no request leaves the page.

const card = document.getElementById('burn');
if (card) init(card);

function init(card) {
  const panes = card.querySelectorAll('.burn-pane');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  let running = false;

  function show(state) {
    for (const p of panes) p.classList.toggle('is-on', p.dataset.state === state);
  }

  const wait = (ms) => new Promise((ok) => setTimeout(ok, ms));

  async function run() {
    if (running) return;
    running = true;
    show('open');
    // long enough to actually read the plaintext before it goes
    await wait(reduced.matches ? 1400 : 1900);
    if (!reduced.matches) {
      // fade the link out, swap it to its dead face while hidden, fade back
      card.classList.add('is-burning');
      await wait(400);
    }
    card.classList.add('is-dead');
    card.classList.remove('is-burning');
    show('dead');
    running = false;
  }

  function reset() {
    if (running) return;
    card.classList.remove('is-dead');
    show('idle');
  }

  card.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-burn]');
    if (!btn) return;
    if (btn.dataset.burn === 'run') run();
    else reset();
  });
}
