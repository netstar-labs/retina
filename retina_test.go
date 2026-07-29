package retina

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"testing"
)

// BenchmarkHashes1080p measures the cost of all three hashes over a screenshot-sized
// image — every source pixel is read once per hash, through the [image.Image] interface.
func BenchmarkHashes1080p(b *testing.B) {
	img := texture(1920, 1080, 1)
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = PHash(img), DHash(img), AHash(img)
	}
}

// texture renders a deterministic multi-frequency pattern in normalized coordinates,
// so the same pattern at different pixel sizes samples the same continuous function —
// rich enough content that PHash is stable (unlike a plain gradient, whose DCT is
// almost all near-zero noise). freq scales the spatial frequencies to make a
// distinguishable second pattern.
func texture(w, h int, freq float64) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := float64(y) / float64(h)
		for x := 0; x < w; x++ {
			fx := float64(x) / float64(w)
			v := math.Sin(2*math.Pi*freq*3*fx) +
				math.Sin(2*math.Pi*freq*2*fy) +
				math.Sin(2*math.Pi*freq*4*(fx+fy))
			img.SetGray(x, y, color.Gray{Y: uint8((v + 3) / 6 * 255)})
		}
	}
	return img
}

func gradient(w, h int, vertical bool) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := x * 255 / w
			if vertical {
				v = y * 255 / h
			}
			img.SetGray(x, y, color.Gray{Y: uint8(v)})
		}
	}
	return img
}

func checker(w, h, cell int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := uint8(0)
			if (x/cell+y/cell)%2 == 0 {
				c = 255
			}
			img.SetGray(x, y, color.Gray{Y: c})
		}
	}
	return img
}

func solid(w, h int, v uint8) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = v
	}
	return img
}

func TestDeterministic(t *testing.T) {
	g := gradient(120, 120, false)
	for _, f := range []func(image.Image) uint64{PHash, DHash, AHash} {
		if first, second := f(g), f(g); first != second {
			t.Errorf("hash is not deterministic: %x vs %x", first, second)
		}
	}
}

func TestIdenticalZeroDistance(t *testing.T) {
	g := gradient(100, 80, false)
	if d := Distance(PHash(g), PHash(g)); d != 0 {
		t.Errorf("identical PHash distance = %d, want 0", d)
	}
	if Distance(0xdeadbeef, 0xdeadbeef) != 0 {
		t.Error("Distance of equal values must be 0")
	}
	if Distance(0, ^uint64(0)) != 64 {
		t.Error("Distance of 0 vs all-ones must be 64")
	}
}

func TestScaleRobustAndDiscriminates(t *testing.T) {
	big := texture(128, 128, 1)   // a structured pattern
	small := texture(64, 64, 1)   // the same pattern, half the resolution
	other := texture(128, 128, 2) // a different pattern (higher frequencies)

	scale := Distance(PHash(big), PHash(small)) // same image, different size
	diff := Distance(PHash(big), PHash(other))  // genuinely different image
	t.Logf("scale-distance=%d  discrimination-distance=%d", scale, diff)

	// The property that matters: the same pattern rescaled is closer than a different
	// pattern, and rescaling perturbs only a small fraction of the 63 bits.
	if diff <= scale {
		t.Errorf("PHash does not discriminate: different-image distance %d not greater than scale distance %d", diff, scale)
	}
	if scale > 16 {
		t.Errorf("PHash not scale-robust: distance %d between 128px and 64px of the same pattern is too high", scale)
	}
}

func TestUniformImageDHashAHashZero(t *testing.T) {
	// A uniform image has no structure, so DHash and AHash are exactly 0 (adjacent /
	// above-mean comparisons are all "not greater") for any brightness — brightness is
	// not structure. PHash is deliberately NOT asserted here: a mathematically-flat
	// image leaves only sub-epsilon floating-point residue in the DCT AC terms, so its
	// PHash is noise. Near-flat images are simply not meaningfully comparable by PHash;
	// real logos have structure. (DHash/AHash being 0 is the stable, testable fact.)
	for _, v := range []uint8{0, 128, 255} {
		s := solid(64, 64, v)
		if DHash(s) != 0 || AHash(s) != 0 {
			t.Errorf("uniform image (v=%d): DHash=%x AHash=%x, want 0/0", v, DHash(s), AHash(s))
		}
	}
}

