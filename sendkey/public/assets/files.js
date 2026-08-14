// File send pipeline: chunked client-side encryption over the ordinary
// secret API. A file becomes N chunk secrets (raw AES-GCM under one file
// key, chunk index bound as AAD) plus one head secret whose sealed body is
// the manifest. The server stores opaque blobs; it cannot tell a file from
// a long password beyond the size.
//
// Reliability model: chunks get extra view slack (+2, capped at 10) because
// a download is many consumes, not one, and a transient network failure on
// a views=1 chunk would destroy the file. Burn semantics genuinely live at
// the head: chunk ids exist only inside the sealed manifest, so slack views
// on chunks give an attacker nothing.
import { b64u, seal, sealChunk, openChunk, FLAG_FILE } from './crypto.js';

export const CHUNK_SIZE = 512 * 1024;
export const SITE_MAX_FILE = 8 * 1024 * 1024;
const MAX_MANIFEST_CHUNKS = 32;
const MAX_NAME = 255;
const CHUNK_VIEW_SLACK = 2;
const CHUNK_TTL_SLACK = 300; // chunks outlive the head by five minutes

function randomBytes(n) {
  const b = new Uint8Array(n);
  crypto.getRandomValues(b);
  return b;
}

// serverMaxBytes asks /healthz for the deployment's chunk ceiling, so a
// self-host with a small -max-bytes refuses files with the real number
// instead of a mysterious 413 on the first chunk.
export async function serverMaxBytes() {
  try {
    const res = await fetch('/healthz');
    const data = await res.json();
    if (Number.isInteger(data.maxBytes) && data.maxBytes > 0) return data.maxBytes;
  } catch { /* fall through to the optimistic default */ }
  return 640 * 1024;
}

