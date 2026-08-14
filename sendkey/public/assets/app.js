// Compose widget: seals the secret locally, uploads only ciphertext, and
// builds the link whose #fragment carries the key. A dropped file takes the
// same road, chunked: see files.js.
import { seal, b64u } from './crypto.js';
import { uploadFile, human, SITE_MAX_FILE } from './files.js';

const $ = (id) => document.getElementById(id);
const secretEl = $('secret'), passEl = $('pass'), usePass = $('usePass');
const errEl = $('err'), goBtn = $('go');

let selectedFile = null;
let uploading = false;

// Dedicated pages: /send is the markup default; /drop promotes the file
// path. Same composer, same ids, one script.
if (location.pathname === '/drop') {
  const t = $('pageTitle'), g = $('pageTag'), dz = $('dropzone'), tw = $('secretWrap');
  if (t) t.textContent = 'Send a file';
  if (g) g.textContent = 'Up to 8 MB, sealed in this browser before upload.';
  if (dz && tw) {
    tw.parentNode.insertBefore(dz, tw);
    dz.classList.add('dz-primary');
    // primary position: nothing precedes it, so no leading "or"
    dz.querySelector('.dz-label').firstChild.textContent = 'drop a file here';
  }
  document.title = 'Send a file · SendKey';
}

secretEl.addEventListener('input', () => {
  $('count').textContent = new TextEncoder().encode(secretEl.value).length;
});

usePass.addEventListener('change', () => {
  passEl.classList.toggle('hidden', !usePass.checked);
  if (usePass.checked) passEl.focus();
});

function showError(msg) {
  errEl.textContent = msg;
  errEl.classList.remove('hidden');
}

// ---- file selection ---------------------------------------------------------
// A file replaces the textarea (one or the other, never both). Everything
// else about the composer stays the same.

function setFile(file) {
  if (!file) return;
  if (file.size > SITE_MAX_FILE) {
    showError(`Files are capped at ${human(SITE_MAX_FILE)} here. ` +
      'For bigger transfers, zip and split, or self host with a higher limit.');
    return;
  }
  if (file.size === 0) {
    showError('That file is empty.');
    return;
  }
  errEl.classList.add('hidden');
  selectedFile = file;
  $('fName').textContent = file.name;
  $('fSize').textContent = human(file.size);
  $('secretWrap').classList.add('hidden');
  $('dropzone').classList.add('hidden');
  $('fileCard').classList.remove('hidden');
}

function clearFile() {
  selectedFile = null;
  $('fileCard').classList.add('hidden');
  $('secretWrap').classList.remove('hidden');
  $('dropzone').classList.remove('hidden');
  $('upWrap').classList.add('hidden');
}

$('fileInput').addEventListener('change', () => {
  setFile($('fileInput').files[0]);
  $('fileInput').value = '';
});
$('pickBtn').addEventListener('click', () => $('fileInput').click());
$('fRemove').addEventListener('click', clearFile);

const dz = $('dropzone');
for (const ev of ['dragover', 'dragenter']) {
  dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.add('dz-hot'); });
}
for (const ev of ['dragleave', 'drop']) {
  dz.addEventListener(ev, (e) => { e.preventDefault(); dz.classList.remove('dz-hot'); });
}
dz.addEventListener('drop', (e) => setFile(e.dataTransfer.files[0]));

// an upload in flight is the one moment leaving the page loses work
window.addEventListener('beforeunload', (e) => {
  if (uploading) e.preventDefault();
});

function setProgress(frac) {
  $('upWrap').classList.remove('hidden');
  $('upBar').style.width = `${Math.round(frac * 100)}%`;
  $('upPct').textContent = `${Math.round(frac * 100)}%`;
}

// ---- create -----------------------------------------------------------------

async function create() {
  errEl.classList.add('hidden');
  const text = secretEl.value;
  if (!selectedFile && !text) {
    return showError('Nothing to send yet. Type a secret or drop a file.');
  }
  if (usePass.checked && !passEl.value) {
    return showError('Enter a passphrase, or uncheck the box.');
  }

  const ttl = parseInt($('ttl').value, 10);
  const views = parseInt($('views').value, 10);
  const passphrase = usePass.checked ? passEl.value : '';

  goBtn.disabled = true;
  goBtn.textContent = 'Encrypting…';
  uploading = true;
  try {
    let sealed;
    if (selectedFile) {
      goBtn.textContent = 'Encrypting chunks…';
      sealed = await uploadFile(selectedFile, {
        ttl, views, passphrase, onProgress: setProgress,
      });
    } else {
      sealed = await seal(new TextEncoder().encode(text), passphrase);
    }

    const res = await fetch('/api/secret', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ct: b64u.encode(sealed.ct),
        iv: b64u.encode(sealed.iv),
        ttl,
        views,
      }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `server returned ${res.status}`);

    $('link').value = `${location.origin}/s/${data.id}#${b64u.encode(sealed.key)}`;
    $('rViews').textContent = views === 1 ? 'once' : `${views} times`;

    // Drop the plaintext from the DOM as soon as it is no longer needed.
    secretEl.value = '';
    passEl.value = '';
    $('count').textContent = '0';
    clearFile();
    $('compose-form').classList.add('hidden');
    $('compose-result').classList.remove('hidden');
    $('link').select();
  } catch (e) {
    $('upWrap').classList.add('hidden');
    showError(e.message || 'Something went wrong.');
  } finally {
    uploading = false;
    goBtn.disabled = false;
    goBtn.textContent = 'Create secret link';
  }
}

goBtn.addEventListener('click', create);
secretEl.addEventListener('keydown', (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') create();
});

$('copy').addEventListener('click', async () => {
  const btn = $('copy');
  try {
    await navigator.clipboard.writeText($('link').value);
    btn.textContent = 'Copied ✓';
  } catch {
    $('link').select();
    btn.textContent = 'Press ⌘C';
  }
  setTimeout(() => { btn.textContent = 'Copy'; }, 1800);
});

// The CLI section's install one-liner. The prompt span stays out of the
// copied text, so what lands in the clipboard is directly runnable.
const cmdBtn = $('copyCmd');
if (cmdBtn) {
  cmdBtn.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText($('installCmd').textContent);
      cmdBtn.textContent = 'Copied ✓';
    } catch {
      cmdBtn.textContent = 'Press ⌘C';
    }
    setTimeout(() => { cmdBtn.textContent = 'Copy'; }, 1800);
  });
}

$('again').addEventListener('click', () => {
  $('link').value = '';
  $('compose-result').classList.add('hidden');
  $('compose-form').classList.remove('hidden');
  secretEl.focus();
});
