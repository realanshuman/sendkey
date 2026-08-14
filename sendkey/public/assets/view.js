// View page: peeks metadata without burning, then on an explicit click fetches
// (destroying the server copy) and decrypts entirely in this tab.
import { openOuter, openInner, needsPassphrase, b64u } from './crypto.js';

const $ = (id) => document.getElementById(id);
const id = location.pathname.replace(/^\/s\//, '');
const keyB64 = location.hash.slice(1);

let pendingBody = null; // inner body kept local for passphrase retries

const show = (s) => $(s).classList.remove('hidden');
const hide = (s) => $(s).classList.add('hidden');

function only(section) {
  for (const s of ['loading', 'gate', 'passgate', 'out', 'gone']) {
    s === section ? show(s) : hide(s);
  }
}

function dead(msg) {
  if (msg) $('goneMsg').innerHTML = msg;
  $('headline').textContent = 'Nothing here';
  $('tagline').textContent = 'This link has already done its job.';
  only('gone');
}

function render(bytes) {
  $('secret').textContent = new TextDecoder().decode(bytes);
  $('headline').textContent = 'Here is your secret';
  $('tagline').textContent = 'Save it now. This link no longer works.';
  only('out');
  // Strip the key from the URL and history: the secret is already open, and a
  // lingering fragment is one more place it could leak from.
  history.replaceState(null, '', location.pathname);
}

async function init() {
  if (!keyB64) {
    return dead('<strong>This link is missing its decryption key.</strong> ' +
      'It was probably cut short. Copy the whole link, including the part after the #.');
  }
  try {
    const res = await fetch(`/api/secret/${encodeURIComponent(id)}/meta`);
    if (!res.ok) return dead();
    const meta = await res.json();
    const left = meta.views === 1
      ? 'This is the last view.'
      : `${meta.views} views remain.`;
    $('meta').textContent = `${left} Expires ${new Date(meta.expiresAt).toLocaleString()}.`;
    only('gate');
  } catch {
    dead('Could not reach the server. Check your connection and reload.');
  }
}

$('reveal').addEventListener('click', async () => {
  const btn = $('reveal');
  btn.disabled = true;
  btn.textContent = 'Decrypting…';
  try {
    // This request is the burn: the server destroys its copy right here.
    const res = await fetch(`/api/secret/${encodeURIComponent(id)}`);
    if (!res.ok) return dead();
    const data = await res.json();

    const { flags, body } = await openOuter(
      b64u.decode(data.ct), b64u.decode(data.iv), b64u.decode(keyB64));

    if (needsPassphrase(flags)) {
      pendingBody = body;
      $('headline').textContent = 'One more step';
      $('tagline').textContent = 'This secret is passphrase-protected.';
      only('passgate');
      $('pass').focus();
      return;
    }
    render(body);
  } catch (e) {
    dead(e.message === 'bad-key'
      ? '<strong>The key in this link is wrong.</strong> The secret could not be decrypted.'
      : 'Something went wrong while decrypting this secret.');
  } finally {
    btn.disabled = false;
    btn.textContent = 'Reveal secret';
  }
});

async function unlock() {
  const errEl = $('passerr');
  errEl.classList.add('hidden');
  try {
    render(await openInner(pendingBody, $('pass').value));
  } catch (e) {
    errEl.textContent = e.message === 'bad-passphrase'
      ? 'Wrong passphrase. Try again. Nothing is lost, the check runs on this device.'
      : 'This secret appears to be damaged.';
    errEl.classList.remove('hidden');
    $('pass').select();
  }
}

$('unlock').addEventListener('click', unlock);
$('pass').addEventListener('keydown', (e) => { if (e.key === 'Enter') unlock(); });

$('copy').addEventListener('click', async () => {
  const btn = $('copy');
  try {
    await navigator.clipboard.writeText($('secret').textContent);
    btn.textContent = 'Copied ✓';
  } catch {
    btn.textContent = 'Select it and press ⌘C';
  }
  setTimeout(() => { btn.textContent = 'Copy'; }, 1800);
});

init();