// wordmark draws a solid ink rectangle on a fully transparent background, the way a real
// brand asset ships. The transparent pixels carry a non-black RGB under alpha 0, which is
// what exporters write and what a naive (premultiplied) read would throw away.
func wordmark(ink color.NRGBA, x1, y1 int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0})
		}
	}
	for y := 8; y < y1; y++ {
		for x := 8; x < x1; x++ {
			img.SetNRGBA(x, y, ink)
		}
	}
	return img
}

// overWhite composites src onto an opaque white background — what a browser shows, and
// what a screenshot of the page would capture.
func overWhite(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, b, src, b.Min, draw.Over)
	return dst
}

func TestTransparentBackgroundMatchesRendered(t *testing.T) {
	// The asset with its transparent background must hash exactly like the same asset as
	// rendered on a page. Without compositing, alpha reads as black: the two disagreed on
	// all 64 AHash bits.
	logo := wordmark(color.NRGBA{R: 20, G: 20, B: 20, A: 0xff}, 40, 24)
	rendered := overWhite(logo)
	for _, h := range []struct {
		name string
		fn   func(image.Image) uint64
	}{{"PHash", PHash}, {"DHash", DHash}, {"AHash", AHash}} {
		if a, b := h.fn(logo), h.fn(rendered); a != b {
			t.Errorf("%s of a transparent-background asset differs from the rendered one: %016x vs %016x (distance %d)",
				h.name, a, b, Distance(a, b))
		}
	}
	// Antialiased edges (partial alpha) round slightly differently through image/draw, so
	// allow a couple of bits there rather than demanding exactness.
	soft := wordmark(color.NRGBA{R: 20, G: 20, B: 20, A: 0x80}, 40, 24)
	if d := Distance(PHash(soft), PHash(overWhite(soft))); d > 2 {
		t.Errorf("PHash of a half-transparent asset differs from the rendered one by %d bits", d)
	}
}

func TestInkColourIsNotLostToAlpha(t *testing.T) {
	// Same silhouette, opposite ink. Read premultiplied, both marks sit on black and the
	// two hash identically — a false match between unrelated brand assets.
	dark := wordmark(color.NRGBA{R: 20, G: 20, B: 20, A: 0xff}, 40, 24)
	light := wordmark(cWhite, 40, 24)
	if PHash(dark) == PHash(light) && DHash(dark) == DHash(light) && AHash(dark) == AHash(light) {
		t.Error("a dark and a white mark of the same shape hash identically — alpha is being read as black")
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	// A hash taken from a PNG-encoded-then-Decoded image should match the in-memory one
	// (grayscale luma is stable across the PNG round-trip for a gray gradient).
	c := checker(96, 96, 12)
	var buf bytes.Buffer
	if err := png.Encode(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if d := Distance(PHash(c), PHash(got)); d > 2 {
		t.Errorf("PHash changed across PNG round-trip: distance %d", d)
	}
}

func TestDecodeRejectsBomb(t *testing.T) {
	// A tiny PNG that only declares enormous dimensions must be rejected at the config
	// stage, before any full-image allocation.
	bomb := pngHeader(100000, 100000)
	if _, err := Decode(bomb); err == nil {
		t.Fatal("Decode must reject an over-MaxPixels image")
	}
	// A sane size still decodes.
	small := gradient(32, 32, false)
	var buf bytes.Buffer
	png.Encode(&buf, small)
	if _, err := Decode(buf.Bytes()); err != nil {
		t.Fatalf("Decode rejected a valid small image: %v", err)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := Decode([]byte("not an image")); err == nil {
		t.Fatal("Decode must error on non-image bytes")
	}
}

// pngHeader builds a PNG signature + IHDR chunk (with a valid CRC) declaring w×h. That
// is all image.DecodeConfig reads, so it exercises the MaxPixels guard without a
// full image.
func pngHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8] = 8 // bit depth
	ihdr[9] = 2 // color type: truecolor
	var chunk bytes.Buffer
	binary.Write(&chunk, binary.BigEndian, uint32(len(ihdr)))
	chunk.WriteString("IHDR")
	chunk.Write(ihdr)
	crc := crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...))
	binary.Write(&chunk, binary.BigEndian, crc)
	b.Write(chunk.Bytes())
	return b.Bytes()
}
