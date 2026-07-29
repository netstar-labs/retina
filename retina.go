package retina

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/bits"
	"sort"

	_ "image/gif"  // register GIF decoder for Decode
	_ "image/jpeg" // register JPEG decoder for Decode
	_ "image/png"  // register PNG decoder for Decode
)

// MaxPixels caps the pixel count [Decode] will accept, so a decompression-bomb image
// (huge dimensions from a tiny file) cannot exhaust memory. ~40M px covers a 4K
// screenshot with headroom; brand logos and favicons are far smaller.
const MaxPixels = 40 << 20

// Decode decodes a PNG, JPEG, GIF, ICO, or BMP image, rejecting one whose declared
// dimensions exceed [MaxPixels] before the full, memory-allocating decode.
//
// The format is taken from the bytes, not from any filename, because a favicon is
// routinely served under the wrong extension — a "favicon.ico" is often a PNG, a GIF, or
// a bare BMP. ICO and BMP are decoded by this package, the standard library having no
// decoder for either; the rest go to [image.Decode]. Neither is published through
// [image.RegisterFormat], so they are reachable through Decode and not through a bare
// image.Decode elsewhere in the program. WebP is the one format left out: decoding it
// would cost retina its zero dependencies.
func Decode(data []byte) (image.Image, error) {
	switch {
	case bytes.HasPrefix(data, icoMagic):
		return decodeICO(data)
	case bytes.HasPrefix(data, bmpMagic):
		return decodeBMP(data)
	}
	return decodeBounded(data)
}

// decodeBounded is Decode's standard-library path: read the cheap header-only config
// first and reject an over-[MaxPixels] image before the allocating decode, so a
// decompression bomb (huge dimensions from a tiny file) cannot exhaust memory.
func decodeBounded(data []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := checkPixels(cfg.Width, cfg.Height); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// checkPixels rejects degenerate or over-[MaxPixels] dimensions.
func checkPixels(w, h int) error {
	if w <= 0 || h <= 0 || int64(w)*int64(h) > MaxPixels {
		return fmt.Errorf("retina: image %dx%d exceeds MaxPixels (%d)", w, h, MaxPixels)
	}
	return nil
}

// Distance is the Hamming distance between two hashes — the number of differing bits.
// A small distance means two images (or favicons) are perceptually close; identical
// images hash to distance 0.
func Distance(a, b uint64) int { return bits.OnesCount64(a ^ b) }

// AHash is the average hash: downscale to 8×8 grayscale, then set each bit where the
// pixel is brighter than the mean. The cheapest, most forgiving perceptual hash — a
// good coarse pre-filter.
func AHash(img image.Image) uint64 {
	g := grayResize(img, 8, 8)
	var mean float64
	for _, v := range g {
		mean += v
	}
	mean /= float64(len(g))
	var h uint64
	for i, v := range g {
		if v > mean {
			h |= 1 << uint(63-i)
		}
	}
	return h
}

// DHash is the difference hash: downscale to 9×8 grayscale, then set each bit where a
// pixel is darker than its right-hand neighbour — i.e. where the gradient rises to the
// right (8 comparisons per row × 8 rows, the usual dhash convention). It keys on
// gradients, so it is robust to brightness and gamma shifts.
func DHash(img image.Image) uint64 {
	const w, h = 9, 8
	g := grayResize(img, w, h)
	var hash uint64
	i := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w-1; x++ {
			if g[y*w+x] < g[y*w+x+1] {
				hash |= 1 << uint(63-i)
			}
			i++
		}
	}
	return hash
}

// PHash is the perceptual (DCT) hash: downscale to 32×32 grayscale, take the 2-D
// DCT-II, keep the top-left 8×8 low-frequency block, and set each bit where an AC
// coefficient exceeds the median of that block. The DC term (overall brightness) is
// excluded from both the median and the bits, so the hash keys on structure and is
// invariant to brightness — a re-lit or re-toned logo still matches. 63 AC bits (bit 0
// is unused); the most robust of the three to scaling, compression, and minor edits.
// A near-flat image (little structure) has only sub-epsilon DCT residue in the AC
// terms, so its PHash is floating-point noise and not meaningfully comparable — expected
// for a structureless input, and not a concern for real logos or screenshots.
func PHash(img image.Image) uint64 {
	d := dct2D(grayResize(img, phashDim, phashDim))
	vals := make([]float64, 0, phashLow*phashLow)
	for y := 0; y < phashLow; y++ {
		for x := 0; x < phashLow; x++ {
			vals = append(vals, d[y*phashDim+x])
		}
	}
	med := medianExcludingDC(vals)
	var h uint64
	for i := 1; i < len(vals); i++ { // skip vals[0] (DC / brightness)
		if vals[i] > med {
			h |= 1 << uint(64-i) // i=1→bit 63 … i=63→bit 1
		}
	}
	return h
}

