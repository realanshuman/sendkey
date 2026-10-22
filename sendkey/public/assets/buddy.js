// The keyhole buddy. Presentation only: it reads state the page already
// shows (which section is visible, .arrived on a status line, focus on the
// passphrase field, the burn demo's panes) and turns it into a face. It
// never calls into the app's scripts, and nothing in them knows it exists.
//
// Faces live in brand.css under .buddy[data-mood]. This file only picks the
// mood, plus three small motions: pupils that follow a mouse pointer, an
// occasional blink, and a hop or a head shake when something happens. All
// motion stays off under prefers-reduced-motion.

const buddies = [...document.querySelectorAll('svg.buddy')];
if (buddies.length) init();

function init() {
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  const fine = matchMedia('(hover: hover) and (pointer: fine)');
  const $ = (id) => document.getElementById(id);
  const shown = (id) => {
    const el = $(id);
    return !!el && !el.closest('.hidden');
  };
  const arrived = (id) => shown(id) && $(id).classList.contains('arrived');
  const focused = (id) => !!document.activeElement && document.activeElement.id === id;

  // Each live buddy names what it watches in data-buddy.
  const moods = {
    composer() {
      if (shown('compose-result')) return arrived('receiptStatus') ? 'delighted' : 'happy';
      if (focused('pass')) return 'shy';
      return 'idle';
    },
    receive() {
      if (shown('gone')) return 'sad';
      if (shown('out')) return 'happy';
      if (shown('passgate')) return focused('pass') ? 'shy' : 'curious';
      if (shown('gate')) return 'curious';
      return 'idle';
    },
    ask() {
      if (shown('dead')) return 'sad';
      if (shown('opened') || shown('answered')) return 'happy';
      if (shown('owner')) return arrived('askStatus') ? 'delighted' : 'idle';
      if (shown('respond')) return 'curious';
      return 'idle';
    },
    burn() {
      const card = $('burn');
      if (!card) return 'idle';
      if (card.classList.contains('is-dead')) return 'sad';
      const on = card.querySelector('.burn-pane.is-on');
      return on && on.dataset.state === 'open' ? 'happy' : 'idle';
    },
  };
  // error lines that make the buddy wince for a moment when they appear
  const errors = {
    composer: ['err'],
    receive: ['passerr'],
    ask: ['askErr', 'ansErr'],
    burn: [],
  };
  const OPEN_EYED = new Set(['idle', 'curious', 'oops']);
  const MOTION = ['is-hop', 'is-hop2', 'is-shake'];

  const state = buddies.map((el) => {
    el.addEventListener('animationend', () => el.classList.remove(...MOTION));
    return {
      el,
      kind: moods[el.dataset.buddy] ? el.dataset.buddy : null,
      mood: el.dataset.mood || 'idle',
      follow: el.querySelector('.b-follow'),
      errs: new Map(),
      oopsUntil: 0,
    };
  });

  function play(el, cls) {
    if (reduced.matches) return;
    el.classList.remove(...MOTION);
    void el.getBoundingClientRect(); // restart the animation if it is mid-run
    el.classList.add(cls);
  }

  function update() {
    const now = performance.now();
    for (const b of state) {
      if (!b.kind) continue;
      for (const id of errors[b.kind]) {
        const on = shown(id);
        if (on && !b.errs.get(id)) {
          b.oopsUntil = now + 1100;
          play(b.el, 'is-shake');
          setTimeout(schedule, 1150);
        }
        b.errs.set(id, on);
      }
      let mood = moods[b.kind]();
      if (now < b.oopsUntil && (mood === 'idle' || mood === 'shy' || mood === 'curious')) mood = 'oops';
      if (mood === b.mood) continue;
      b.mood = mood;
      b.el.dataset.mood = mood;
      if (mood === 'happy') play(b.el, 'is-hop');
      else if (mood === 'delighted') play(b.el, 'is-hop2');
    }
  }

  let queued = false;
  function schedule() {
    if (queued) return;
    queued = true;
    requestAnimationFrame(() => { queued = false; update(); });
  }

  new MutationObserver(schedule).observe(document.body, {
    subtree: true, attributes: true, attributeFilter: ['class'],
  });
  document.addEventListener('focusin', schedule);
  document.addEventListener('focusout', schedule);
  update();

  // ---- pupils follow a mouse, never a finger -------------------------------------
  let px = 0, py = 0, frame = 0;
  function look() {
    frame = 0;
    for (const b of state) {
      if (!b.follow) continue;
      const r = b.el.getBoundingClientRect();
      if (r.bottom < 0 || r.top > innerHeight) continue;
      const dx = px - (r.left + r.width / 2);
      const dy = py - (r.top + r.height * 0.38);
      const d = Math.hypot(dx, dy) || 1;
      const k = Math.min(1, d / 260) * 3.5;
      b.follow.style.transform =
        `translate(${((dx / d) * k).toFixed(2)}px, ${((dy / d) * k).toFixed(2)}px)`;
    }
  }
  addEventListener('pointermove', (e) => {
    if (e.pointerType !== 'mouse' || !fine.matches || reduced.matches) return;
    px = e.clientX;
    py = e.clientY;
    if (!frame) frame = requestAnimationFrame(look);
  }, { passive: true });

  // ---- an occasional blink, each buddy on its own clock ------------------------------
  for (const b of state) {
    const tick = () => {
      setTimeout(() => {
        if (!reduced.matches && document.visibilityState === 'visible'
            && OPEN_EYED.has(b.el.dataset.mood || 'idle')) {
          b.el.classList.add('is-blink');
          setTimeout(() => b.el.classList.remove('is-blink'), 140);
        }
        tick();
      }, 2600 + Math.random() * 4200);
    };
    tick();
  }
}
