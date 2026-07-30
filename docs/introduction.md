# Meet retina — the logo, not the label

Everything else in the brand-protection family reads a *name*. `snare` measures how
many edits `paypa1.com` is from `paypal.com`; `unmask` folds the Cyrillic look-alike;
`echo` catches the homophone. But a phishing page gives itself away another way, one no
name-matcher can see: it clones the brand's **logo and favicon**, pixel for pixel,
because that is what convinces a human. retina is the sensor for that.

## What it actually is

retina is a pure-Go image fingerprinting library. It reduces an image to a compact hash
you can compare against a brand asset:

- **Perceptual hashes** — [PHash], [DHash], [AHash] — answer *does this look like that?*
  They survive scaling, re-compression, and small edits, because they key on coarse
  structure, not exact bytes. A candidate logo whose [PHash] is a few bits ([Distance])
  from the brand's is almost certainly the same mark.
- **A favicon hash** — [Favicon] — answers *who else serves this exact icon?* It is the
  same hash Shodan indexes as `http.favicon.hash`, so one value pivots into a search
  that returns every host on the internet serving that favicon — a phishing campaign's
  whole fleet at once.

## How the perceptual hash works

Shrink the image to a tiny grayscale grid (so resolution and colour stop mattering),
run a discrete cosine transform to pull out its low-frequency structure, and set one
bit per coefficient according to whether it is above the median. Two images that look
alike produce nearly the same bits; the [Distance] between their hashes is the number
that differ. It is deliberately brightness-invariant — a re-toned logo still matches.

Two details decide whether that works on real assets rather than clean test images. A
brand logo ships on a **transparent** background, and transparency read naively is *black* —
so the hash would key on the cut-out silhouette instead of the artwork, and a dark and a
white version of the same mark would be indistinguishable. retina composites over white,
which is what a page shows. And a **`favicon.ico` is frequently not an icon** — often a PNG,
a GIF, or a bare Windows bitmap wearing the wrong extension — so retina reads the format
from the bytes and decodes the icon and bitmap formats the standard library will not touch.

## The line it will not cross

retina turns an image you already have into a fingerprint. It does **not** go and get
the image: no fetching pages, no headless-browser screenshots — that networked,
stateful job belongs to a `worker` or a crawler. It does no OCR and runs no
logo-detection model; it is arithmetic over pixels, not machine learning. And it makes
no judgement — a small [Distance] is a signal for a scorer to weigh, not a verdict.

## The scope it keeps

An image in, a comparable hash out, plus the Hamming [Distance] to weigh matches — and
the exact-match favicon hash for Shodan pivoting. That narrowness is the point: it keeps
retina a small, dependency-free, side-effect-free function that drops into a visual
detection pipeline and composes with the name-matchers and `ditto` to catch a
page-clone the domain alone would never reveal.

*Read next:* [executive-summary.md](executive-summary.md) ·
[architecture.md](architecture.md) · [userguide.md](userguide.md) ·
[../example/README.md](../example/README.md)
