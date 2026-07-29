package retina

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// patternNRGBA paints diagonal colour bands — structure coarse enough to survive the 8×8
// downscale, so a hash of it is a real hash and not the degenerate hash of something that
// averages flat. (A high-frequency pattern would average away and hash to all-zero.)
func patternNRGBA(w, h int, colors ...color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, colors[(x*4/w+y*3/h)%len(colors)])
		}
	}
	return img
}

var (
	cBlack = color.NRGBA{A: 0xff}
	cWhite = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	cRed   = color.NRGBA{R: 0xd0, G: 0x20, B: 0x30, A: 0xff}
	cBlue  = color.NRGBA{R: 0x20, G: 0x40, B: 0xc0, A: 0xff}
)

// icoWrap assembles an .ico directory around already-encoded payloads.
func icoWrap(entries ...icoEntry) []byte {
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(0))            // reserved
	binary.Write(&out, binary.LittleEndian, uint16(1))            // type: icon
	binary.Write(&out, binary.LittleEndian, uint16(len(entries))) // count
	off := icoDirLen + icoEntryLen*len(entries)
	for _, e := range entries {
		out.Write([]byte{byte(e.w % 256), byte(e.h % 256), 0, 0}) // 256 encodes as 0
		binary.Write(&out, binary.LittleEndian, uint16(1))
		binary.Write(&out, binary.LittleEndian, uint16(e.bpp))
		binary.Write(&out, binary.LittleEndian, uint32(len(e.payload)))
		binary.Write(&out, binary.LittleEndian, uint32(off))
		off += len(e.payload)
	}
	for _, e := range entries {
		out.Write(e.payload)
	}
	return out.Bytes()
}

// pngEntry encodes img as an ICO entry carrying an embedded PNG (the modern form).
func pngEntry(t *testing.T, img image.Image) icoEntry {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	return icoEntry{w: b.Dx(), h: b.Dy(), bpp: 32, payload: buf.Bytes()}
}

// dibEntry encodes img as an ICO entry carrying a classic DIB at the given colour depth,
// bottom-up, with a 1-bpp AND mask when withMask is set (transparent where alpha is 0).
func dibEntry(t *testing.T, img *image.NRGBA, bpp int, withMask bool) icoEntry {
	t.Helper()
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var pal []color.NRGBA
	if bpp <= 8 {
		pal = distinctColors(img)
		if len(pal) > 1<<bpp {
			t.Fatalf("pattern has %d colours, too many for %d bpp", len(pal), bpp)
		}
	}
	var p bytes.Buffer
	declaredH := h
	if withMask {
		declaredH = 2 * h
	}
	binary.Write(&p, binary.LittleEndian, uint32(dibHeadLen))
	binary.Write(&p, binary.LittleEndian, int32(w))
	binary.Write(&p, binary.LittleEndian, int32(declaredH))
	binary.Write(&p, binary.LittleEndian, uint16(1))   // planes
	binary.Write(&p, binary.LittleEndian, uint16(bpp)) // bit count
	binary.Write(&p, binary.LittleEndian, uint32(0))   // BI_RGB
	binary.Write(&p, binary.LittleEndian, uint32(0))   // image size
	binary.Write(&p, binary.LittleEndian, int32(0))    // x px/m
	binary.Write(&p, binary.LittleEndian, int32(0))    // y px/m
	binary.Write(&p, binary.LittleEndian, uint32(len(pal)))
	binary.Write(&p, binary.LittleEndian, uint32(0)) // colours important
	for _, c := range pal {
		p.Write([]byte{c.B, c.G, c.R, 0})
	}
	stride := ((w*bpp + 31) / 32) * 4
	for y := h - 1; y >= 0; y-- { // bottom-up
		row := make([]byte, stride)
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			switch bpp {
			case 32:
				copy(row[x*4:], []byte{c.B, c.G, c.R, c.A})
			case 24:
				copy(row[x*3:], []byte{c.B, c.G, c.R})
			case 8:
				row[x] = byte(colorIndex(pal, c))
			case 4:
				if i := byte(colorIndex(pal, c)); x%2 == 0 {
					row[x/2] |= i << 4
				} else {
					row[x/2] |= i & 0x0f
				}
			case 1:
				if colorIndex(pal, c) == 1 {
					row[x/8] |= 1 << (7 - uint(x)%8)
				}
			}
		}
		p.Write(row)
	}
	if withMask {
		mstride := ((w + 31) / 32) * 4
		for y := h - 1; y >= 0; y-- {
			row := make([]byte, mstride)
			for x := 0; x < w; x++ {
				if img.NRGBAAt(x, y).A == 0 {
					row[x/8] |= 1 << (7 - uint(x)%8)
				}
			}
			p.Write(row)
		}
	}
	return icoEntry{w: w, h: h, bpp: bpp, payload: p.Bytes()}
}