async function postSecret(body, tries = 3) {
  for (let attempt = 0; ; attempt++) {
    const res = await fetch('/api/secret', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (res.ok) return (await res.json()).id;
    // 429/5xx are retryable: the bucket refills about every two seconds
    if ((res.status === 429 || res.status >= 500) && attempt < tries - 1) {
      await new Promise((ok) => setTimeout(ok, 1500 * (attempt + 1)));
      continue;
    }
    const data = await res.json().catch(() => ({}));
    const err = new Error(data.error || `server returned ${res.status}`);
    err.status = res.status;
    throw err;
  }
}

function sanitizeName(name) {
  const clean = String(name || '')
    .replace(/[/\\]/g, '_')
    .replace(/[\u0000-\u001f\u007f]/g, '')
    .trim();
  return clean.slice(0, MAX_NAME) || 'download.bin';
}

// uploadFile seals and uploads file, reporting progress(0..1). Returns
// { ct, iv, key } of the sealed manifest head, ready for the normal POST in
// app.js, plus the head body so the caller can reuse ttl/views. onProgress
// covers chunk uploads only; the head is the caller's existing flow.
export async function uploadFile(file, { ttl, views, passphrase, onProgress }) {
  if (file.size === 0) throw new Error('This file is empty.');
  if (file.size > SITE_MAX_FILE) {
    throw new Error(`Files are capped at ${human(SITE_MAX_FILE)} here.`);
  }
  const maxBytes = await serverMaxBytes();
  if (CHUNK_SIZE + 64 > maxBytes) {
    throw new Error(`This server accepts secrets up to ${human(maxBytes)}, ` +
      'which is too small for file chunks.');
  }

  const kf = randomBytes(32);
  const total = Math.ceil(file.size / CHUNK_SIZE);
  if (total > MAX_MANIFEST_CHUNKS) throw new Error('Too many chunks.'); // unreachable under the site cap
  const chunkViews = Math.min(10, views + CHUNK_VIEW_SLACK);
  const chunkTTL = ttl + CHUNK_TTL_SLACK;

  const ids = [];
  const uploaded = [];
  try {
    for (let i = 0; i < total; i++) {
      const plain = new Uint8Array(
        await file.slice(i * CHUNK_SIZE, (i + 1) * CHUNK_SIZE).arrayBuffer());
      const { ct, iv } = await sealChunk(kf, i, plain);
      const id = await postSecret({
        ct: b64u.encode(ct), iv: b64u.encode(iv),
        ttl: chunkTTL, views: chunkViews,
      });
      ids.push(id);
      uploaded.push(id);
      onProgress?.((i + 1) / total);
    }
  } catch (e) {
    // Best effort, capped: consuming an orphan burns it now instead of at
    // TTL. Failures here are irrelevant; TTL is the real cleanup.
    for (const id of uploaded.slice(0, 8)) {
      for (let v = 0; v < Math.min(chunkViews, 4); v++) {
        fetch(`/api/secret/${encodeURIComponent(id)}`).catch(() => {});
      }
    }
    throw e;
  }

  const manifest = {
    v: 1,
    name: sanitizeName(file.name),
    mime: String(file.type || 'application/octet-stream').slice(0, 128),
    size: file.size,
    kf: b64u.encode(kf),
    chunks: ids,
  };
  return seal(new TextEncoder().encode(JSON.stringify(manifest)),
    passphrase, FLAG_FILE);
}

// parseManifest validates a decrypted head body. Throws on anything a
// hostile sender could use to demand unbounded work from the viewer.
export function parseManifest(bodyBytes) {
  let m;
  try {
    m = JSON.parse(new TextDecoder().decode(bodyBytes));
  } catch {
    throw new Error('This file manifest is damaged.');
  }
  const idOK = (id) => typeof id === 'string' && /^[\w-]{22}$/.test(id);
  if (m.v !== 1
    || !Array.isArray(m.chunks) || m.chunks.length === 0
    || m.chunks.length > MAX_MANIFEST_CHUNKS || !m.chunks.every(idOK)
    || !Number.isInteger(m.size) || m.size <= 0 || m.size > 64 * 1024 * 1024
    || typeof m.kf !== 'string' || b64u.decode(m.kf).length !== 32) {
    throw new Error('This file manifest is not valid.');
  }
  m.name = sanitizeName(m.name);
  return m;
}

// downloadFile fetches and decrypts every chunk of a validated manifest.
// Returns a Blob (always application/octet-stream: the declared mime is
// display-only and must never make a blob: URL executable). Retries missing
// chunks only; already-fetched plaintext is kept.
export async function downloadFile(manifest, { onProgress }) {
  const kf = b64u.decode(manifest.kf);
  const total = manifest.chunks.length;

  // Cheap preflight: if the first and last chunks are gone, say so before
  // burning anything else. Meta peeks do not consume views.
  for (const idx of total > 1 ? [0, total - 1] : [0]) {
    const res = await fetch(
      `/api/secret/${encodeURIComponent(manifest.chunks[idx])}/meta`);
    if (!res.ok) {
      throw new Error('Parts of this file have already expired or been ' +
        'downloaded. It cannot be recovered.');
    }
  }

  const plain = new Array(total).fill(null);
  let done = 0;
  for (let attempt = 0; attempt < 3; attempt++) {
    for (let i = 0; i < total; i++) {
      if (plain[i]) continue;
      try {
        const res = await fetch(
          `/api/secret/${encodeURIComponent(manifest.chunks[i])}`);
        if (!res.ok) { plain[i] = null; continue; }
        const data = await res.json();
        plain[i] = await openChunk(
          kf, i, b64u.decode(data.ct), b64u.decode(data.iv));
        done++;
        onProgress?.(done / total);
      } catch (e) {
        if (e.message === 'bad-chunk') throw new Error('bad-chunk');
        // network hiccup: leave the slot empty for the retry pass
      }
    }
    if (done === total) break;
    if (attempt < 2) await new Promise((ok) => setTimeout(ok, 1200));
  }

  const missing = plain.filter((p) => !p).length;
  if (missing > 0) {
    throw new Error(`${missing} of ${total} parts could not be fetched. ` +
      'They may have expired or been consumed by another download.');
  }
  return new Blob(plain, { type: 'application/octet-stream' });
}

export function human(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10240 ? 1 : 0)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
