# retina examples

| Example | Shows | Run |
|---|---|---|
| [hash](hash/main.go) | perceptual-hash matching + a Shodan favicon hash, offline | `go run ./example/hash` |

The `hash` example is network-free: it builds a synthetic "brand mark", the same mark at
half resolution, and a different mark, then prints their `PHash`es and the `Distance`
between them — showing the rescaled mark lands close to the original and the different
mark lands far. It also prints a Shodan-compatible favicon hash.

For the CLI on real files:

```sh
go run ./app/retina hash brand.png suspect.png
go run ./app/retina favicon suspect-favicon.ico
```
