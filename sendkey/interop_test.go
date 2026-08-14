package sendkey

// Cross-implementation tests: the Go envelope (crypto.go) and the browser
// envelope (web/assets/crypto.js) must be byte-for-byte compatible, or a
// secret sent from the CLI cannot be opened in a browser and vice versa.
//
// These run only when Node is on PATH (it provides the same WebCrypto API
// browsers do); otherwise they skip, so `go test` stays dependency-free.

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const interopHarness = `
import { seal, openOuter, openInner, needsPassphrase, b64u }
  from '%CRYPTO_JS%';

const mode = process.argv[2];
if (mode === 'open') {
  const [,,, ct, iv, key, pass] = process.argv;
  const { flags, body } = await openOuter(b64u.decode(ct), b64u.decode(iv), b64u.decode(key));
  const out = needsPassphrase(flags) ? await openInner(body, pass ?? '') : body;
  process.stdout.write(new TextDecoder().decode(out));
} else if (mode === 'sealChunk') {
  const [,,, kf, index, plain] = process.argv;
  const { sealChunk } = await import('%CRYPTO_JS%');
  const { ct, iv } = await sealChunk(b64u.decode(kf), parseInt(index, 10),
    new TextEncoder().encode(plain));
  process.stdout.write(JSON.stringify({ ct: b64u.encode(ct), iv: b64u.encode(iv) }));
} else if (mode === 'openChunk') {
  const [,,, kf, index, ct, iv] = process.argv;
  const { openChunk } = await import('%CRYPTO_JS%');
  try {
    const plain = await openChunk(b64u.decode(kf), parseInt(index, 10),
      b64u.decode(ct), b64u.decode(iv));
    process.stdout.write('OK:' + new TextDecoder().decode(plain));
  } catch (e) {
    process.stdout.write('ERR:' + e.message);
  }
} else if (mode === 'flags') {
  const [,,, ct, iv, key] = process.argv;
  const { flags } = await openOuter(b64u.decode(ct), b64u.decode(iv), b64u.decode(key));
  process.stdout.write(String(flags));
} else {
  const [,,, secret, pass, extra] = process.argv;
  const { ct, iv, key } = await seal(
    new TextEncoder().encode(secret), pass ?? '', parseInt(extra ?? '0', 10));
  process.stdout.write(JSON.stringify({
    ct: b64u.encode(ct), iv: b64u.encode(iv), key: b64u.encode(key) }));
}
`

