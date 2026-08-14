package sendkey

// Known-answer tests for the local PBKDF2-HMAC-SHA256. These vectors are the
// widely published PBKDF2-HMAC-SHA256 counterparts to the RFC 6070 SHA-1 set
// and appear in the same form across implementations, so they check the
// derivation against the specification rather than against another copy of the
// same code.

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestPBKDF2KnownAnswers(t *testing.T) {
	cases := []struct {
		password, salt string
		iter, dkLen    int
		want           string
	}{
		{"password", "salt", 1, 32,
			"120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{"password", "salt", 2, 32,
			"ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{"password", "salt", 4096, 32,
			"c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
		// Multi-block output (dkLen > hLen) exercises the block loop.
		{"passwordPASSWORDpassword", "saltSALTsaltSALTsaltSALTsaltSALTsalt", 4096, 40,
			"348c89dbcbd32b2f32d814b8116e84cf2b17347ebc1800181c4e2a1fb8dd53e1c635518c7dac47e9"},
		// Embedded NULs must be carried through unmodified.
		{"pass\x00word", "sa\x00lt", 4096, 16,
			"89b69d0516f829893c696226650a8687"},
	}

	for _, c := range cases {
		got := pbkdf2Key([]byte(c.password), []byte(c.salt), c.iter, c.dkLen)
		want, err := hex.DecodeString(c.want)
		if err != nil {
			t.Fatalf("bad test vector: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("pbkdf2(%q, %q, %d, %d)\n got %x\nwant %x",
				c.password, c.salt, c.iter, c.dkLen, got, want)
		}
	}
}

func TestPBKDF2OutputLengths(t *testing.T) {
	// Lengths either side of the 32-byte block boundary must all be exact.
	for _, n := range []int{1, 16, 31, 32, 33, 64, 100} {
		if got := len(pbkdf2Key([]byte("p"), []byte("s"), 2, n)); got != n {
			t.Errorf("dkLen %d: got %d bytes", n, got)
		}
	}
}

func TestPBKDF2DiffersOnInputChange(t *testing.T) {
	base := pbkdf2Key([]byte("password"), []byte("salt"), 100, 32)
	for name, got := range map[string][]byte{
		"different password": pbkdf2Key([]byte("Password"), []byte("salt"), 100, 32),
		"different salt":     pbkdf2Key([]byte("password"), []byte("Salt"), 100, 32),
		"different iters":    pbkdf2Key([]byte("password"), []byte("salt"), 101, 32),
	} {
		if bytes.Equal(base, got) {
			t.Errorf("%s produced an identical key", name)
		}
	}
}
