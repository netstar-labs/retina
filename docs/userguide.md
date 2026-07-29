# retina — user guide

## Library

```go
import "github.com/netstar-labs/retina"

img, err := retina.Decode(pngOrJpegOrGifBytes) // decompression-bomb-guarded
if err != nil { /* not an image, or over MaxPixels */ }

ph := retina.PHash(img) // perceptual (DCT) — most robust
dh := retina.DHash(img) // gradient — cheap, brightness-tolerant
ah := retina.AHash(img) // average — coarsest pre-filter

if retina.Distance(ph, brandPHash) <= 10 { /* visually close to the brand */ }

fav := retina.Favicon(iconBytes) // Shodan-compatible http.favicon.hash (int32)
```

### API

| Symbol | Meaning |
|---|---|
| `Decode(data []byte) (image.Image, error)` | decode PNG/JPEG/GIF, rejecting an image over `MaxPixels` before full decode |
| `PHash(img image.Image) uint64` | perceptual DCT hash (63 AC bits, brightness-invariant) |
| `DHash(img image.Image) uint64` | difference (gradient) hash |
| `AHash(img image.Image) uint64` | average hash |
| `Distance(a, b uint64) int` | Hamming distance (differing bits; 0 = identical, 64 = opposite) |
| `Favicon(icon []byte) int32` | Shodan-compatible favicon hash (mmh3 of line-wrapped base64) |
| `MaxPixels` | the pixel-count cap `Decode` enforces (~40M) |

**Contract.** Hashes are deterministic (same image → same hash). `PHash`/`DHash`/`AHash`
key on structure and are invariant to brightness/scale/format to differing degrees;
`PHash` is the most robust. A **near-flat image** has no stable structure, so its
`PHash` is floating-point noise — such images are not meaningfully comparable (their
`DHash`/`AHash` are 0). `Distance` over any two hashes is the differing-bit count; pick a
threshold empirically (≈10/64 is a reasonable "looks alike" cutoff for `PHash`).
`Favicon` is exact-content — a re-encoded icon changes it; use `PHash` on the decoded
icon image for near-match.

**Choosing a hash.** `PHash` for the primary look-alike decision; `AHash`/`DHash` as
cheap pre-filters over a large candidate set before the DCT. `Favicon` when you want to
pivot on Shodan/Censys (`http.favicon.hash`) rather than compare pixels.

**Safety.** `Decode` guards against decompression bombs (`MaxPixels`); the hashers and
`Favicon` are fuzzed and never panic on arbitrary input.

## CLI

```
retina hash <file...>       # file⇥phash⇥dhash⇥ahash (16 hex digits each)
retina favicon <file...>    # file⇥shodan-hash (signed int32)
retina distance <a> <b>     # bit distance between two hex hashes
retina version
```

- `hash` / `favicon` read the named image/icon files; a per-file error goes to stderr
  and does not abort the batch.
- `distance` parses two 64-bit hex hashes and prints their Hamming distance.

```sh
retina hash brand.png suspect.png
retina favicon suspect-favicon.ico              # compare to Shodan http.favicon.hash
retina distance $(retina hash brand.png | cut -f2) $(retina hash suspect.png | cut -f2)
```

## Building an index

To watch a candidate stream (e.g. `vigil` hits) against a set of brand assets:

```go
brandHashes := []uint64{ /* PHash of each known brand logo */ }
func looksLikeBrand(img image.Image, maxDist int) bool {
    h := retina.PHash(img)
    for _, b := range brandHashes {
        if retina.Distance(h, b) <= maxDist {
            return true
        }
    }
    return false
}
```

## Build & run

```sh
GOWORK=off go build -o retina ./app/retina
GOWORK=off go run ./app/retina hash logo.png
```
