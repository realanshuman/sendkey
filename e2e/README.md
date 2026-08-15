# End to end suites

Playwright scripts that drive the built binary through a real browser. They
exist because the Go suite cannot reach the parts of this product that only
happen in a browser: the key never leaving the fragment, a link dying for its
second visitor, a file surviving a chunked round trip.

- `browser.mjs`: the core text-secret flow, including the proof that the
  key fragment never appears in any request.
- `ask-e2e.mjs`: the full ask-link round trip across three browser contexts,
  including the refusal of a second answer.
- `file-e2e.mjs`: encrypted file drop, including passphrase composition,
  burn semantics and a byte-for-byte comparison.
- `pages-e2e.mjs`: the dedicated `/send` and `/drop` pages, including the
  file-first arrangement and round trips from each.
- `receipt-e2e.mjs`: live burn receipts, proving the sender's tab learns the
  open time across browser contexts without a reload.
- `api-e2e.mjs`: the API reference page, the API link in every footer, and
  the CORS contract: a page on a foreign origin drives the whole
  store-and-burn cycle with plain fetch.

## Running them

```sh
go build -o /tmp/sendkey ./cmd/sendkey
/tmp/sendkey serve -addr 127.0.0.1:8404 &

cd e2e
npm install                            # once
npx playwright install chromium        # once

BASE=http://127.0.0.1:8404 npm test    # every suite
BASE=http://127.0.0.1:8404 node receipt-e2e.mjs   # or just one
```

Each script exits non-zero on the first failed check, so `npm test` stops at
the first broken suite.

If your Chromium lives outside Playwright's own download (a distro package, a
container image), point `CHROME_BIN` at the binary:

```sh
CHROME_BIN=/usr/bin/chromium BASE=http://127.0.0.1:8404 npm test
```

## CI

`.github/workflows/ci.yml` runs all six in the `browser` job, against a
server built and started in the same step, on every push. The `test` job runs
the Go suite (including the Go-to-browser envelope interop tests, which use
Node's WebCrypto rather than a browser).
