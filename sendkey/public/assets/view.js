// View page: peeks metadata without burning, then on an explicit click
// fetches (destroying the server copy) and decrypts entirely in this tab.
// File heads (envelope flag bit1) branch into the chunked download in
// files.js; text stays exactly as before.
import { openOuter, openInner, needsPassphrase, isFile, b64u } from './crypto.js';
import { parseManifest, downloadFile, human } from './files.js';

const $ = (id) => document.getElementById(id);
const id = location.pathname.replace(/^\/s\//, '');
const keyB64 = location.hash.slice(1);

let pendingBody = null;  // inner body kept local for passphrase retries
let pendingFlags = 0;    // envelope flags, needed to route after unlock
let savedURL = null;     // object URL kept alive for "save again"
let savedName = 'download.bin';

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

// stripFragment removes the key from the URL and history once the envelope
// is open: the secret is in hand and a lingering fragment is one more place
// it could leak from.
function stripFragment() {
  history.replaceState(null, '', location.pathname);
}

function render(bytes) {
  $('secret').textContent = new TextDecoder().decode(bytes);
  $('headline').textContent = 'Here is your secret';
  $('tagline').textContent = 'Save it now. This link no longer works.';
  hide('fileOut');
  only('out');
  stripFragment();
}

// renderFile validates the manifest and immediately fetches every chunk:
// the reveal click was the burn consent, and waiting only gives the chunks
// time to expire under the viewer.
async function renderFile(bytes) {
  let manifest;
  try {
    manifest = parseManifest(bytes);
  } catch (e) {
    return dead(e.message);
  }
  stripFragment();

  $('headline').textContent = 'An encrypted file';
  $('tagline').textContent = 'Decrypting it in this tab…';
  $('outMsg').textContent = 'Downloading and decrypting on this device.';
  $('dlName').textContent = manifest.name;
  $('dlSize').textContent = human(manifest.size);
  hide('secret'); hide('copy');
  show('fileOut');
  only('out');

  try {
    const blob = await downloadFile(manifest, {
      onProgress(frac) {
        $('dlBar').style.width = `${Math.round(frac * 100)}%`;
        $('dlPct').textContent = `${Math.round(frac * 100)}%`;
      },
    });
    savedURL = URL.createObjectURL(blob);
    savedName = manifest.name;
    $('dlBar').style.width = '100%';
    $('dlPct').textContent = '100%';
    $('outMsg').textContent = 'Decrypted on this device. This link is now dead.';
    $('headline').textContent = 'Your file is ready';
    $('tagline').textContent = 'Save it now. The server copy is gone.';
    show('saveBtn');
    saveFile(); // hand it over immediately; the button stays for retries
  } catch (e) {
    const msg = e.message === 'bad-chunk'
      ? 'A part of this file failed decryption. The data on the server was corrupted or tampered with.'
      : (e.message || 'Something went wrong while downloading this file.');
    dead(msg);
    $('headline').textContent = 'Download failed';
    $('tagline').textContent = 'The file could not be recovered.';
  }
}

function saveFile() {
  if (!savedURL) return;
  const a = document.createElement('a');
  a.href = savedURL;
  a.download = savedName;
  document.body.appendChild(a);
  a.click();
  a.remove();
}

// route sends an opened envelope body down the right path.
function route(flags, body) {
  if (isFile(flags)) return renderFile(body);
  return render(body);
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
      pendingFlags = flags;
      $('headline').textContent = 'One more step';
      $('tagline').textContent = 'This secret is passphrase-protected.';
      only('passgate');
      $('pass').focus();
      return;
    }
    await route(flags, body);
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
    const inner = await openInner(pendingBody, $('pass').value);
    await route(pendingFlags, inner);
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

$('saveBtn').addEventListener('click', saveFile);
window.addEventListener('unload', () => {
  if (savedURL) URL.revokeObjectURL(savedURL);
});

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
