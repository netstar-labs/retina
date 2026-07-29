# retina — audit findings (v0.1.0 baseline)

Single-pass audit over the v0.1.0 baseline, 2026-07, across four dimensions: correctness
of the hash arithmetic, hostile input, **real-world format coverage** (the dimension that
turned out to matter most), and doc-drift. Every finding was reproduced against the code
before repair — `*` marks the ones with a reproduction artifact recorded below.

Repairs re-validated: gofmt / build / vet / `go test -race` / staticcheck clean;
`FuzzDecode` re-soaked over the new ICO/BMP parsers (7.2M execs, no panic); the perceptual
decoders diffed **pixel-for-pixel against Pillow** over 4,756 real `.ico` files; and
`Favicon` re-checked end-to-end against Python `mmh3` + `base64.encodebytes` on a real
`favicon.ico` (`-469509536` both sides).

## CONFIRMED — fixed

### C-1 · MAJOR — alpha read as black: the hashes keyed on the silhouette, not the artwork `*`

`grayResize` took `img.At(x, y).RGBA()`, which returns **alpha-premultiplied** channels, and
discarded alpha. A fully transparent pixel therefore arrives as `(0,0,0)` — black — and
brand logos and favicons routinely ship on a transparent background. The hash saw the
artwork cut out of black rather than the artwork.

Reproduced with a synthetic wordmark (solid ink rect on an alpha-0 background):

- the same asset, transparent background vs. composited over white as a browser renders it:
  **AHash distance 64** — a complete inversion — **PHash 58**, DHash 8. A brand asset never
  matched a screenshot of itself, which is the primary cross-match the pipeline exists for.
- a **dark** mark and a **white** mark of the same shape hashed **identically**
  (`PHash 9393926c6c6c6d92`, `AHash 0078780000000000` for both): the bits only ever encode
  "ink brighter than the black background", so ink colour was invisible to all three hashes
  and unrelated assets with a shared silhouette collided.

**Fix:** `luma601` composites each pixel over an opaque white background before the Rec.601
weighting — white being what an unstyled page shows. Opaque images are bit-for-bit
unaffected (their uncovered alpha is zero). A caller wanting another backdrop composites it
with `image/draw` first, which the docs now say.

Composited **per channel**, not by the algebraically equivalent single addition of the
uncovered alpha: the three weights sum to 1 in real arithmetic but not in float64, so the
shortcut left a transparent pixel an epsilon from an opaque white one — enough to flip two
PHash bits wherever a flat region ties with the block median. Caught only because the
exactness test failed at distance 2; a tolerance-based test would have shipped it.

Guarded by `TestTransparentBackgroundMatchesRendered` (exact equality across all three
hashes, plus a tolerance case for antialiased partial alpha) and
`TestInkColourIsNotLostToAlpha`.

### C-2 · MAJOR — no ICO decode, in a library whose headline use is favicon matching `*`

The standard library has no ICO decoder, so `Decode` on any real `favicon.ico` returned
`image: unknown format` — the entire perceptual half was unreachable for the most common
favicon format on the web. (`Favicon` was unaffected; it fingerprints raw bytes.)

Sampling 4,756 real `.ico` files also showed that a `favicon.ico` is frequently **not an
icon**: 567 were PNG or GIF, 7 a bare BMP, 19 WebP. Trusting the extension — or the
stdlib's format set — loses most of the population one way or another.

**Fix:** `ico.go`, zero dependencies. Icon directory → **largest** entry (ties by colour
depth), that being the frame with the most signal for a perceptual hash; payloads decoded
as embedded PNG (modern) or DIB at 1/4/8/16/24/32 bpp, bottom-up or top-down, palette, and
the 1-bpp AND mask. Bare **BMP** too, reusing the DIB path. `Decode` dispatches on the
leading bytes rather than any filename. WebP stays out: it would cost retina its zero
dependencies, and it is now documented as the one gap.

Two sub-decisions, both load-bearing:

- **The directory's `bpp` field is not trusted** — real icons leave it 0 or contradict the
  bitmap header. Depth comes from the DIB header alone; reading the directory instead walks
  the rows at the wrong stride and mistakes a 32-bpp alpha channel for a legacy mask. This
  is the sole reason retina's decode ever differs from Pillow's, and where it does, Pillow
  is the one that is wrong (below).
