// Ask page: one page, two roles. The browser that created the ask holds the
// private key in localStorage and sees the owner view; anyone else opening
// the link sees the responder view. All crypto is in asym.js; the server
// only ever stores one opaque blob at an id both sides derive locally.
import { b64u } from './crypto.js';
import { newRequest, mailboxFor, answer, openAnswer, identicon } from './asym.js';

const $ = (id) => document.getElementById(id);
const show = (id) => $(id).classList.remove('hidden');
const hide = (id) => $(id).classList.add('hidden');
const only = (section) => {
  for (const s of ['create', 'owner', 'opened', 'respond', 'answered', 'dead']) {
    s === section ? show(s) : hide(s);
  }
};

const storeKey = (reqId) => 'sendkey-ask-' + reqId;
// mutable: create mode starts on /ask with no id and assigns one on the fly
let reqId = location.pathname.startsWith('/a/')
  ? location.pathname.slice(3) : '';
const pubFromHash = location.hash.slice(1);

function dead(msg) {
  if (msg) $('deadMsg').textContent = msg;
  $('headline').textContent = 'Nothing to do here';
  only('dead');
}

async function drawIdenticon(el, pubB64) {
  const grid = await identicon(pubB64);
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 8 8');
  svg.setAttribute('shape-rendering', 'crispEdges');
  for (let r = 0; r < 8; r++) {
    for (let c = 0; c < 8; c++) {
      if (!grid[r][c]) continue;
      const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
      rect.setAttribute('x', c); rect.setAttribute('y', r);
      rect.setAttribute('width', 1); rect.setAttribute('height', 1);
      svg.appendChild(rect);
    }
  }
  el.replaceChildren(svg);
}

function wireCopy(btnId, value) {
  $(btnId).addEventListener('click', async () => {
    const btn = $(btnId);
    try {
      await navigator.clipboard.writeText(value());
      btn.textContent = 'Copied ✓';
    } catch {
      btn.textContent = 'Press ⌘C';
    }
    setTimeout(() => { btn.textContent = 'Copy'; }, 1800);
  });
}

// ---- owner role -------------------------------------------------------------

let pollTimer = null;

async function ownerView(record) {
  $('headline').textContent = 'Your ask is live';
  $('tagline').textContent = 'Share the link. The answer can only open here.';
  only('owner');

  $('askLink').value = `${location.origin}/a/${reqId}#${record.pub}`;
  drawIdenticon($('ownerIcon'), record.pub);

  const mailbox = await mailboxFor(reqId);

  async function poll() {
    if (document.visibilityState === 'hidden') return;
    try {
      const res = await fetch(`/api/mailbox/${encodeURIComponent(mailbox)}`);
      const data = await res.json();
      if (!data.ready) return; // nothing yet
      clearInterval(pollTimer);
      $('askStatusText').textContent = 'An answer arrived.';
      $('askStatus').classList.add('arrived');
      show('askReveal');
    } catch { /* network hiccup: the next tick retries */ }
  }
  poll();
  pollTimer = setInterval(poll, 5000);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') poll();
  });

  $('askReveal').addEventListener('click', async () => {
    const btn = $('askReveal');
    btn.disabled = true;
    btn.textContent = 'Decrypting…';
    try {
      // This fetch is the burn: the mailbox is destroyed right here.
      const res = await fetch(`/api/secret/${encodeURIComponent(mailbox)}`);
      if (!res.ok) throw new Error('The answer expired before you opened it.');
      const data = await res.json();
      const secret = await openAnswer(
        record.priv, reqId, b64u.decode(data.ct), b64u.decode(data.iv));
      $('askSecret').textContent = new TextDecoder().decode(secret);
      $('headline').textContent = 'Here is the answer';
      $('tagline').textContent = 'Save it now. The mailbox no longer exists.';
      only('opened');
      try { localStorage.removeItem(storeKey(reqId)); } catch { /* fine */ }
    } catch (e) {
      $('askErr').textContent = e.message === 'bad-key'
        ? 'This answer was not sealed to this browser’s key.'
        : (e.message || 'Something went wrong while decrypting.');
      show('askErr');
      btn.disabled = false;
      btn.textContent = 'Open the answer';
    }
  });

  wireCopy('askCopy', () => $('askLink').value);
}

// ---- responder role ---------------------------------------------------------

async function responderView() {
  $('headline').textContent = 'Answer an ask';
  $('tagline').textContent = 'Encrypted to their key before anything is sent.';
  only('respond');
  drawIdenticon($('respIcon'), pubFromHash);

  const mailbox = await mailboxFor(reqId);
  try {
    const res = await fetch(`/api/mailbox/${encodeURIComponent(mailbox)}`);
    if ((await res.json()).ready) {
      return dead('This ask has already been answered. ' +
        'Each ask accepts exactly one answer.');
    }
  } catch { /* offline check is best effort */ }

  $('ansGo').addEventListener('click', async () => {
    const errEl = $('ansErr');
    errEl.classList.add('hidden');
    const text = $('ansSecret').value;
    if (!text) {
      errEl.textContent = 'Nothing to send yet. Type or paste the secret first.';
      errEl.classList.remove('hidden');
      return;
    }
    const btn = $('ansGo');
    btn.disabled = true;
    btn.textContent = 'Encrypting…';
    try {
      const sealed = await answer(pubFromHash, reqId, new TextEncoder().encode(text));
      const res = await fetch('/api/secret', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ct: b64u.encode(sealed.ct),
          iv: b64u.encode(sealed.iv),
          ttl: parseInt($('ansTtl').value, 10),
          views: 1,
          id: mailbox,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || `server returned ${res.status}`);
      $('ansSecret').value = '';
      only('answered');
      $('headline').textContent = 'Delivered';
      $('tagline').textContent = 'The requester’s browser is the only opener.';
    } catch (e) {
      errEl.textContent = e.message || 'Something went wrong.';
      errEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = 'Encrypt and answer';
    }
  });
}

// ---- create role ------------------------------------------------------------

function createView() {
  only('create');
  $('askCreate').addEventListener('click', async () => {
    const btn = $('askCreate');
    btn.disabled = true;
    btn.textContent = 'Generating keys…';
    try {
      const req = await newRequest();
      localStorage.setItem(storeKey(req.reqId),
        JSON.stringify({ pub: req.pub, priv: req.priv, created: Date.now() }));
      reqId = req.reqId;
      history.replaceState(null, '', `/a/${req.reqId}#${req.pub}`);
      await ownerView({ pub: req.pub, priv: req.priv });
    } catch (e) {
      btn.disabled = false;
      btn.textContent = 'Create ask link';
      dead('Could not create the ask: ' + (e.message || 'unknown error'));
    }
  });
}

// ---- role dispatch ----------------------------------------------------------

wireCopy('askSecretCopy', () => $('askSecret').textContent);

if (!reqId) {
  createView();
} else {
  let record = null;
  try { record = JSON.parse(localStorage.getItem(storeKey(reqId))); } catch { /* fall through */ }
  if (record && record.priv) {
    ownerView(record);
  } else if (pubFromHash) {
    responderView();
  } else {
    dead();
  }
}
