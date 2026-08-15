// Inline burn demo for the landing page. The page says "it opens once" in
// prose several times; this shows it instead. A staged link opens, the
// plaintext appears, then the link dissolves into ordered-dither noise and
// comes back dead. Pure theater on the client: no secret is created and no
// request leaves the page.
//
// The dissolve uses the same Bayer 8x8 threshold matrix that generates the
// brand's texture PNGs (tools/dither/main.go), so the noise the link burns
// into is literally the site's own grain.

const BAYER8 = [
  [0, 32, 8, 40, 2, 34, 10, 42],
  [48, 16, 56, 24, 50, 18, 58, 26],
  [12, 44, 4, 36, 14, 46, 6, 38],
  [60, 28, 52, 20, 62, 30, 54, 22],
  [3, 35, 11, 43, 1, 33, 9, 41],
  [51, 19, 59, 27, 49, 17, 57, 25],
  [15, 47, 7, 39, 13, 45, 5, 37],
  [63, 31, 55, 23, 61, 29, 53, 21],
];

const card = document.getElementById('burn');
if (card) init(card);

function init(card) {
  const link = document.getElementById('burnLink');
  const canvas = document.getElementById('burnNoise');
  const panes = card.querySelectorAll('.burn-pane');
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  let running = false;

  function show(state) {
    for (const p of panes) p.classList.toggle('is-on', p.dataset.state === state);
  }

  // The dissolve sweeps a density front across the link: behind the front
  // the noise is solid, ahead of it clear, with a dithered ramp between.
  // Density is quantized to eighths so the ramp reads as chunky 1-bit
  // texture, not a smooth fade. One full pass covers the link in ink, the
  // second half retreats to reveal whatever the link looks like now.
  function dissolve(onCovered) {
    return new Promise((done) => {
      const scale = 2; // one canvas pixel is a 2px screen cell
      const w = Math.max(1, Math.round(link.offsetWidth / scale));
      const h = Math.max(1, Math.round(link.offsetHeight / scale));
      canvas.width = w;
      canvas.height = h;
      const ctx = canvas.getContext('2d');
      const ink = getComputedStyle(document.body).color;
      const ramp = w * 0.55; // width of the dithered edge, in cells
      const IN = 520, OUT = 520; // ms per half
      let covered = false;
      canvas.classList.add('is-live');

      let t0;
      function frame(now) {
        t0 ??= now;
        const t = now - t0;
        const phase = t < IN ? t / IN : Math.min(1, (t - IN) / OUT);
        // the front runs past the far edge so the ramp fully clears
        const front = (t < IN ? phase : 1 - phase) * (w + ramp);
        ctx.clearRect(0, 0, w, h);
        ctx.fillStyle = ink;
        for (let y = 0; y < h; y++) {
          for (let x = 0; x < w; x++) {
            let d = (front - x) / ramp;
            d = d < 0 ? 0 : d > 1 ? 1 : Math.round(d * 8) / 8;
            if (d * 64 > BAYER8[y % 8][x % 8]) ctx.fillRect(x, y, 1, 1);
          }
        }
        if (!covered && t >= IN) {
          covered = true;
          onCovered(); // swap the link to its dead face while it is hidden
        }
        if (t < IN + OUT) {
          requestAnimationFrame(frame);
        } else {
          ctx.clearRect(0, 0, w, h);
          canvas.classList.remove('is-live');
          done();
        }
      }
      requestAnimationFrame(frame);
    });
  }

  const wait = (ms) => new Promise((ok) => setTimeout(ok, ms));

  async function run() {
    if (running) return;
    running = true;
    show('open');
    // long enough to actually read the plaintext before it goes
    await wait(reduced.matches ? 1400 : 1900);
    if (reduced.matches) {
      card.classList.add('is-dead');
    } else {
      await dissolve(() => card.classList.add('is-dead'));
    }
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