- **A 32-bpp alpha channel wins over the AND mask**, a modern icon's mask being decorative
  and often stale — but an **all-zero** alpha channel is taken as opaque, that being an
  encoder quirk rather than a genuinely invisible icon. Without the fallback such an icon
  hashes as a blank white square.

Every offset and index an icon or bitmap declares is bounds-checked before use, and
`MaxPixels` bounds both the PNG-payload and DIB paths.

**Validation.** 4,737 of the 4,756 files now decode; the 19 that do not are WebP. The
v0.1.0 binary, run over the same corpus, decoded **567** — only the PNG and GIF files
wearing an `.ico` extension, and not one actual icon.

Of the **4,163** genuine icons, **4,133 decode pixel-identically to Pillow**. All 30
remaining paths (16 distinct files, several duplicated in the corpus) are Pillow trusting the
directory `bpp`: 26 where it decodes the frame differently — a stale mask over a live alpha
channel — and 4 where it cannot open the file at all (`buffer is not large enough`, on a
24-bpp bitmap whose directory claims 32). Re-running every one of the 30 with Pillow forced
to the depth the bitmap header declares makes **all 30 match exactly, none unresolved** —
which is what establishes the direction of the disagreement rather than assuming retina is
the right one. BMP matches
Pillow exactly bar ±1 mid-scale rounding on 5-bit channels (retina replicates bits, so full
scale maps to `0xff` rather than `0xf8`).

Guarded by `TestICOEmbeddedPNG`, `TestICODIBDepths`, `TestICOMaskTransparency`,
`TestICOPicksLargestEntry`, `TestICOTopDownDIB`, `TestICODirectoryDepthIsNotTrusted`,
`TestICOMalformed` (10 hostile shapes), `TestBMPStandalone`, `TestBMP32ZeroAlphaIsOpaque`,
`TestICONotConfusedWithOtherFormats`.

### C-3 · MAJOR — the CLI exited 0 when every file failed `*`

`hash` and `favicon` printed a per-file error and continued — correct, a batch should not
abort — but then unconditionally `return nil`, so a run in which *nothing* decoded exited 0.
The same fail-open exit shape as vigil #2. Reproduced against the v0.1.0 binary: two
nonexistent paths, two errors on stderr, **`exit 0`**; the same input now exits 1 with
`hash: 2 of 2 file(s) failed`.

**Fix:** both count failures and return `batchErr`, so the exit status is non-zero if any
file failed while the rows that succeeded still print. Documented in the CLI help, the user
guide, and the README, since a caller has to be able to trust `exit 0`.

### C-4 · MINOR — `medianExcludingDC` panicked before its own length check

It sliced `vals[1:]` and *then* tested `len(rest) == 0`, so an empty input panicked on the
line before the guard. Unreachable from `PHash`, which always passes 64 values, but a latent
trap for any future caller. **Fix:** the length check moved ahead of the slice.

### D-1 · doc-drift — `DHash`'s documented comparison was inverted against its code

The comment claimed a bit is set where a pixel is "**brighter** than its right-hand
neighbour"; the code is `g[i] < g[i+1]`, i.e. darker — the gradient rising to the right. The
code matches the usual dhash convention (and imagehash), so the comment was the error, and
changing the code instead would have invalidated every stored hash for nothing. **Fix:** the
comment now states the code's actual comparison and names the convention.

## Considered, deliberately not changed

- **Typed fast paths in `grayResize`.** Every hash walks the full source through the
  `image.Image` interface, so hashing a 4K screenshot is ~25M dynamic dispatches. A type
  switch over `*image.RGBA`/`*image.NRGBA`/`*image.Gray`/`*image.YCbCr` would cut that
  several-fold, but it doubles the luma code and risks the fast and slow paths diverging —
  an optimisation with no measured need. `BenchmarkHashes1080p` was added instead, so the
  cost is visible before anyone pays down that risk.
- **PHash is not imagehash-compatible**, and deliberately: retina excludes the DC term from
  both the median and the bits (imagehash includes it), buying brightness invariance at the
  price of cross-tool comparability. Only `Favicon` claims compatibility with anything, and
  it is pinned to Shodan by known-answer tests.
- **A near-flat image's PHash remains floating-point noise.** Documented rather than fixed:
  there is no structure to hash, and `DHash`/`AHash` of a uniform image are exactly 0, which
  is the stable, testable fact.
