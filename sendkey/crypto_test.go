package sendkey

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	secret := []byte("AKIA-super-secret-api-key-🔑")
	sealed, err := Seal(secret, "")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed.CT, secret) {
		t.Fatal("plaintext leaked into ciphertext")
	}
	if len(sealed.Key) != KeyLen || len(sealed.IV) != IVLen {
		t.Fatalf("bad key/iv lengths: %d/%d", len(sealed.Key), len(sealed.IV))
	}

	got, err := Open(sealed.CT, sealed.IV, sealed.Key, "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("roundtrip mismatch: %q", got)
	}
}

func TestSealOpenWithPassphrase(t *testing.T) {
	secret := []byte("db password: hunter2")
	sealed, err := Seal(secret, "correct horse")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	needs, err := NeedsPassphrase(sealed.CT, sealed.IV, sealed.Key)
	if err != nil || !needs {
		t.Fatalf("NeedsPassphrase: %v, %v", needs, err)
	}

	// Wrong passphrase is retryable, not fatal, and burns nothing.
	if _, err := Open(sealed.CT, sealed.IV, sealed.Key, "wrong"); err != ErrBadPassphrase {
		t.Fatalf("want ErrBadPassphrase, got %v", err)
	}

	got, err := Open(sealed.CT, sealed.IV, sealed.Key, "correct horse")
	if err != nil {
		t.Fatalf("open with correct passphrase: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("roundtrip mismatch: %q", got)
	}
}

func TestNoPassphraseEnvelopeReportsFalse(t *testing.T) {
	sealed, _ := Seal([]byte("x"), "")
	needs, err := NeedsPassphrase(sealed.CT, sealed.IV, sealed.Key)
	if err != nil || needs {
		t.Fatalf("want needs=false, got %v, %v", needs, err)
	}
}

func TestWrongKeyFails(t *testing.T) {
	sealed, _ := Seal([]byte("secret"), "")
	badKey := make([]byte, KeyLen)
	copy(badKey, sealed.Key)
	badKey[0] ^= 0xFF
	if _, err := Open(sealed.CT, sealed.IV, badKey, ""); err == nil {
		t.Fatal("open with wrong key must fail")
	}
}

func TestTamperedCiphertextFails(t *testing.T) {
	sealed, _ := Seal([]byte("secret"), "")
	sealed.CT[len(sealed.CT)/2] ^= 0x01
	if _, err := Open(sealed.CT, sealed.IV, sealed.Key, ""); err == nil {
		t.Fatal("GCM must reject tampered ciphertext")
	}
}

func TestFreshRandomnessPerSeal(t *testing.T) {
	a, _ := Seal([]byte("same"), "")
	b, _ := Seal([]byte("same"), "")
	if bytes.Equal(a.Key, b.Key) || bytes.Equal(a.IV, b.IV) || bytes.Equal(a.CT, b.CT) {
		t.Fatal("two seals of the same secret must not share key/iv/ct")
	}
}

func TestBinarySecretsSurvive(t *testing.T) {
	secret := make([]byte, 256)
	for i := range secret {
		secret[i] = byte(i)
	}
	sealed, _ := Seal(secret, "p")
	got, err := Open(sealed.CT, sealed.IV, sealed.Key, "p")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("binary roundtrip failed: %v", err)
	}
}

func TestManifestValidateBounds(t *testing.T) {
	goodKF := base64.RawURLEncoding.EncodeToString(make([]byte, KeyLen))
	goodID := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	base := func() Manifest {
		return Manifest{V: 1, Name: "a.bin", Mime: "application/octet-stream",
			Size: 1024, KF: goodKF, Chunks: []string{goodID}}
	}
	if err := (func() error { m := base(); return m.Validate() })(); err != nil {
		t.Fatalf("baseline manifest should validate: %v", err)
	}

	cases := map[string]func(*Manifest){
		"wrong version":   func(m *Manifest) { m.V = 2 },
		"no chunks":       func(m *Manifest) { m.Chunks = nil },
		"too many chunks": func(m *Manifest) { m.Chunks = make([]string, MaxManifestChunks+1) },
		"empty name":      func(m *Manifest) { m.Name = "" },
		"huge name":       func(m *Manifest) { m.Name = strings.Repeat("x", MaxManifestName+1) },
		"zero size":       func(m *Manifest) { m.Size = 0 },
		"absurd size":     func(m *Manifest) { m.Size = MaxManifestSize + 1 },
		"bad kf":          func(m *Manifest) { m.KF = "short" },
		"bad chunk id":    func(m *Manifest) { m.Chunks = []string{"nope"} },
	}
	for name, mutate := range cases {
		m := base()
		mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: want validation error", name)
		}
	}
}
