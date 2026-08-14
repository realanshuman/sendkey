# SendKey

**Send a password that deletes itself.**

SendKey turns a password, API key or connection string into a link you can paste
into Slack. The first person to open it sees the secret; the second sees a
tombstone. The server that stores it in between cannot read it — not because it
promises not to, but because it never receives the key.

```
$ sendkey send "AKIA-EXAMPLE-SECRET-KEY-42"
one-time link (burns after 1 view(s), expires in 24h0m0s):
https://sendkey.app/s/H6847TzgXETBsG0KKgXZlA#HLsZDskZ-NQFE5xoluVVJ47aqYzyLRITMo953b4GNyk

$ sendkey get 'https://sendkey.app/s/H6847TzgXETBsG0KKgXZlA#HLsZ…'
AKIA-EXAMPLE-SECRET-KEY-42

$ sendkey get 'https://sendkey.app/s/H6847TzgXETBsG0KKgXZlA#HLsZ…'
sendkey: server: this secret has expired or already been viewed
```

## Why the server can't read your secret

Look closely at that link. Everything after the `#` is the AES-256 key — and a
URL fragment is the one part of a URL that **browsers never put on the wire.**
It isn't in the request line, it isn't in `Referer`, and it isn't in the access
log. The sender encrypts locally, uploads ciphertext, and hands you the key
through a channel the server is structurally incapable of observing.

```
   sender                        server                      recipient
 ┌────────────────┐                                     ┌────────────────┐
 │ plaintext      │                                     │  plaintext     │
 │   │ AES-256-GCM│         ┌──────────────┐            │      ▲         │
 │   ▼            │  POST   │ ciphertext   │   GET      │      │ decrypt │
 │ ciphertext ────┼────────►│ (opaque)     ├───────────►│──────┘         │
 │ key ───────────┼─┐       │              │         ┌──┼─► key          │
 └────────────────┘ │       │  ✗ no key    │         │  └────────────────┘
                    │       │  ✗ burns on  │         │
                    │       │    first read│         │
                    │       └──────────────┘         │
                    └───── link #fragment ───────────┘
                          (never transmitted)
```

A breach of the database yields a pile of AES-256-GCM blobs and nothing else.
There is no key to steal, because the key was never there.

## Install

```sh
go install github.com/realanshuman/sendkey/cmd/sendkey@latest
```

One static binary, **zero third-party dependencies** — Go standard library only.
It is the server, the web app and the CLI.

## Use it

**Run the server** (serves the site and the API):

```sh
sendkey serve                          # listens on :8080
sendkey serve -addr :9000 -rate 60     # custom port and per-IP rate limit
```

**From the CLI** — pipe-friendly, so it drops into existing workflows:

```sh
sendkey send "hunter2"                       # secret as an argument
pass show prod/db | sendkey send             # or from stdin
sendkey send -ttl 1h -views 3 "shared token" # expiry and read count
sendkey send -passphrase "very sensitive"    # add a passphrase layer

sendkey get 'https://…/s/ID#KEY'             # fetch, decrypt, burn
sendkey get 'https://…/s/ID#KEY' > key.txt   # redirect anywhere
```

Point the client at a remote server with `-server` or `SENDKEY_SERVER`.

**From a browser** — open the server root. Same crypto, same envelope format,
implemented against the Web Crypto API. A link made in the CLI opens in the
browser and vice versa; this is enforced by tests, not by hope.

## Deploy to Vercel

```sh
vercel deploy
```

`vercel.json` builds `api/index.go` as a Go function, serves `public/` from the
CDN, and rewrites `/s/*` to the view page.

**Storage is not optional here.** Serverless invocations do not share memory, so
an in-process store would lose every secret the moment the instance rotated. Add
a Vercel KV (Upstash) integration, or set the variables by hand:

| Variable | Notes |
| --- | --- |
| `KV_REST_API_URL` | Set automatically by the Vercel KV integration |
| `KV_REST_API_TOKEN` | Set automatically by the Vercel KV integration |
| `UPSTASH_REDIS_REST_URL` | Alternative spelling, if you use Upstash directly |
| `UPSTASH_REDIS_REST_TOKEN` | Alternative spelling |
| `SENDKEY_RATE` | Creations per minute per IP (default 30) |
| `SENDKEY_MAX_BYTES` | Largest accepted ciphertext (default 131072) |

Without credentials the function returns a readable 503 explaining what is
missing, rather than failing opaquely.

The same auto-detection works for `sendkey serve`: set those variables and it
uses Redis, so several instances can sit behind one load balancer. Leave them
unset and it keeps secrets in memory.

## The passphrase layer

`-passphrase` nests a second, independently keyed envelope inside the first:

