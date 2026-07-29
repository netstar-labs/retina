package retina

import "testing"

// FuzzDecode throws arbitrary bytes at Decode: it must never panic — malformed or
// truncated image data is an error, not a crash — and any image it does return must
// hash without panicking.
func FuzzDecode(f *testing.F) {
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("GIF89a"))
	f.Add([]byte{0xff, 0xd8, 0xff})       // JPEG SOI
	f.Add([]byte{0x00, 0x00, 0x01, 0x00}) // ICONDIR, no entries
	f.Add([]byte("not an image"))
	// A structurally valid single-entry icon, so the fuzzer mutates a real DIB rather than
	// bouncing off the header checks.
	f.Add(icoWrap(icoEntry{w: 2, h: 2, bpp: 32, payload: append(
		[]byte{40, 0, 0, 0, 2, 0, 0, 0, 4, 0, 0, 0, 1, 0, 32, 0},
		make([]byte, 24+2*2*4+2*4)...)}))
	f.Fuzz(func(t *testing.T, data []byte) {
		img, err := Decode(data)
		if err != nil {
			return
		}
		_ = PHash(img)
		_ = DHash(img)
		_ = AHash(img)
	})
}

// FuzzFavicon feeds arbitrary bytes to the favicon hash: it must never panic and must
// be deterministic.
func FuzzFavicon(f *testing.F) {
	f.Add([]byte("abc"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if first, second := Favicon(data), Favicon(data); first != second {
			t.Errorf("Favicon is not deterministic: %d vs %d", first, second)
		}
	})
}
