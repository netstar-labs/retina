package retina

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math"
	"testing"
)

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
