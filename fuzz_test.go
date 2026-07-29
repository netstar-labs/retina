package retina

import "testing"

// FuzzDecode throws arbitrary bytes at Decode: it must never panic — malformed or
// truncated image data is an error, not a crash — and any image it does return must
// hash without panicking.
func FuzzDecode(f *testing.F) {
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("GIF89a"))
	f.Add([]byte{0xff, 0xd8, 0xff}) // JPEG SOI
	f.Add([]byte("not an image"))
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