// distinctColors lists the RGB triples used by img, in first-seen order — the palette a
// low-depth DIB would carry. Alpha is ignored: the AND mask, not the palette, carries
// transparency.
func distinctColors(img *image.NRGBA) []color.NRGBA {
	var pal []color.NRGBA
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			c.A = 0xff
			if colorIndex(pal, c) < 0 {
				pal = append(pal, c)
			}
		}
	}
	return pal
}

// degenerate reports whether a hash carries no information — every comparison the same
// way, which is what a flat (structureless) image produces.
func degenerate(h uint64) bool { return h == 0 || h == ^uint64(0) }

func colorIndex(pal []color.NRGBA, c color.NRGBA) int {
	for i, p := range pal {
		if p.R == c.R && p.G == c.G && p.B == c.B {
			return i
		}
	}
	return -1
}

// sameHashes asserts that two images fingerprint identically under all three hashes, and
// that the reference is not degenerate — an all-zero or all-ones hash means the image
// carried no structure, and every equality below would hold vacuously.
func sameHashes(t *testing.T, what string, want, got image.Image) {
	t.Helper()
	p, d, a := PHash(want), DHash(want), AHash(want)
	if degenerate(p) || degenerate(d) || degenerate(a) {
		t.Fatalf("%s: reference image is too flat to test (p=%x d=%x a=%x)", what, p, d, a)
	}
	for _, h := range []struct {
		name string
		fn   func(image.Image) uint64
	}{{"PHash", PHash}, {"DHash", DHash}, {"AHash", AHash}} {
		if w, g := h.fn(want), h.fn(got); w != g {
			t.Errorf("%s: %s mismatch: want %016x, got %016x (distance %d)", what, h.name, w, g, Distance(w, g))
		}
	}
}

func TestICOEmbeddedPNG(t *testing.T) {
	src := patternNRGBA(32, 32, cBlack, cWhite, cRed, cBlue)
	img, err := Decode(icoWrap(pngEntry(t, src)))
	if err != nil {
		t.Fatalf("Decode(.ico with PNG payload): %v", err)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("bounds = %v, want 32x32", b)
	}
	sameHashes(t, "ico/png", src, img)
}

func TestICODIBDepths(t *testing.T) {
	// One pattern per depth: 24/32 bpp carry colour directly, the paletted depths need a
	// palette that fits (2 colours at 1 bpp).
	for _, c := range []struct {
		bpp    int
		colors []color.NRGBA
	}{
		{32, []color.NRGBA{cBlack, cWhite, cRed, cBlue}},
		{24, []color.NRGBA{cBlack, cWhite, cRed, cBlue}},
		{8, []color.NRGBA{cBlack, cWhite, cRed, cBlue}},
		{4, []color.NRGBA{cBlack, cWhite, cRed}},
		{1, []color.NRGBA{cBlack, cWhite}},
	} {
		src := patternNRGBA(32, 32, c.colors...)
		img, err := Decode(icoWrap(dibEntry(t, src, c.bpp, true)))
		if err != nil {
			t.Errorf("%d bpp: Decode: %v", c.bpp, err)
			continue
		}
		if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
			t.Errorf("%d bpp: bounds = %v, want 32x32 (mask-doubled height not halved?)", c.bpp, b)
			continue
		}
		sameHashes(t, "ico/dib", src, img)
	}
}

func TestICOMaskTransparency(t *testing.T) {
	// Half the mark is transparent. Both the source and the decoded icon must composite
	// those pixels over white, so the two agree — and the result must differ from the
	// same icon decoded with everything opaque.
	src := patternNRGBA(32, 32, cBlack, cWhite, cRed, cBlue)
	holed := patternNRGBA(32, 32, cBlack, cWhite, cRed, cBlue)
	for y := 0; y < 32; y++ {
		for x := 16; x < 32; x++ {
			c := holed.NRGBAAt(x, y)
			c.A = 0
			holed.SetNRGBA(x, y, c)
		}
	}
	img, err := Decode(icoWrap(dibEntry(t, holed, 8, true)))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sameHashes(t, "ico/mask", holed, img)
	if PHash(img) == PHash(src) {
		t.Error("masked and unmasked icons hash identically — the AND mask was ignored")
	}
}

