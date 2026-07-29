# retina — architecture

retina is two small, independent fingerprinting paths over the standard library: a
perceptual-hash path (decode → grayscale downscale → hash) and an exact-content favicon
path (base64 → MurmurHash3). No state, no I/O beyond decoding bytes you pass in, no
dependencies, no CGO.

## Data flow

```
 image bytes ─▶ Decode ─▶ image.Image ─┬─▶ grayResize 32×32 ─▶ DCT ─▶ 8×8 low-freq ─▶ PHash
   (bomb-guarded, PNG/JPEG/GIF)         ├─▶ grayResize 9×8  ─▶ h-gradient bits    ─▶ DHash
                                        └─▶ grayResize 8×8  ─▶ >mean bits         ─▶ AHash

 icon bytes  ─▶ base64.encodebytes (76-col) ─▶ MurmurHash3 x86_32 (seed 0) ─▶ Favicon (int32)

 two hashes  ─▶ XOR ─▶ popcount ─▶ Distance (bit difference)
```

## Subsystems

| Piece | Responsibility |
|---|---|
| `retina.go` | `PHash`/`DHash`/`AHash`, `Distance`, `Decode` (with the `MaxPixels` bomb guard), and the internals: `grayResize` (area-average downscale to a grayscale matrix), the precomputed DCT cosine table, `dct2D` (separable DCT-II), `medianExcludingDC`. |
| `favicon.go` | `Favicon`, `base64Chunked` (Python `base64.encodebytes`-compatible wrapping), and the canonical `murmur3x86_32`. |

## The perceptual hashes

All three start with `grayResize`, which **area-averages** the source into a small
grayscale (Rec.601 luma) grid — so a hash is independent of the source resolution and
format. From there:

- **PHash** — 32×32 grid → separable 2-D DCT-II → the top-left 8×8 low-frequency block →
  set each AC bit where the coefficient exceeds the block median. The **DC term is
  excluded** from both the median and the bits, so the hash is brightness-invariant
  (63 AC bits). Most robust to scaling/compression. The cosine table is precomputed
  once, so PHash does no trigonometry per call.
- **DHash** — 9×8 grid → 64 bits, each comparing a cell to its right-hand neighbour.
  Gradient-based, cheap, robust to smooth brightness shifts.
- **AHash** — 8×8 grid → 64 bits, each vs the mean. The coarsest, most forgiving.

`Distance` is `bits.OnesCount64(a ^ b)` — the Hamming distance. A small distance means
the images are perceptually close; identical images hash equal (distance 0).

## The favicon hash

`Favicon` is exact-content, not perceptual: it reproduces Shodan's `http.favicon.hash`
so a value can be pivoted straight into Shodan/Censys. That means matching two quirks
exactly: the icon bytes are encoded with Python's `base64.encodebytes` (standard base64
wrapped at 76 columns, each line newline-terminated), and hashed with **MurmurHash3 x86
32-bit, seed 0**, reported as a signed `int32`. Both are pinned by known-answer tests
against real `mmh3`.

## Design choices & trade-offs

- **Pure stdlib, no resize dependency.** Go has no image-resize in the standard library,
  so `grayResize` implements a simple area-average downscale — robust for the
  downscaling hashing needs and keeps retina zero-dependency (no `x/image`, no CGO).
- **Brightness-invariant PHash.** Dropping the DC coefficient makes a re-lit or re-toned
  logo still match. The cost: a near-flat image has no stable structure, so its PHash is
  floating-point noise — expected and harmless for real logos/screenshots (DHash/AHash of
  a uniform image are exactly 0).
- **Decompression-bomb guard.** `Decode` checks `image.DecodeConfig` first and rejects an
  image whose declared dimensions exceed `MaxPixels` **before** the full, allocating
  decode — a tiny file cannot force a multi-gigabyte allocation.
- **Shodan-compatible favicon, deliberately.** The interoperability (pivoting on an
  existing global index) is worth reproducing Python's exact base64 wrapping; a "cleaner"
  encoding would silently stop matching Shodan.

## Deliberately out (YAGNI)

- **No page/screenshot fetching** — a `worker`/crawler job; retina hashes bytes you have.
- **No OCR, no logo-detection ML** — this is arithmetic over pixels, not a model.
- **No image resize/format library** — the built-in downscale is enough for hashing.
- **No scoring / verdict** — retina emits a hash and a distance; weighing them is the
  consumer's, or `mirage`'s.