const (
	phashDim = 32 // the DCT works on a 32×32 grayscale downscale
	phashLow = 8  // the low-frequency block kept is 8×8 = 64 bits
)

// luma601 is the Rec.601 luma of one pixel composited over an opaque white background,
// in the 0…0xffff range.
//
// The compositing is not cosmetic. [image.Image.At] returns alpha-premultiplied
// channels, so a fully transparent pixel arrives as black — and brand logos and
// favicons routinely ship on a transparent background. Hashing the premultiplied values
// directly would key on the alpha silhouette against black rather than on the artwork:
// the same mark drawn in dark ink and in white ink hashes identically, and neither
// matches a screenshot of the mark as a browser renders it. Compositing over white
// first is what makes those match, white being what an unstyled page shows.
//
// Each channel is composited before the weighting rather than after — algebraically the
// same, since the weights sum to 1, but not in floating point: the shortcut leaves a
// transparent pixel an epsilon away from an opaque white one, which is enough to flip a
// PHash bit wherever a flat region puts DCT coefficients in a tie with the median. A
// caller needing a different backdrop (a white-on-transparent dark-mode logo vanishes
// against white) should composite explicitly with [image/draw] before hashing.
func luma601(c color.Color) float64 {
	r, g, b, a := c.RGBA()
	d := float64(0xffff - a) // the uncovered part of the pixel, showing white through
	return 0.299*(float64(r)+d) + 0.587*(float64(g)+d) + 0.114*(float64(b)+d)
}

// grayResize downscales img to a w×h grayscale ([luma601]) matrix by area-averaging, so
// a hash is independent of the source resolution and format.
// Row-major; the zero matrix for a degenerate (empty) image.
func grayResize(img image.Image, w, h int) []float64 {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	out := make([]float64, w*h)
	if sw <= 0 || sh <= 0 {
		return out
	}
	for ty := 0; ty < h; ty++ {
		sy0 := b.Min.Y + ty*sh/h
		sy1 := b.Min.Y + (ty+1)*sh/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for tx := 0; tx < w; tx++ {
			sx0 := b.Min.X + tx*sw/w
			sx1 := b.Min.X + (tx+1)*sw/w
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var sum, n float64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					sum += luma601(img.At(sx, sy))
					n++
				}
			}
			out[ty*w+tx] = sum / n
		}
	}
	return out
}

// dctCos[k][x] = cos(π·(x+½)·k / N) for N = phashDim — precomputed once so PHash does
// no trigonometry per call.
var dctCos = func() [phashDim][phashDim]float64 {
	var t [phashDim][phashDim]float64
	for k := 0; k < phashDim; k++ {
		for x := 0; x < phashDim; x++ {
			t[k][x] = math.Cos(math.Pi * (float64(x) + 0.5) * float64(k) / float64(phashDim))
		}
	}
	return t
}()

// dct2D computes the separable 2-D DCT-II of a phashDim×phashDim matrix (rows then
// columns), returning the coefficients row-major.
func dct2D(m []float64) []float64 {
	tmp := make([]float64, phashDim*phashDim)
	for y := 0; y < phashDim; y++ {
		for k := 0; k < phashDim; k++ {
			var s float64
			for x := 0; x < phashDim; x++ {
				s += m[y*phashDim+x] * dctCos[k][x]
			}
			tmp[y*phashDim+k] = s
		}
	}
	out := make([]float64, phashDim*phashDim)
	for x := 0; x < phashDim; x++ {
		for k := 0; k < phashDim; k++ {
			var s float64
			for y := 0; y < phashDim; y++ {
				s += tmp[y*phashDim+x] * dctCos[k][y]
			}
			out[k*phashDim+x] = s
		}
	}
	return out
}

// medianExcludingDC returns the median of vals[1:] — the low-frequency coefficients
// without the DC term (vals[0]), which otherwise dominates and skews the threshold.
func medianExcludingDC(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	rest := append([]float64(nil), vals[1:]...)
	sort.Float64s(rest)
	n := len(rest)
	if n%2 == 1 {
		return rest[n/2]
	}
	return (rest[n/2-1] + rest[n/2]) / 2
}
