# End to end suites

Playwright scripts that drive the built binary through a real browser:

- `browser.mjs`: the core text-secret flow, including the proof that the
  key fragment never appears in any request.
- `ask-e2e.mjs`: the full ask-link round trip across three browser contexts.
- `file-e2e.mjs`: encrypted file drop, including passphrase composition,
  burn semantics and a byte-for-byte comparison.

Run them against a local server:

```sh
go build -o /tmp/sendkey ./cmd/sendkey
/tmp/sendkey serve -addr 127.0.0.1:8404 &
npm i playwright && npx playwright install chromium   # once
BASE=http://127.0.0.1:8404 node e2e/browser.mjs
BASE=http://127.0.0.1:8404 node e2e/ask-e2e.mjs
BASE=http://127.0.0.1:8404 node e2e/file-e2e.mjs
```

CI runs the Go suite only (it has no browser); these are for release
checks. The scripts use Playwright's default browser resolution; if your Chromium
lives elsewhere, point CHROME_BIN at the binary:

```sh
CHROME_BIN=/usr/bin/chromium BASE=http://127.0.0.1:8404 node e2e/browser.mjs
```
