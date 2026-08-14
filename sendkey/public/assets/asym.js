// SendKey ask envelope (v2): reverse secret sharing.
//
// The requester generates a P-256 keypair. The public key travels in the ask
// link's fragment; the private key never leaves the requester's browser. The
// responder makes an ephemeral P-256 keypair and derives
//
//   shared  = ECDH(ephemeralPriv, requesterPub)
//   aesKey  = HKDF-SHA256(ikm=shared, salt=requestId, info="sendkey/ask/v2")
//   payload = ephemeralPub(65) || AES-256-GCM(aesKey, iv, 0x02 || 0x00 || secret)
//
// and stores the payload at a mailbox id both sides derive from the request
// id alone: SHA-256("sendkey/ask/v1|" || requestId)[0..16]. The server sees
// two random-looking ids and one opaque blob; it can connect them, but it
// can read neither, and the mailbox burns on first read like any secret.

import { b64u } from './crypto.js';

// The ask envelope has its own version line, independent of the v1 secret
// envelope in crypto.js. Named distinctly so the two can never be read as
// the same number.
const ASK_VERSION = 0x02;
const EPH_PUB_LEN = 65; // uncompressed P-256 point
const IV_LEN = 12;

const EC = { name: 'ECDH', namedCurve: 'P-256' };

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

// newRequest creates the requester's keypair and request id. The private key
// is returned as a JWK for localStorage; it is marked non-extractable in use.
export async function newRequest() {
  const pair = await crypto.subtle.generateKey(EC, true, ['deriveBits']);
  const pub = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey));
  const priv = await crypto.subtle.exportKey('jwk', pair.privateKey);
  return { reqId: b64u.encode(randomBytes(16)), pub: b64u.encode(pub), priv };
}

// mailboxFor derives the storage id for an ask's answer. Pure function of
// the request id, so both sides find the same slot without coordinating.
export async function mailboxFor(reqId) {
  const material = concat(
    new TextEncoder().encode('sendkey/ask/v1|'), b64u.decode(reqId));
  const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', material));
  return b64u.encode(digest.subarray(0, 16));
}

async function deriveAesKey(privKey, pubKey, reqId, usages) {
  const shared = await crypto.subtle.deriveBits({ name: 'ECDH', public: pubKey }, privKey, 256);
  const hkdfKey = await crypto.subtle.importKey('raw', shared, 'HKDF', false, ['deriveKey']);
  return crypto.subtle.deriveKey(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: b64u.decode(reqId),
      info: new TextEncoder().encode('sendkey/ask/v2'),
    },
    hkdfKey, { name: 'AES-GCM', length: 256 }, false, usages);
}

// answer seals secretBytes to the requester's public key. Returns { ct, iv }
// in the same shape the normal composer uploads.
export async function answer(pubB64, reqId, secretBytes) {
  const requesterPub = await crypto.subtle.importKey(
    'raw', b64u.decode(pubB64), EC, false, []);
  const eph = await crypto.subtle.generateKey(EC, false, ['deriveBits']);
  const ephPub = new Uint8Array(await crypto.subtle.exportKey('raw', eph.publicKey));

  const key = await deriveAesKey(eph.privateKey, requesterPub, reqId, ['encrypt']);
  const iv = randomBytes(IV_LEN);
  const env = concat(new Uint8Array([ASK_VERSION, 0]), secretBytes);
  const sealed = new Uint8Array(
    await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, env));

  return { ct: concat(ephPub, sealed), iv };
}

// openAnswer decrypts a mailbox payload with the requester's private key.
// Throws Error('bad-key') when the payload does not match the keypair.
export async function openAnswer(privJwk, reqId, ctBytes, ivBytes) {
  if (ctBytes.length < EPH_PUB_LEN + 1) throw new Error('malformed');
  const ephPub = await crypto.subtle.importKey(
    'raw', ctBytes.subarray(0, EPH_PUB_LEN), EC, false, []);
  const priv = await crypto.subtle.importKey(
    'jwk', privJwk, EC, false, ['deriveBits']);
  const key = await deriveAesKey(priv, ephPub, reqId, ['decrypt']);

  let env;
  try {
    env = new Uint8Array(await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: ivBytes }, key, ctBytes.subarray(EPH_PUB_LEN)));
  } catch {
    throw new Error('bad-key');
  }
  if (env.length < 2 || env[0] !== ASK_VERSION) throw new Error('bad-version');
  return env.subarray(2);
}

// identicon returns an 8x8 grid of booleans derived from the public key, the
// left half mirrored onto the right. Both sides render it, so a requester
// and a responder can compare patterns out of band before trusting a link.
export async function identicon(pubB64) {
  const digest = new Uint8Array(
    await crypto.subtle.digest('SHA-256', b64u.decode(pubB64)));
  const grid = [];
  for (let r = 0; r < 8; r++) {
    const row = [];
    for (let c = 0; c < 4; c++) row.push((digest[r] >> c & 1) === 1);
    grid.push(row.concat(row.slice().reverse()));
  }
  return grid;
}
