// Compose widget: seals the secret locally, uploads only ciphertext, and
// builds the link whose #fragment carries the key.
import { seal, b64u } from './crypto.js';

const $ = (id) => document.getElementById(id);
const secretEl = $('secret'), passEl = $('pass'), usePass = $('usePass');
const errEl = $('err'), goBtn = $('go');

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

async function create() {
  errEl.classList.add('hidden');
  const text = secretEl.value;
  if (!text) return showError('Nothing to send yet. Type or paste a secret first.');
  if (usePass.checked && !passEl.value) {
    return showError('Enter a passphrase, or uncheck the box.');
  }

  goBtn.disabled = true;
  goBtn.textContent = 'Encrypting…';
  try {
    const bytes = new TextEncoder().encode(text);
    const { ct, iv, key } = await seal(bytes, usePass.checked ? passEl.value : '');

    const res = await fetch('/api/secret', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ct: b64u.encode(ct),
        iv: b64u.encode(iv),
        ttl: parseInt($('ttl').value, 10),
        views: parseInt($('views').value, 10),
      }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `server returned ${res.status}`);

    $('link').value = `${location.origin}/s/${data.id}#${b64u.encode(key)}`;
    const n = parseInt($('views').value, 10);
    $('rViews').textContent = n === 1 ? 'once' : `${n} times`;

    // Drop the plaintext from the DOM as soon as it is no longer needed.
    secretEl.value = '';
    passEl.value = '';
    $('count').textContent = '0';
    $('compose-form').classList.add('hidden');
    $('compose-result').classList.remove('hidden');
    $('link').select();
  } catch (e) {
    showError(e.message || 'Something went wrong.');
  } finally {
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

$('again').addEventListener('click', () => {
  $('link').value = '';
  $('compose-result').classList.add('hidden');
  $('compose-form').classList.remove('hidden');
  secretEl.focus();
});