// setupNode writes the harness to a temp dir and returns a runner, or skips.
func setupNode(t *testing.T) func(args ...string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping Go<->browser interop test")
	}
	cryptoJS, err := filepath.Abs("public/assets/crypto.js")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "interop.mjs")
	body := strings.ReplaceAll(interopHarness, "%CRYPTO_JS%", "file://"+cryptoJS)
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return func(args ...string) string {
		cmd := exec.Command(node, append([]string{script}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("node %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}
}

func b64e(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func TestInteropGoSealsBrowserOpens(t *testing.T) {
	run := setupNode(t)
	secret := "go-sealed: postgres://user:p@ssw0rd@db.internal:5432/prod"

	sealed, err := Seal([]byte(secret), "")
	if err != nil {
		t.Fatal(err)
	}
	got := run("open", b64e(sealed.CT), b64e(sealed.IV), b64e(sealed.Key), "")
	if got != secret {
		t.Fatalf("browser could not open Go envelope:\n got %q\nwant %q", got, secret)
	}
}

func TestInteropGoSealsBrowserOpensWithPassphrase(t *testing.T) {
	run := setupNode(t)
	secret := "go-sealed-protected 🔐"
	const pass = "correct horse battery staple"

	sealed, err := Seal([]byte(secret), pass)
	if err != nil {
		t.Fatal(err)
	}
	got := run("open", b64e(sealed.CT), b64e(sealed.IV), b64e(sealed.Key), pass)
	if got != secret {
		t.Fatalf("browser could not open passphrase envelope:\n got %q\nwant %q", got, secret)
	}
}

func TestInteropBrowserSealsGoOpens(t *testing.T) {
	run := setupNode(t)
	secret := "browser-sealed: ghp_exampletoken"

	var out struct{ CT, IV, Key string }
	if err := json.Unmarshal([]byte(run("seal", secret, "")), &out); err != nil {
		t.Fatalf("bad harness output: %v", err)
	}
	ct, _ := base64.RawURLEncoding.DecodeString(out.CT)
	iv, _ := base64.RawURLEncoding.DecodeString(out.IV)
	key, _ := base64.RawURLEncoding.DecodeString(out.Key)

	got, err := Open(ct, iv, key, "")
	if err != nil {
		t.Fatalf("Go could not open browser envelope: %v", err)
	}
	if string(got) != secret {
		t.Fatalf("mismatch:\n got %q\nwant %q", got, secret)
	}
}

func TestInteropBrowserSealsGoOpensWithPassphrase(t *testing.T) {
	run := setupNode(t)
	secret := "browser-sealed-protected"
	const pass = "s3cr3t phrase"

	var out struct{ CT, IV, Key string }
	if err := json.Unmarshal([]byte(run("seal", secret, pass)), &out); err != nil {
		t.Fatalf("bad harness output: %v", err)
	}
	ct, _ := base64.RawURLEncoding.DecodeString(out.CT)
	iv, _ := base64.RawURLEncoding.DecodeString(out.IV)
	key, _ := base64.RawURLEncoding.DecodeString(out.Key)

	if _, err := Open(ct, iv, key, "wrong"); err != ErrBadPassphrase {
		t.Fatalf("wrong passphrase should give ErrBadPassphrase, got %v", err)
	}
	got, err := Open(ct, iv, key, pass)
	if err != nil {
		t.Fatalf("Go could not open browser passphrase envelope: %v", err)
	}
	if string(got) != secret {
		t.Fatalf("mismatch:\n got %q\nwant %q", got, secret)
	}
}

func TestInteropFileFlagGoToBrowser(t *testing.T) {
	run := setupNode(t)
	manifest := `{"name":"backup.tar.gz","size":1048576}`

	sealed, err := SealWithFlags([]byte(manifest), "", FlagFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := run("flags", b64e(sealed.CT), b64e(sealed.IV), b64e(sealed.Key)); got != "2" {
		t.Fatalf("browser sees flags %s, want 2 (FLAG_FILE)", got)
	}
	// the body still round-trips as bytes
	if got := run("open", b64e(sealed.CT), b64e(sealed.IV), b64e(sealed.Key), ""); got != manifest {
		t.Fatalf("manifest mismatch: %q", got)
	}
}

func TestInteropFileFlagBrowserToGo(t *testing.T) {
	run := setupNode(t)
	manifest := `{"name":"notes.pdf"}`

	var out struct{ CT, IV, Key string }
	if err := json.Unmarshal([]byte(run("seal", manifest, "", "2")), &out); err != nil {
		t.Fatalf("bad harness output: %v", err)
	}
	ct, _ := base64.RawURLEncoding.DecodeString(out.CT)
	iv, _ := base64.RawURLEncoding.DecodeString(out.IV)
	key, _ := base64.RawURLEncoding.DecodeString(out.Key)

	flags, err := EnvelopeFlags(ct, iv, key)
	if err != nil {
		t.Fatal(err)
	}
	if flags != FlagFile {
		t.Fatalf("Go sees flags %d, want %d", flags, FlagFile)
	}
}

func TestInteropFileFlagWithPassphrase(t *testing.T) {
	run := setupNode(t)
	manifest := `{"name":"kdbx"}`
	const pass = "belt and braces"

	sealed, err := SealWithFlags([]byte(manifest), pass, FlagFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := run("flags", b64e(sealed.CT), b64e(sealed.IV), b64e(sealed.Key)); got != "3" {
		t.Fatalf("flags %s, want 3 (passphrase|file)", got)
	}
	if got := run("open", b64e(sealed.CT), b64e(sealed.IV), b64e(sealed.Key), pass); got != manifest {
		t.Fatalf("manifest mismatch through passphrase layer: %q", got)
	}
}

func TestInteropChunkGoSealsBrowserOpens(t *testing.T) {
	run := setupNode(t)
	kf := make([]byte, KeyLen)
	for i := range kf {
		kf[i] = byte(i)
	}
	plain := "chunk-payload-7: not a full envelope, raw GCM"

	ct, iv, err := SealChunk(kf, 7, []byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if got := run("openChunk", b64e(kf), "7", b64e(ct), b64e(iv)); got != "OK:"+plain {
		t.Fatalf("browser open of Go chunk: %q", got)
	}
	// wrong index must fail AAD authentication
	if got := run("openChunk", b64e(kf), "8", b64e(ct), b64e(iv)); got != "ERR:bad-chunk" {
		t.Fatalf("wrong index should fail auth, got %q", got)
	}
}

func TestInteropChunkBrowserSealsGoOpens(t *testing.T) {
	run := setupNode(t)
	kf := make([]byte, KeyLen)
	kf[0] = 0xAB
	plain := "browser chunk 0"

	var out struct{ CT, IV string }
	if err := json.Unmarshal([]byte(run("sealChunk", b64e(kf), "0", plain)), &out); err != nil {
		t.Fatalf("bad harness output: %v", err)
	}
	ct, _ := base64.RawURLEncoding.DecodeString(out.CT)
	iv, _ := base64.RawURLEncoding.DecodeString(out.IV)

	got, err := OpenChunk(kf, 0, ct, iv)
	if err != nil || string(got) != plain {
		t.Fatalf("Go open of browser chunk: %q err %v", got, err)
	}
	if _, err := OpenChunk(kf, 1, ct, iv); err == nil {
		t.Fatal("wrong index must fail authentication")
	}
	other := make([]byte, KeyLen)
	other[0] = 0xCD
	if _, err := OpenChunk(other, 0, ct, iv); err == nil {
		t.Fatal("cross-file key must fail authentication")
	}
}

func TestManifestParseCompat(t *testing.T) {
	// Go-marshaled manifests must parse under the shared validation, and the
	// field names are pinned by the wire format comment in crypto.go.
	m := Manifest{
		V: 1, Name: "backup.tar.gz", Mime: "application/gzip", Size: 8 << 20,
		KF:     b64e(make([]byte, KeyLen)),
		Chunks: []string{b64e(make([]byte, 16))},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back Manifest
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("round-tripped manifest invalid: %v", err)
	}
	for _, k := range []string{`"v":`, `"name":`, `"mime":`, `"size":`, `"kf":`, `"chunks":`} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("manifest JSON missing pinned key %s: %s", k, raw)
		}
	}
}
