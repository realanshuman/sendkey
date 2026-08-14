package sendkey

// PBKDF2-HMAC-SHA256 (RFC 8018 §5.2), implemented here rather than imported
// from crypto/pbkdf2 because that package only landed in Go 1.24. Keeping it
// local means the module builds on any reasonably recent toolchain — which
// matters for serverless builders that pin their own Go version — without
// taking a third-party dependency.
//
// Verified against the published PBKDF2-HMAC-SHA256 test vectors in
// pbkdf2_test.go.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// pbkdf2Key derives a dkLen-byte key from password and salt over iter rounds.
func pbkdf2Key(password, salt []byte, iter, dkLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hLen := prf.Size()

	numBlocks := (dkLen + hLen - 1) / hLen
	dk := make([]byte, 0, numBlocks*hLen)

	var (
		blockIdx [4]byte
		u        = make([]byte, 0, hLen)
		t        = make([]byte, hLen)
	)

	for block := 1; block <= numBlocks; block++ {
		// U_1 = PRF(password, salt || INT_BE32(block))
		binary.BigEndian.PutUint32(blockIdx[:], uint32(block))
		prf.Reset()
		prf.Write(salt)
		prf.Write(blockIdx[:])
		u = prf.Sum(u[:0])
		copy(t, u)

		// U_n = PRF(password, U_{n-1});  T_block = U_1 xor U_2 xor ... xor U_iter
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for i := range t {
				t[i] ^= u[i]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:dkLen]
}
