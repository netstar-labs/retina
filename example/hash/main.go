// Runnable example: fingerprint synthetic images and compare them, offline.
//
//	go run ./example/hash
//
// It builds a "brand mark", the same mark at half resolution, and a different mark,
// then shows that the perceptual hash puts the rescaled mark close to the original and
// the different mark far — the core of visual look-alike detection. It also shows a
// Shodan-compatible favicon hash.
package main

import (
	"fmt"
	"image"
	"image/color"

	"github.com/netstar-labs/retina"
)

// mark draws a bold plus (+) sign — a stand-in for a brand logo — sized w×h. When
// diagonal is true it draws an X instead, a clearly different mark.
func mark(w, h int, diagonal bool) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	th := w / 6
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			on := false
			if diagonal {
				on = abs(x-y) < th || abs((w-1-x)-y) < th
			} else {
				on = abs(x-w/2) < th || abs(y-h/2) < th
			}
			c := uint8(30)
			if on {
				c = 230
			}
			img.SetGray(x, y, color.Gray{Y: c})
		}
	}
	return img
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func main() {
	logo := mark(128, 128, false)
	rescaled := mark(64, 64, false) // same mark, half resolution
	imposter := mark(128, 128, true)

	lh, rh, ih := retina.PHash(logo), retina.PHash(rescaled), retina.PHash(imposter)
	fmt.Printf("logo      PHash = %016x\n", lh)
	fmt.Printf("rescaled  PHash = %016x  distance to logo = %d  (close — same mark)\n", rh, retina.Distance(lh, rh))
	fmt.Printf("imposter  PHash = %016x  distance to logo = %d  (far — different mark)\n", ih, retina.Distance(lh, ih))

	fmt.Printf("\nfavicon(\"example-icon-bytes\") = %d  (Shodan http.favicon.hash)\n",
		retina.Favicon([]byte("example-icon-bytes")))
}