func TestICOPicksLargestEntry(t *testing.T) {
	small := patternNRGBA(16, 16, cBlack, cWhite)
	large := patternNRGBA(64, 64, cBlack, cWhite, cRed, cBlue)
	img, err := Decode(icoWrap(dibEntry(t, small, 32, true), pngEntry(t, large)))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 64 {
		t.Fatalf("bounds = %v, want the 64x64 entry", b)
	}
	sameHashes(t, "ico/largest", large, img)
}

func TestICOTopDownDIB(t *testing.T) {
	// A negative declared height means the rows are stored top-down. Build one by hand by
	// reversing the bottom-up payload's row order and flipping the height sign.
	src := patternNRGBA(16, 16, cBlack, cWhite, cRed, cBlue)
	e := dibEntry(t, src, 32, false)
	p := e.payload
	negH := int32(-16)
	binary.LittleEndian.PutUint32(p[8:12], uint32(negH))
	rows := p[dibHeadLen:]
	stride := 16 * 4
	flipped := make([]byte, 0, len(rows))
	for y := 15; y >= 0; y-- {
		flipped = append(flipped, rows[y*stride:(y+1)*stride]...)
	}
	copy(rows, flipped)
	img, err := Decode(icoWrap(e))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sameHashes(t, "ico/top-down", src, img)
}

func TestICOMalformed(t *testing.T) {
	good := icoWrap(dibEntry(t, patternNRGBA(16, 16, cBlack, cWhite), 32, true))
	cases := []struct {
		name string
		data []byte
	}{
		{"header only", icoMagic},
		{"zero entries", []byte{0, 0, 1, 0, 0, 0}},
		{"directory truncated", []byte{0, 0, 1, 0, 4, 0, 1, 2, 3}},
		{"payload past EOF", func() []byte {
			d := append([]byte(nil), good...)
			binary.LittleEndian.PutUint32(d[icoDirLen+12:], 1<<30) // offset
			return d
		}()},
		{"zero-length payload", func() []byte {
			d := append([]byte(nil), good...)
			binary.LittleEndian.PutUint32(d[icoDirLen+8:], 0) // size
			return d
		}()},
		{"DIB header truncated", icoWrap(icoEntry{w: 16, h: 16, bpp: 32, payload: []byte{40, 0, 0}})},
		{"unsupported depth", func() []byte {
			d := append([]byte(nil), good...)
			binary.LittleEndian.PutUint16(d[icoDirLen+icoEntryLen+14:], 2) // 2 bpp: not a DIB depth
			return d
		}()},
		{"compressed DIB", func() []byte {
			d := append([]byte(nil), good...)
			binary.LittleEndian.PutUint32(d[icoDirLen+icoEntryLen+16:], 5) // BI_PNG
			return d
		}()},
		{"pixels truncated", func() []byte {
			e := dibEntry(t, patternNRGBA(16, 16, cBlack, cWhite), 32, true)
			e.payload = e.payload[:dibHeadLen+64]
			return icoWrap(e)
		}()},
		{"bomb dimensions", func() []byte {
			e := dibEntry(t, patternNRGBA(16, 16, cBlack, cWhite), 32, true)
			binary.LittleEndian.PutUint32(e.payload[4:8], 100000)
			binary.LittleEndian.PutUint32(e.payload[8:12], 100000)
			return icoWrap(e)
		}()},
	}
	for _, c := range cases {
		if img, err := Decode(c.data); err == nil {
			t.Errorf("%s: Decode succeeded (bounds %v), want an error", c.name, img.Bounds())
		}
	}
	// The known-good icon this table mutates must itself decode, or the table proves
	// nothing.
	if _, err := Decode(good); err != nil {
		t.Fatalf("the reference icon does not decode: %v", err)
	}
}

