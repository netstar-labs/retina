# retina — executive summary

**What.** retina fingerprints images so a brand's logo or favicon can be matched even
when the domain and page text do not — the visual axis the org previously had no
coverage of. Two fingerprint kinds: perceptual hashes (`PHash`/`DHash`/`AHash` +
`Distance`) for "does this look like the brand?", and a Shodan-compatible `Favicon`
hash for "who else serves this exact icon?".

**Why it matters.** A phishing kit clones the brand's logo and favicon because that is
what convinces a victim — and that clone is invisible to every name-based matcher. A
perceptual hash catches a copied logo through scaling and re-compression; the favicon
hash, being identical to Shodan's `http.favicon.hash`, turns one hit into a search that
finds every host reusing the icon (a whole campaign's fleet).

**How it fits.** retina is the visual stage of the netstar brand-protection family.
`vigil` discovers a brand-adjacent domain, `twist`/`unmask`/`echo` score its name, and
retina scores the page it serves. It pairs with `ditto` (text near-duplicate) for
full page-clone detection and feeds a visual signal to `mirage`.

**Shape.** `PHash`/`DHash`/`AHash(image.Image) uint64` reduce an image to a 64-bit hash
whose `Distance` (Hamming) to a brand-asset hash is small when the images look alike;
`Decode([]byte)` turns PNG/JPEG/GIF into an image with a decompression-bomb guard;
`Favicon([]byte) int32` is the exact-content Shodan favicon hash. PHash (DCT,
brightness-invariant) is the most robust; AHash the cheapest; DHash gradient-based.

**Verification.** The favicon hash is pinned to real `mmh3` + `base64.encodebytes`
values, so it matches Shodan exactly. The perceptual hashes are tested for determinism,
scale-robustness beating discrimination, and a PNG round-trip; `Decode`/`Favicon`/the
hashers are fuzzed for no-panic on arbitrary input.

**Boundaries.** Fingerprinting only — no fetching pages or screenshots (a worker's job),
no OCR, no logo-detection ML, no verdict. Pure Go, standard library only, no CGO.
