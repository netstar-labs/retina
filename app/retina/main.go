// Command retina fingerprints image files for visual brand-abuse matching.
//
//	retina hash <file...>       # phash, dhash, ahash (hex) per image file
//	retina favicon <file...>    # Shodan-compatible favicon hash (int32) per file
//	retina distance <a> <b>     # Hamming distance between two hex hashes
//	retina version
//
// hash and favicon read the named image/icon files. hash prints
// "file <tab> phash <tab> dhash <tab> ahash" (16 hex digits each); favicon prints
// "file <tab> hash" (the signed 32-bit Shodan http.favicon.hash). distance parses two
// 64-bit hex hashes and prints their bit distance. A per-file error goes to stderr and
// does not abort the batch, but the exit status is 1 if any file failed — a caller
// scripting this must not read "exit 0" as "everything hashed".
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"

	"github.com/netstar-labs/retina"
)

// stamped by the build via -ldflags -X.
var (
	version = "dev"
	build   = "none"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "hash":
		err = hash(os.Args[2:])
	case "favicon":
		err = favicon(os.Args[2:])
	case "distance":
		err = distance(os.Args[2:])
	case "version", "-version", "--version", "-v":
		fmt.Printf("retina %s (%s)\n", version, build)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "retina:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: retina <hash|favicon|distance|version> [args...]")
	fmt.Fprintln(os.Stderr, "  retina hash <file...>       phash/dhash/ahash (hex) per image")
	fmt.Fprintln(os.Stderr, "  retina favicon <file...>    Shodan favicon hash per icon")
	fmt.Fprintln(os.Stderr, "  retina distance <a> <b>     bit distance between two hex hashes")
	os.Exit(2)
}

func hash(files []string) error {
	if len(files) == 0 {
		return fmt.Errorf("hash: need at least one file")
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	var failed int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "retina: %s: %v\n", f, err)
			failed++
			continue
		}
		img, err := retina.Decode(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "retina: %s: %v\n", f, err)
			failed++
			continue
		}
		fmt.Fprintf(w, "%s\t%016x\t%016x\t%016x\n", f, retina.PHash(img), retina.DHash(img), retina.AHash(img))
	}
	return batchErr("hash", failed, len(files))
}

// batchErr turns a per-file failure count into the command's exit status, so a batch that
// silently hashed nothing does not look like a success.
func batchErr(cmd string, failed, total int) error {
	if failed == 0 {
		return nil
	}
	return fmt.Errorf("%s: %d of %d file(s) failed", cmd, failed, total)
}

func favicon(files []string) error {
	if len(files) == 0 {
		return fmt.Errorf("favicon: need at least one file")
	}
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	var failed int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "retina: %s: %v\n", f, err)
			failed++
			continue
		}
		fmt.Fprintf(w, "%s\t%d\n", f, retina.Favicon(data))
	}
	return batchErr("favicon", failed, len(files))
}

func distance(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("distance: need exactly two hex hashes")
	}
	a, err := strconv.ParseUint(args[0], 16, 64)
	if err != nil {
		return fmt.Errorf("distance: bad hash %q: %w", args[0], err)
	}
	b, err := strconv.ParseUint(args[1], 16, 64)
	if err != nil {
		return fmt.Errorf("distance: bad hash %q: %w", args[1], err)
	}
	fmt.Println(retina.Distance(a, b))
	return nil
}
