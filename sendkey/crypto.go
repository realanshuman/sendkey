package sendkey

// SendKey envelope format v1
// --------------------------
// This is the wire format shared byte-for-byte between the Go CLI and the
// browser (public/assets/crypto.js). The server never sees anything below the
// outer layer.
//
//	outer ciphertext = AES-256-GCM(K, outerIV, envelope)
//	  K       : 32 random bytes, carried ONLY in the URL fragment (#...)
//	  outerIV : 12 random bytes, stored with the ciphertext (not secret)
//
//	envelope = version(1) || flags(1) || body
//	  version : 0x01
//	  flags   : bit0 = passphrase-protected
//
//	body (flags&1 == 0):  secret bytes
//	body (flags&1 == 1):  salt(16) || innerIV(12) || innerCT
//	  innerKey = PBKDF2-HMAC-SHA256(passphrase, salt, 310000, 32)
//	  innerCT  = AES-256-GCM(innerKey, innerIV, secret bytes)
//
// The passphrase layer sits INSIDE the outer layer, so a recipient who typed
// the wrong passphrase can retry locally, forever, without touching the
// server — the one burn-on-read fetch already happened.
//
// File manifests (flags bit1, FlagFile)
// --------------------------------------
// A file is sent as N chunk secrets plus one head secret. The head is a
// normal v1 envelope whose body is a JSON manifest:
//
//	{"v":1,"name":...,"mime":...,"size":N,"kf":b64u(32),"chunks":[ids...]}
//
// Each chunk is raw AES-256-GCM under the manifest's file key kf — NOT an
// envelope — with AAD "sendkey/file/v1|" + decimal chunk index (ASCII), so
// chunks cannot be reordered, and a fresh kf per file means chunks cannot be
// spliced across files. The id list rides inside the GCM-authenticated
// manifest, so the server cannot substitute or truncate it. Integrity is
// complete without a separate whole-file hash.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

const (
	envVersion      = 0x01
	flagPassphrase  = 0x01
	flagFile        = 0x02 // body is a file manifest, not the secret itself
	saltLen         = 16
	IVLen           = 12      // outer/inner GCM nonce length
	KeyLen          = 32      // AES-256 key length
	pbkdf2Iters     = 310_000 // OWASP recommendation for PBKDF2-HMAC-SHA256
	maxEnvelopeSize = 1 << 20 // sanity bound when opening
)

// ErrBadPassphrase is returned when the inner layer fails to authenticate.
var ErrBadPassphrase = errors.New("wrong passphrase")

// Sealed is an envelope encrypted under a fresh random key.
type Sealed struct {
	CT  []byte // outer ciphertext for the server
	IV  []byte // outer nonce for the server
	Key []byte // 32-byte key for the URL fragment — never send this anywhere
}

// Seal encrypts secret into a v1 envelope. If passphrase is non-empty an
// inner PBKDF2-derived layer is added.
func Seal(secret []byte, passphrase string) (*Sealed, error) {
	return SealWithFlags(secret, passphrase, 0)
}

// SealWithFlags is Seal with extra envelope flags, used for file manifests
// (FlagFile). The passphrase bit is managed here and must not be passed in.
func SealWithFlags(secret []byte, passphrase string, extra byte) (*Sealed, error) {
	body := secret
	flags := extra &^ flagPassphrase

	if passphrase != "" {
		salt := make([]byte, saltLen)
		innerIV := make([]byte, IVLen)
		if err := fillRandom(salt, innerIV); err != nil {
			return nil, err
		}
		innerKey := pbkdf2Key([]byte(passphrase), salt, pbkdf2Iters, KeyLen)
		innerCT, err := gcmSeal(innerKey, innerIV, secret)
		if err != nil {
			return nil, err
		}
		flags |= flagPassphrase
		body = make([]byte, 0, saltLen+IVLen+len(innerCT))
		body = append(body, salt...)
		body = append(body, innerIV...)
		body = append(body, innerCT...)
	}

	envelope := make([]byte, 0, 2+len(body))
	envelope = append(envelope, envVersion, flags)
	envelope = append(envelope, body...)

	key := make([]byte, KeyLen)
	outerIV := make([]byte, IVLen)
	if err := fillRandom(key, outerIV); err != nil {
		return nil, err
	}
	ct, err := gcmSeal(key, outerIV, envelope)
	if err != nil {
		return nil, err
	}
	return &Sealed{CT: ct, IV: outerIV, Key: key}, nil
}

// FlagFile is the envelope flag marking the body as a file manifest.
const FlagFile = flagFile

// NeedsPassphrase opens only the outer layer and reports whether the envelope
// carries an inner passphrase layer.
func NeedsPassphrase(ct, iv, key []byte) (bool, error) {
	env, err := openOuter(ct, iv, key)
	if err != nil {
		return false, err
	}
	return env[1]&flagPassphrase != 0, nil
}

// EnvelopeFlags opens only the outer layer and returns the raw flags byte.
func EnvelopeFlags(ct, iv, key []byte) (byte, error) {
	env, err := openOuter(ct, iv, key)
	if err != nil {
		return 0, err
	}
	return env[1], nil
}

