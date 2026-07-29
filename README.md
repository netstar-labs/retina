# retina

Visual fingerprinting for Go — **hash a brand's logo or favicon so a phishing page
that clones the pixels gives itself away, even when the domain and text do not**. Pure
Go, **zero dependencies**, `image/*` standard library only (no CGO). retina is the
**visual axis** of the netstar brand-protection family: `twist`/`unmask`/`echo` match a
*name* by edits, glyphs, and sound — retina matches the *image*.

```go
img, _ := retina.Decode(pngBytes)     // PNG/JPEG/GIF, with a decompression-bomb guard
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

## Where it fits

`vigil` surfaces a brand-adjacent domain from Certificate Transparency; `twist`/`unmask`/`echo`
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
| [favicon.go](favicon.go) | `Favicon` — the Shodan-compatible MurmurHash3 favicon hash |
| [doc.go](doc.go) | package doc — the visual-axis metaphor and the two fingerprint kinds |
| [app/retina/](app/retina/main.go) | the CLI — `hash` · `favicon` · `distance` · `version` |

## Notes

- Go module `github.com/netstar-labs/retina`. **Standard library only** — no
  dependencies, no CGO. Build standalone with `GOWORK=off`.
- **Hostile-input safe.** `Decode` rejects an over-`MaxPixels` image at the header
  stage (decompression-bomb guard); the hashers and `Favicon` never panic on arbitrary
  input (fuzzed).
- **Scope.** Fingerprinting only — retina does not fetch pages or screenshots (a
  `worker`/consumer job), do OCR, or run logo-detection ML. See
  [docs/architecture.md](docs/architecture.md) § "Deliberately out".
