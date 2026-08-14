// SendKey envelope v1, browser implementation.
// Must stay byte-for-byte compatible with crypto.go:
//
//   outer ct = AES-256-GCM(K, outerIV, envelope)
//   envelope = version(0x01) || flags(1) || body
//   flags bit0 set: body = salt(16) || innerIV(12) || innerCT
//     innerKey = PBKDF2-HMAC-SHA256(passphrase, salt, 310000, 32 bytes)
//
// K is generated here, carried only in the URL fragment, and never sent
// anywhere: the server stores ciphertext it cannot read.

const VERSION = 0x01;
const FLAG_PASSPHRASE = 0x01;
const FLAG_FILE = 0x02; // body is a file manifest, not the secret itself
const SALT_LEN = 16;
const IV_LEN = 12;
const PBKDF2_ITERS = 310000;

export const b64u = {
  encode(bytes) {
    let s = '';
    for (const b of bytes) s += String.fromCharCode(b);
    return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  },
  decode(str) {
    const s = atob(str.replace(/-/g, '+').replace(/_/g, '/'));
    const out = new Uint8Array(s.length);
    for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
    return out;
  },
};

function randomBytes(n) {
  const b = new Uint8Array(n);
  crypto.getRandomValues(b);
  return b;
}

function concat(...parts) {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let off = 0;
  for (const p of parts) { out.set(p, off); off += p.length; }
  return out;
}

async function aesKey(rawBytes, usages) {
  return crypto.subtle.importKey('raw', rawBytes, 'AES-GCM', false, usages);
}

async function deriveInnerKey(passphrase, salt, usages) {
  const material = await crypto.subtle.importKey(
    'raw', new TextEncoder().encode(passphrase), 'PBKDF2', false, ['deriveKey']);
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt, iterations: PBKDF2_ITERS, hash: 'SHA-256' },
    material, { name: 'AES-GCM', length: 256 }, false, usages);
}

// seal encrypts secretBytes; returns { ct, iv, key } as Uint8Arrays.
// extraFlags adds envelope flags (FLAG_FILE for manifests); the passphrase
// bit is managed here and stripped from extraFlags.
export async function seal(secretBytes, passphrase, extraFlags = 0) {
  let flags = extraFlags & ~FLAG_PASSPHRASE;
  let body = secretBytes;

  if (passphrase) {
    flags |= FLAG_PASSPHRASE;
    const salt = randomBytes(SALT_LEN);
    const innerIV = randomBytes(IV_LEN);
    const innerKey = await deriveInnerKey(passphrase, salt, ['encrypt']);
    const innerCT = new Uint8Array(
      await crypto.subtle.encrypt({ name: 'AES-GCM', iv: innerIV }, innerKey, secretBytes));
    body = concat(salt, innerIV, innerCT);
  }

  const envelope = concat(new Uint8Array([VERSION, flags]), body);
  const keyBytes = randomBytes(32);
  const iv = randomBytes(IV_LEN);
  const key = await aesKey(keyBytes, ['encrypt']);
  const ct = new Uint8Array(
    await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, envelope));

  return { ct, iv, key: keyBytes };
}

// openOuter peels the outer layer only. Returns { flags, body } or throws
// Error('bad-key') when K is wrong / data corrupted.
export async function openOuter(ctBytes, ivBytes, keyBytes) {
  const key = await aesKey(keyBytes, ['decrypt']);
  let env;
  try {
    env = new Uint8Array(
      await crypto.subtle.decrypt({ name: 'AES-GCM', iv: ivBytes }, key, ctBytes));
  } catch {
    throw new Error('bad-key');
  }
  if (env.length < 2 || env[0] !== VERSION) throw new Error('bad-version');
  return { flags: env[1], body: env.subarray(2) };
}

export function needsPassphrase(flags) {
  return (flags & FLAG_PASSPHRASE) !== 0;
}

export function isFile(flags) {
  return (flags & FLAG_FILE) !== 0;
}

// openInner decrypts a passphrase-protected body. Throws
// Error('bad-passphrase') on a wrong passphrase. Retryable locally, the
// server is never contacted again.
export async function openInner(body, passphrase) {
  if (body.length < SALT_LEN + IV_LEN + 1) throw new Error('malformed');
  const salt = body.subarray(0, SALT_LEN);
  const innerIV = body.subarray(SALT_LEN, SALT_LEN + IV_LEN);
  const innerCT = body.subarray(SALT_LEN + IV_LEN);
  const innerKey = await deriveInnerKey(passphrase, salt, ['decrypt']);
  try {
    return new Uint8Array(
      await crypto.subtle.decrypt({ name: 'AES-GCM', iv: innerIV }, innerKey, innerCT));
  } catch {
    throw new Error('bad-passphrase');
  }
}
