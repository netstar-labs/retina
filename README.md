# retina

Visual fingerprinting for Go — **hash a brand's logo or favicon so a phishing page
that clones the pixels gives itself away, even when the domain and text do not**. Pure
Go, **zero dependencies**, `image/*` standard library only (no CGO). retina is the
**visual axis** of the netstar brand-protection family: `snare`/`unmask`/`echo` match a
*name* by edits, glyphs, and sound — retina matches the *image*.

```go
img, _ := retina.Decode(iconBytes)    // PNG/JPEG/GIF/ICO/BMP, decompression-bomb guarded
h := retina.PHash(img)                // 64-bit perceptual (DCT) hash
retina.Distance(h, brandPHash)        // small ⇒ looks like the brand

retina.Favicon(iconBytes)             // Shodan-compatible favicon hash (int32)
```

## Two fingerprints, two questions

| Question | Function(s) | Kind |
|---|---|---|
| *Does this look like the brand?* | `PHash` (DCT) · `DHash` (gradient) · `AHash` (average) + `Distance` | perceptual — robust to scale, re-compression, minor edits |
| *Who else serves this exact icon?* | `Favicon` | exact-content — the Shodan `http.favicon.hash` |

`PHash` is the most robust (brightness-invariant, DCT low-frequency); `AHash` the
cheapest coarse pre-filter; `DHash` keys on gradients. `Favicon` matches Shodan/Censys
exactly, so a hit **pivots straight into a Shodan search** for every host reusing the
icon.

## Reading real favicons

`Decode` takes the format from the **bytes, not the filename** — a `favicon.ico` in the
wild is as often a PNG, a GIF, or a bare BMP as an actual icon. **ICO and BMP are decoded
here**, the standard library having no decoder for either: an icon's largest image wins
(ties by colour depth), the payload being an embedded PNG or a classic DIB at 1/4/8/16/24/32
bpp with its transparency mask. The colour depth comes from the bitmap header, never from
the icon directory's `bpp` field, which real icons leave at 0 or fill in wrongly.

**Transparency composites over white.** A logo or favicon usually ships on a transparent
background; hashed as-is that background reads as *black*, so the hash keys on the alpha
silhouette instead of the artwork — a dark and a white mark of the same shape then collide,
and neither matches a screenshot of the page. retina composites over white first, which is
what an unstyled page shows. Need a different backdrop? Composite it yourself with
`image/draw` before hashing.

WebP is the one format left out: decoding it would cost retina its zero dependencies.

## Where it fits

`vigil` surfaces a brand-adjacent domain from Certificate Transparency; `snare`/`unmask`/`echo`
score its *name*; retina scores the *page it serves* — a logo `PHash` close to the
brand's, or a favicon hash that Shodan shows on a hundred other hosts. Pairs with
`ditto` (text near-duplicate) for full page-clone detection, and feeds a visual signal
to `mirage`.

## Quick start

```go
// Match a candidate logo against a brand asset:
brand, _ := retina.Decode(brandLogoPNG)
cand, _ := retina.Decode(candidatePNG)
if retina.Distance(retina.PHash(brand), retina.PHash(cand)) <= 10 {
    // visually close — likely a clone
}

// Pivot on a favicon (compare to Shodan's http.favicon.hash):
fmt.Println(retina.Favicon(faviconBytes))
```

```sh
go run ./app/retina hash logo.png candidate.png   # file⇥phash⇥dhash⇥ahash (hex)
go run ./app/retina favicon favicon.ico            # file⇥shodan-hash
go run ./app/retina distance 30ba31cfceba3038 323233cdcc3233ce   # bit distance
```

## Documentation

- **Start here** — [docs/introduction.md](docs/introduction.md) ·
  [docs/executive-summary.md](docs/executive-summary.md)
- **Deep dive** — [docs/architecture.md](docs/architecture.md)
- **Operations** — [docs/userguide.md](docs/userguide.md)
- **Examples** — [example/README.md](example/README.md)

## Layout

| File | Purpose |
|---|---|
| [retina.go](retina.go) | `PHash`/`DHash`/`AHash`, `Distance`, `Decode`, and the grayscale-downscale + DCT internals |
| [ico.go](ico.go) | the ICO and BMP decoders the standard library lacks (icon directory, DIB, transparency mask) |
| [favicon.go](favicon.go) | `Favicon` — the Shodan-compatible MurmurHash3 favicon hash |
| [doc.go](doc.go) | package doc — the visual-axis metaphor and the two fingerprint kinds |
| [app/retina/](app/retina/main.go) | the CLI — `hash` · `favicon` · `distance` · `version` |

## Notes

- Go module `github.com/netstar-labs/retina`. **Standard library only** — no
  dependencies, no CGO. Build standalone with `GOWORK=off`.
- **Hostile-input safe.** `Decode` rejects an over-`MaxPixels` image at the header
  stage (decompression-bomb guard); the hashers and `Favicon` never panic on arbitrary
  input (fuzzed). Every offset an icon or bitmap declares is bounds-checked before use.
- **Verified against real icons.** The ICO/BMP decoders were checked over 4,756 `.ico`
  files found on a developer machine: 4,737 decode (the 19 that do not are WebP), and every
  genuine icon is **pixel-identical to Pillow's** decode of the same frame — bar the cases
  where Pillow trusts the directory's `bpp` field and retina does not, where retina is
  right and Pillow renders a legacy mask over a live alpha channel or fails outright.
- **Batch exit status.** The CLI reports a per-file error on stderr and keeps going, but
  exits non-zero if any file failed — `exit 0` means everything hashed.
- **Scope.** Fingerprinting only — retina does not fetch pages or screenshots (a
  `worker`/consumer job), do OCR, or run logo-detection ML. See
  [docs/architecture.md](docs/architecture.md) § "Deliberately out".
