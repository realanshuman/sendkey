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

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

const (
	envVersion      = 0x01
	flagPassphrase  = 0x01
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
	body := secret
	flags := byte(0)

	if passphrase != "" {
		salt := make([]byte, saltLen)
		innerIV := make([]byte, IVLen)
		if err := fillRandom(salt, innerIV); err != nil {
			return nil, err
		}
		innerKey, err := pbkdf2.Key(sha256.New, passphrase, salt, pbkdf2Iters, KeyLen)
		if err != nil {
			return nil, err
		}
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

// NeedsPassphrase opens only the outer layer and reports whether the envelope
// carries an inner passphrase layer.
func NeedsPassphrase(ct, iv, key []byte) (bool, error) {
	env, err := openOuter(ct, iv, key)
	if err != nil {
		return false, err
	}
	return env[1]&flagPassphrase != 0, nil
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
	innerKey, err := pbkdf2.Key(sha256.New, passphrase, salt, pbkdf2Iters, KeyLen)
	if err != nil {
		return nil, err
	}
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