// bmpFile wraps a DIB in a BITMAPFILEHEADER to make a standalone .bmp, with slack bytes
// optionally inserted before the pixel rows — real bitmaps do that, and only the file
// header's offset field says where the pixels really start.
func bmpFile(t *testing.T, img *image.NRGBA, bpp, slack int) []byte {
	t.Helper()
	e := dibEntry(t, img, bpp, false)
	palBytes := 0
	if bpp <= 8 {
		palBytes = len(distinctColors(img)) * 4
	}
	body := e.payload
	if slack > 0 {
		head := dibHeadLen + palBytes
		body = append(append(append([]byte(nil), body[:head]...), make([]byte, slack)...), body[head:]...)
	}
	var out bytes.Buffer
	out.WriteString("BM")
	binary.Write(&out, binary.LittleEndian, uint32(bmpFileHeadLen+len(body)))
	binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&out, binary.LittleEndian, uint32(bmpFileHeadLen+dibHeadLen+palBytes+slack))
	out.Write(body)
	return out.Bytes()
}

func TestBMPStandalone(t *testing.T) {
	// A bare BMP served as favicon.ico. Every depth, and with slack before the pixels to
	// prove the file header's offset is what places them.
	for _, bpp := range []int{32, 24, 8, 4, 1} {
		colors := []color.NRGBA{cBlack, cWhite, cRed, cBlue}
		if bpp == 1 {
			colors = colors[:2]
		} else if bpp == 4 {
			colors = colors[:3]
		}
		src := patternNRGBA(32, 32, colors...)
		for _, slack := range []int{0, 8} {
			img, err := Decode(bmpFile(t, src, bpp, slack))
			if err != nil {
				t.Errorf("%d bpp (slack %d): Decode: %v", bpp, slack, err)
				continue
			}
			if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
				t.Errorf("%d bpp (slack %d): bounds = %v, want 32x32 (height wrongly halved?)", bpp, slack, b)
				continue
			}
			sameHashes(t, "bmp", src, img)
		}
	}
}

func TestBMP32ZeroAlphaIsOpaque(t *testing.T) {
	// In a BI_RGB 32-bpp bitmap the fourth byte is officially unused, and encoders leave it
	// zero. Read as alpha that makes the whole image transparent — a blank white square
	// with no fingerprint. It must come out opaque instead.
	src := patternNRGBA(32, 32, cBlack, cWhite, cRed, cBlue)
	zeroed := patternNRGBA(32, 32, cBlack, cWhite, cRed, cBlue)
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			c := zeroed.NRGBAAt(x, y)
			c.A = 0
			zeroed.SetNRGBA(x, y, c)
		}
	}
	img, err := Decode(bmpFile(t, zeroed, 32, 0))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sameHashes(t, "bmp/zero-alpha", src, img) // hashes the artwork, not a blank square
}

func TestICODirectoryDepthIsNotTrusted(t *testing.T) {
	// Real icons write 0 in the directory's bpp field, or a value that disagrees with the
	// bitmap. The DIB header is the only trustworthy source: trusting the directory reads
	// the rows at the wrong stride, and mistakes a 32-bpp alpha channel for a legacy mask.
	src := patternNRGBA(32, 32, cBlack, cWhite, cRed, cBlue)
	for _, dirBPP := range []int{0, 8, 24} { // none of these is the truth (32 is)
		e := dibEntry(t, src, 32, true)
		e.bpp = dirBPP
		img, err := Decode(icoWrap(e))
		if err != nil {
			t.Errorf("directory bpp %d: Decode: %v", dirBPP, err)
			continue
		}
		sameHashes(t, "ico/dir-bpp", src, img)
	}

	// Same trap, sharper: an all-transparent AND mask beside a fully opaque alpha channel.
	// The alpha channel wins for a 32-bpp bitmap, so the artwork survives.
	e := dibEntry(t, src, 32, true)
	e.bpp = 0
	mask := dibHeadLen + 32*32*4
	for i := mask; i < len(e.payload); i++ {
		e.payload[i] = 0xff // every mask bit set: "transparent"
	}
	img, err := Decode(icoWrap(e))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sameHashes(t, "ico/alpha-beats-mask", src, img)
}

func TestICONotConfusedWithOtherFormats(t *testing.T) {
	// A PNG must still take the standard-library path, and a cursor file (type 2) is not
	// an icon.
	var buf bytes.Buffer
	if err := png.Encode(&buf, gradient(16, 16, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); err != nil {
		t.Errorf("plain PNG no longer decodes: %v", err)
	}
	cursor := append([]byte(nil), icoWrap(pngEntry(t, patternNRGBA(16, 16, cBlack, cWhite)))...)
	cursor[2] = 2 // type: cursor
	if _, err := Decode(cursor); err == nil {
		t.Error("a cursor file decoded as an icon")
	}
}
