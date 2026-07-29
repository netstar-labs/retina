// Package retina fingerprints images so a brand's logo or favicon can be matched even
// when the domain and page text do not. It is the visual axis of the netstar
// brand-protection family: twist/unmask/echo match a name by edits, glyphs, and sound,
// and retina matches the pixels — a phishing kit that clones the real logo gives itself
// away here.
//
// Two kinds of fingerprint, for two questions.
//
// Perceptual hashes answer "does this image look like that one?" — robust to scaling,
// re-compression, and minor edits. [AHash] (average), [DHash] (gradient), and [PHash]
// (DCT) each reduce an [image.Image] to a 64-bit hash whose [Distance] (Hamming) to a
// brand-asset hash is small when the images are visually close. PHash is the most
// robust; AHash the most forgiving/cheap; DHash keys on gradients.
//
// [Decode] turns image bytes into an [image.Image] for hashing, with a decompression-bomb
// guard, and takes the format from the bytes rather than any filename — a favicon.ico in
// the wild is as often a PNG, a GIF, or a bare BMP as a real icon. PNG, JPEG, and GIF go
// to the standard library; ICO and BMP are decoded here, since the standard library reads
// neither. Transparency is composited over white, so the hash keys on the artwork as a
// page would show it rather than on an alpha silhouette against black.
//
// [Favicon] answers "who else serves this exact icon?" — an exact-content fingerprint,
// not a perceptual one. It is the Shodan-compatible favicon hash (MurmurHash3 x86
// 32-bit of the icon's line-wrapped base64), so a value can be pivoted straight into a
// Shodan or Censys search for every host reusing the icon.
//
// retina is pure Go, standard library only (image/*, no CGO). It compares against a
// caller-held set of brand-asset hashes; it does not fetch pages or screenshots (a
// worker/consumer job), do OCR, or run logo-detection ML — it turns an image you
// already have into a comparable fingerprint, and nothing more.
package retina
