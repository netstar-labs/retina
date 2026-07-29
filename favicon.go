package retina

import (
	"bytes"
	"encoding/base64"
	"math/bits"
)

// Favicon computes the Shodan-compatible favicon fingerprint of raw icon bytes: the
// MurmurHash3 x86 32-bit hash (seed 0) of the icon's standard base64 encoding, wrapped
// at 76 columns with a trailing newline — exactly Python's base64.encodebytes, which is
// what Shodan hashes. The result equals Shodan's `http.favicon.hash` (a signed 32-bit
// int), so a hit can be pivoted directly into a Shodan/Censys search to find every host
// serving the same icon. It is an exact-content fingerprint, not a perceptual hash: a
// re-encoded icon changes the hash (use [PHash] on the decoded image for near-match).
func Favicon(icon []byte) int32 {
	return int32(murmur3x86_32(base64Chunked(icon), 0))
}

// base64Chunked reproduces Python's base64.encodebytes: standard base64 split into
// 76-character lines, each terminated by '\n'. Empty input yields empty output (no
// trailing newline), matching encodebytes(b"").
func base64Chunked(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	enc := base64.StdEncoding.EncodeToString(data)
	var buf bytes.Buffer
	buf.Grow(len(enc) + len(enc)/76 + 1)
	for i := 0; i < len(enc); i += 76 {
		end := i + 76
		if end > len(enc) {
			end = len(enc)
		}
		buf.WriteString(enc[i:end])
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// murmur3x86_32 is the canonical MurmurHash3 x86 32-bit hash.
func murmur3x86_32(data []byte, seed uint32) uint32 {
	const (
		c1 = 0xcc9e2d51
		c2 = 0x1b873593
	)
	h1 := seed
	n := len(data)
	nblocks := n / 4
	for i := 0; i < nblocks; i++ {
		k1 := uint32(data[i*4]) | uint32(data[i*4+1])<<8 | uint32(data[i*4+2])<<16 | uint32(data[i*4+3])<<24
		k1 *= c1
		k1 = bits.RotateLeft32(k1, 15)
		k1 *= c2
		h1 ^= k1
		h1 = bits.RotateLeft32(h1, 13)
		h1 = h1*5 + 0xe6546b64
	}
	var k1 uint32
	tail := data[nblocks*4:]
	switch len(tail) {
	case 3:
		k1 ^= uint32(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(tail[0])
		k1 *= c1
		k1 = bits.RotateLeft32(k1, 15)
		k1 *= c2
		h1 ^= k1
	}
	h1 ^= uint32(n)
	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16
	return h1
}