// Open decrypts a v1 envelope. passphrase may be empty when the envelope has
// no inner layer; a wrong passphrase yields ErrBadPassphrase (retryable —
// the outer ciphertext is already in hand, nothing else burns).
func Open(ct, iv, key []byte, passphrase string) ([]byte, error) {
	env, err := openOuter(ct, iv, key)
	if err != nil {
		return nil, err
	}
	flags, body := env[1], env[2:]

	if flags&flagPassphrase == 0 {
		return body, nil
	}
	if len(body) < saltLen+IVLen+1 {
		return nil, errors.New("malformed passphrase envelope")
	}
	salt, innerIV, innerCT := body[:saltLen], body[saltLen:saltLen+IVLen], body[saltLen+IVLen:]
	innerKey := pbkdf2Key([]byte(passphrase), salt, pbkdf2Iters, KeyLen)
	secret, err := gcmOpen(innerKey, innerIV, innerCT)
	if err != nil {
		return nil, ErrBadPassphrase
	}
	return secret, nil
}

func openOuter(ct, iv, key []byte) ([]byte, error) {
	if len(key) != KeyLen {
		return nil, fmt.Errorf("key must be %d bytes", KeyLen)
	}
	if len(ct) > maxEnvelopeSize {
		return nil, errors.New("envelope too large")
	}
	env, err := gcmOpen(key, iv, ct)
	if err != nil {
		return nil, errors.New("decryption failed: wrong key or corrupted data")
	}
	if len(env) < 2 || env[0] != envVersion {
		return nil, errors.New("unsupported envelope version")
	}
	return env, nil
}

func gcmSeal(key, iv, plaintext []byte) ([]byte, error) {
	g, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return g.Seal(nil, iv, plaintext, nil), nil
}

func gcmOpen(key, iv, ct []byte) ([]byte, error) {
	g, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return g.Open(nil, iv, ct, nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func fillRandom(bufs ...[]byte) error {
	for _, b := range bufs {
		if _, err := rand.Read(b); err != nil {
			return err
		}
	}
	return nil
}

// --- file chunks -------------------------------------------------------------

// chunkAAD pins the additional authenticated data for chunk index i. ASCII
// prefix plus the decimal index; both implementations must agree byte for
// byte.
func chunkAAD(index int) []byte {
	return []byte(fmt.Sprintf("sendkey/file/v1|%d", index))
}

// SealChunk encrypts one file chunk under the file key with its index bound
// as AAD. The result is raw GCM ciphertext, not an envelope.
func SealChunk(fileKey []byte, index int, plain []byte) (ct, iv []byte, err error) {
	if len(fileKey) != KeyLen {
		return nil, nil, fmt.Errorf("file key must be %d bytes", KeyLen)
	}
	iv = make([]byte, IVLen)
	if err := fillRandom(iv); err != nil {
		return nil, nil, err
	}
	g, err := newGCM(fileKey)
	if err != nil {
		return nil, nil, err
	}
	return g.Seal(nil, iv, plain, chunkAAD(index)), iv, nil
}

// OpenChunk decrypts one file chunk, authenticating its position.
func OpenChunk(fileKey []byte, index int, ct, iv []byte) ([]byte, error) {
	if len(fileKey) != KeyLen {
		return nil, fmt.Errorf("file key must be %d bytes", KeyLen)
	}
	g, err := newGCM(fileKey)
	if err != nil {
		return nil, err
	}
	plain, err := g.Open(nil, iv, ct, chunkAAD(index))
	if err != nil {
		return nil, errors.New("chunk failed authentication: wrong key, index, or corrupted data")
	}
	return plain, nil
}

// Manifest is the sealed head body of a file send (envelope flags FlagFile).
type Manifest struct {
	V      int      `json:"v"`
	Name   string   `json:"name"`
	Mime   string   `json:"mime"`
	Size   int64    `json:"size"`
	KF     string   `json:"kf"`     // b64u file key
	Chunks []string `json:"chunks"` // secret ids, in order
}

// Manifest bounds. A manifest is sender-authored: the viewer validates it
// before fetching anything, so a hostile one cannot demand unbounded work.
const (
	MaxManifestChunks = 32
	MaxManifestName   = 255
	MaxManifestSize   = 64 << 20
)

// ValidateManifest applies the shared bounds and shape checks.
func (m *Manifest) Validate() error {
	switch {
	case m.V != 1:
		return errors.New("unsupported manifest version")
	case len(m.Chunks) == 0 || len(m.Chunks) > MaxManifestChunks:
		return fmt.Errorf("chunk count must be 1..%d", MaxManifestChunks)
	case len(m.Name) == 0 || len(m.Name) > MaxManifestName:
		return errors.New("bad file name length")
	case m.Size <= 0 || m.Size > MaxManifestSize:
		return errors.New("implausible file size")
	}
	kf, err := base64.RawURLEncoding.DecodeString(m.KF)
	if err != nil || len(kf) != KeyLen {
		return errors.New("bad file key")
	}
	for _, id := range m.Chunks {
		raw, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil || len(raw) != 16 {
			return errors.New("bad chunk id")
		}
	}
	return nil
}