```
outer = AES-256-GCM(K_url,  version ‖ flags ‖ inner)
inner = AES-256-GCM(K_pass, secret)     K_pass = PBKDF2-SHA256(passphrase, salt, 310k)
```

Because the passphrase layer sits *inside* the burn boundary, a recipient who
fat-fingers it can retry **locally, indefinitely** — the single server fetch has
already happened and the ciphertext is in hand. A design that checked the
passphrase server-side would have to either burn the secret on a typo or hand
attackers an oracle to grind against. This one does neither.

`310,000` PBKDF2 iterations is the current OWASP figure for PBKDF2-HMAC-SHA256.

## What it does and doesn't protect against

**It protects you from:** the server operator, anyone who breaches the server's
storage, anyone who reads the link *after* the recipient did, and anyone who
finds the link in a log (the key isn't in the log).

**It does not protect you from:** whoever holds the link before it's opened.
The link *is* the secret — send it through a channel you trust, and prefer a
short `-ttl`. Nor does it defend against a malicious *server operator serving
you tampered JavaScript*; that is the standing limitation of all
encrypt-in-the-browser designs. The CLI is immune to that class of attack,
since the binary is what you built. Metadata (a secret's size, creation and
retrieval times) is visible to the operator.

## Design notes

**The burn is atomic.** In `MemStore`, consumption decrements the view counter
under the same mutex that reads the blob. On Redis the same guarantee comes from
a Lua script, because separate `GET` and `DEL` calls would leave a window where
two concurrent readers both observe the last view. Both backends are tested with
fifty simultaneous readers asserting exactly one winner.

**`/meta` peeks without burning.** The view page needs to say "a secret is
waiting" before you commit to destroying it, and link-preview crawlers must not
detonate secrets in a Slack channel. Metadata never touches the counter — only
an explicit click does.

**Reads don't extend the lifetime.** The Redis path rewrites the record with
`KEEPTTL`, so a multi-view secret cannot be kept alive indefinitely by reading it.

**Rate limiting sweeps on a timer, not on size.** A size-gated sweep looks
correct and is a landmine: past the threshold, a burst of fresh clients has
nothing old enough to evict, so every subsequent request pays a full O(n) scan.
Time-gating amortises it. Removing that pathology took the create+consume round
trip from **240µs to 37µs (6.6×)**. Buckets refilled to full are also dropped
under pressure — a full bucket is indistinguishable from one that never existed
— which bounds memory without letting a flood of new keys evict a drained bucket
and reset someone's limit.

**No inline styles or scripts.** The CSP is `default-src 'self'` with no
exceptions, so everything is a same-origin file. A blocked inline style fails
*silently* — the page renders unstyled rather than erroring — so a test asserts
the pages contain none.

## Configuration

| Flag | Default | Purpose |
| --- | --- | --- |
| `-addr` | `:8080` | Listen address (`SENDKEY_ADDR`) |
| `-max-items` | `100000` | Stored-secret ceiling, in-memory backend only |
| `-max-bytes` | `131072` | Largest accepted ciphertext |
| `-rate` | `30` | Creations per minute per IP |
| `-trust-proxy` | `false` | Honour `X-Forwarded-For` — **only** behind a proxy you control |

`-trust-proxy` defaults off deliberately: trusting a client-settable header
without a proxy in front of it lets anyone forge an IP and shrug off the limit.
The Vercel function enables it, because the platform always overwrites that
header itself.

## Tests

```sh
go test ./...           # unit, HTTP, crypto, redis, and Go↔browser interop
go test -race ./...     # the burn is a concurrency claim; prove it
go test -bench=. ./...
```

The suite covers burn atomicity under concurrency on **both** backends, expiry,
view countdown, TTL preservation, capacity reclaim, rate-limiter refill and
sweep behaviour, tamper rejection, security headers, path traversal, CSP
compliance, and — most importantly — **round-trips envelopes between the Go and
JavaScript implementations in both directions**, with and without a passphrase.
That interop test is what keeps the two implementations of the wire format from
silently drifting apart. It skips cleanly when Node isn't installed.

## Layout

```
.                 package sendkey — server, storage backends, crypto envelope
public/           the site: landing page, view page, assets (embedded + CDN)
cmd/sendkey/      the CLI: serve, send, get
api/index.go      Vercel function entry point
```

## HTTP API

| Method | Path | Behaviour |
| --- | --- | --- |
| `POST` | `/api/secret` | Store ciphertext `{ct, iv, ttl, views}` → `{id}` |
| `GET` | `/api/secret/{id}/meta` | Expiry and remaining views. **Does not burn.** |
| `GET` | `/api/secret/{id}` | Return ciphertext and decrement. **Burns.** |
| `GET` | `/healthz` | Liveness |

`ct` and `iv` are base64url (unpadded). The server validates lengths and
encoding, and stores bytes it cannot interpret.

## License

MIT
