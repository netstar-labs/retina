package retina

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// icoMagic is the ICONDIR prefix of a Windows icon: reserved uint16 = 0 followed by
// type uint16 = 1 (icon). Cursor files (type 2) share the layout but are not icons, and
// are not accepted.
var icoMagic = []byte{0x00, 0x00, 0x01, 0x00}

// pngMagic is the PNG signature, which an icon entry's payload carries when the image is
// stored as an embedded PNG instead of a DIB.
var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// bmpMagic is the BITMAPFILEHEADER prefix of a standalone Windows bitmap.
var bmpMagic = []byte("BM")

const (
	icoDirLen      = 6  // ICONDIR: reserved, type, count
	icoEntryLen    = 16 // ICONDIRENTRY
	dibHeadLen     = 40 // BITMAPINFOHEADER
	bmpFileHeadLen = 14 // BITMAPFILEHEADER
)

// decodeBMP decodes a standalone Windows bitmap, which the standard library also cannot
// read. This is here because favicon.ico in the wild is sometimes a bare BMP under the
// wrong extension — browsers sniff the bytes rather than trust the name, so retina does
// too. What follows the file header is the same DIB an icon carries, minus the
// transparency mask, and with the file header saying where the pixels start.
func decodeBMP(data []byte) (image.Image, error) {
	if len(data) < bmpFileHeadLen+dibHeadLen {
		return nil, fmt.Errorf("retina: bmp: truncated (%d bytes)", len(data))
	}
	dib := data[bmpFileHeadLen:]
	h := int(int32(binary.LittleEndian.Uint32(dib[8:12])))
	if h < 0 {
		h = -h // a negative height means top-down rows, not a different height
	}
	// Passing the declared height as the "directory" height tells decodeDIB there is no
	// mask doubling to undo.
	return decodeDIB(dib, h, int(binary.LittleEndian.Uint32(data[10:14]))-bmpFileHeadLen)
}

// decodeICO decodes a Windows icon — favicon.ico, the most common favicon format on the
// web and one the standard library cannot read. An icon file is a directory of images;
// retina takes the largest (ties broken by colour depth), that being the one carrying
// the most signal for a perceptual hash. A payload is either an embedded PNG (modern
// icons) or a bottom-up DIB with a 1-bpp transparency mask (classic icons: 1, 4, 8, 24,
// or 32 bpp). [MaxPixels] bounds both paths.
//
// Only the perceptual hashes need any of this — [Favicon] fingerprints the raw bytes and
// never decodes.
func decodeICO(data []byte) (image.Image, error) {
	entries, err := icoDir(data)
	if err != nil {
		return nil, err
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if e.w*e.h > best.w*best.h || (e.w*e.h == best.w*best.h && e.bpp > best.bpp) {
			best = e
		}
	}
	if bytes.HasPrefix(best.payload, pngMagic) {
		return decodeBounded(best.payload)
	}
	return decodeDIB(best.payload, best.h, -1)
}

// icoEntry is one parsed ICONDIRENTRY: the size and colour depth the directory declares
// for an image, and the payload bytes it points at.
type icoEntry struct {
	w, h    int
	bpp     int
	payload []byte
}

// icoDir parses the icon directory. An entry whose payload falls outside the file is
// skipped rather than fatal — a multi-size icon with one bad record is still usable —
// but a directory with no usable entry is an error.
func icoDir(data []byte) ([]icoEntry, error) {
	if len(data) < icoDirLen || !bytes.HasPrefix(data, icoMagic) {
		return nil, fmt.Errorf("retina: ico: not an icon file")
	}
	n := int(binary.LittleEndian.Uint16(data[4:6]))
	if n == 0 {
		return nil, fmt.Errorf("retina: ico: empty icon directory")
	}
	if len(data) < icoDirLen+n*icoEntryLen {
		return nil, fmt.Errorf("retina: ico: directory of %d entries truncated", n)
	}
	out := make([]icoEntry, 0, n)
	for i := 0; i < n; i++ {
		e := data[icoDirLen+i*icoEntryLen : icoDirLen+(i+1)*icoEntryLen]
		w, h := int(e[0]), int(e[1]) // a 0 byte means 256, which it cannot hold
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		size := uint64(binary.LittleEndian.Uint32(e[8:12]))
		off := uint64(binary.LittleEndian.Uint32(e[12:16]))
		if size == 0 || off < icoDirLen || off+size > uint64(len(data)) {
			continue
		}
		out = append(out, icoEntry{
			w:       w,
			h:       h,
			bpp:     int(binary.LittleEndian.Uint16(e[6:8])),
			payload: data[off : off+size],
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("retina: ico: no usable image among %d directory entries", n)
	}
	return out, nil
}

// decodeDIB decodes a device-independent bitmap: a BITMAPINFOHEADER, an optional palette,
// the XOR pixel rows (bottom-up unless the declared height is negative), then — inside an
// icon — a 1-bpp AND mask whose set bits are transparent pixels. dirH is the height the
// icon directory declared, which is what distinguishes a mask-doubled height from a plain
// one. pixOff locates the pixel rows within p, or -1 for "straight after the palette",
// which is where an icon keeps them.
//
// The colour depth comes from this header, never from the icon directory's bpp field:
// real icons write 0 there, or disagree with the bitmap outright, and trusting it reads
// the pixels at the wrong stride.
func decodeDIB(p []byte, dirH, pixOff int) (image.Image, error) {
	if len(p) < dibHeadLen {
		return nil, fmt.Errorf("retina: ico: DIB header truncated (%d bytes)", len(p))
	}
	headLen := int(binary.LittleEndian.Uint32(p[0:4]))
	if headLen < dibHeadLen || headLen > len(p) {
		return nil, fmt.Errorf("retina: ico: bad DIB header length %d", headLen)
	}
	w := int(int32(binary.LittleEndian.Uint32(p[4:8])))
	rawH := int(int32(binary.LittleEndian.Uint32(p[8:12])))
	bpp := int(binary.LittleEndian.Uint16(p[14:16]))
	if comp := binary.LittleEndian.Uint32(p[16:20]); comp != 0 {
		return nil, fmt.Errorf("retina: ico: compressed DIB (compression %d) unsupported", comp)
	}
	topDown := rawH < 0
	if topDown {
		rawH = -rawH
	}
	// The XOR image and the AND mask are stacked, so a DIB inside an icon declares twice
	// its real height. Let the directory entry settle it where it can.
	h, hasMask := rawH, false
	switch {
	case rawH == 2*dirH:
		h, hasMask = dirH, true
	case rawH == dirH: // no mask
	case rawH%2 == 0:
		h, hasMask = rawH/2, true
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("retina: ico: bad DIB dimensions %dx%d", w, h)
	}
	if err := checkPixels(w, h); err != nil {
		return nil, err
	}

	var pal []color.NRGBA
	switch bpp {
	case 1, 4, 8:
		n := int(binary.LittleEndian.Uint32(p[32:36])) // biClrUsed
		if n <= 0 || n > 1<<bpp {
			n = 1 << bpp
		}
		if headLen+n*4 > len(p) {
			return nil, fmt.Errorf("retina: ico: palette of %d entries truncated", n)
		}
		pal = make([]color.NRGBA, n)
		for i := range pal {
			e := p[headLen+i*4:] // BGRX
			pal[i] = color.NRGBA{R: e[2], G: e[1], B: e[0], A: 0xff}
		}
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("retina: dib: unsupported colour depth %d bpp", bpp)
	}

	if pixOff < 0 {
		pixOff = headLen + len(pal)*4
	}
	if pixOff < dibHeadLen || pixOff > len(p) {
		return nil, fmt.Errorf("retina: dib: pixel data offset %d outside the bitmap", pixOff)
	}
	pix := p[pixOff:]
	stride := ((w*bpp + 31) / 32) * 4 // DIB rows are padded to 4 bytes
	if len(pix) < h*stride {
		return nil, fmt.Errorf("retina: ico: pixel data truncated (want %d bytes, have %d)", h*stride, len(pix))
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for y := 0; y < h; y++ {
		src := dibRow(y, h, topDown)
		row := pix[src*stride : (src+1)*stride]
		for x := 0; x < w; x++ {
			var c color.NRGBA
			idx := -1
			switch bpp {
			case 32:
				q := row[x*4:]
				c = color.NRGBA{R: q[2], G: q[1], B: q[0], A: q[3]}
				anyAlpha = anyAlpha || q[3] != 0
			case 24:
				q := row[x*3:]
				c = color.NRGBA{R: q[2], G: q[1], B: q[0], A: 0xff}
			case 16:
				q := binary.LittleEndian.Uint16(row[x*2:]) // BI_RGB 16 bpp is RGB555
				c = color.NRGBA{R: expand5(q >> 10), G: expand5(q >> 5), B: expand5(q), A: 0xff}
			case 8:
				idx = int(row[x])
			case 4:
				if idx = int(row[x/2] >> 4); x%2 == 1 {
					idx = int(row[x/2] & 0x0f)
				}
			case 1:
				idx = int(row[x/8] >> (7 - uint(x)%8) & 1)
			}
			if idx >= 0 {
				if idx >= len(pal) {
					return nil, fmt.Errorf("retina: ico: palette index %d out of range (%d colours)", idx, len(pal))
				}
				c = pal[idx]
			}
			img.SetNRGBA(x, y, c)
		}
	}

	// A 32-bpp icon carries its own alpha, but encoders do sometimes leave the channel
	// all-zero, which would read as a fully transparent image; take that as opaque and
	// let the mask (if any) speak instead.
	if bpp == 32 && !anyAlpha {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 0xff
		}
	}
	if hasMask && (bpp != 32 || !anyAlpha) {
		applyANDMask(img, pix[h*stride:], w, h, topDown)
	}
	return img, nil
}

// applyANDMask clears every pixel whose mask bit is set. A truncated mask is tolerated
// and the image left opaque: that costs only transparency, not pixels, and a hash of an
// opaque icon still beats no hash at all.
func applyANDMask(img *image.NRGBA, mask []byte, w, h int, topDown bool) {
	stride := ((w + 31) / 32) * 4
	if len(mask) < h*stride {
		return
	}
	for y := 0; y < h; y++ {
		src := dibRow(y, h, topDown)
		row := mask[src*stride : (src+1)*stride]
		for x := 0; x < w; x++ {
			if row[x/8]>>(7-uint(x)%8)&1 == 1 {
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
}

// expand5 widens a 5-bit channel to 8 bits by bit replication, so full scale maps to 0xff
// rather than the 0xf8 a bare shift would give — white has to stay white.
func expand5(v uint16) uint8 {
	v &= 0x1f
	return uint8(v<<3 | v>>2)
}

// dibRow maps a destination row to its source row: DIB rows are stored bottom-up unless
// the bitmap declared a negative height.
func dibRow(y, h int, topDown bool) int {
	if topDown {
		return y
	}
	return h - 1 - y
}
