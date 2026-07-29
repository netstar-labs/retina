package retina

import "testing"

// MurmurHash3 x86_32 (seed 0) known-answer values, cross-checked against Python's mmh3
// (which is exactly this algorithm, reported signed) — the ground truth Shodan uses.
func TestMurmur3KnownAnswers(t *testing.T) {
	cases := map[string]int32{
		"":      0,
		"foo":   -156908512,
		"hello": 613153351,
		"test":  -1167338989,
	}
	for in, want := range cases {
		if got := int32(murmur3x86_32([]byte(in), 0)); got != want {
			t.Errorf("murmur3(%q) = %d, want %d", in, got, want)
		}
	}
}

// Favicon must equal Shodan's http.favicon.hash: mmh3 of Python base64.encodebytes of
// the icon bytes. Values captured from mmh3 + base64.encodebytes.
func TestFaviconShodanCompatible(t *testing.T) {
	bytes256 := make([]byte, 256)
	for i := range bytes256 {
		bytes256[i] = byte(i)
	}
	cases := []struct {
		name string
		in   []byte
		want int32
	}{
		{"range256", bytes256, -757223386},
		{"abc", []byte("abc"), -868969266},
		{"empty", []byte{}, 0},
	}
	for _, c := range cases {
		if got := Favicon(c.in); got != c.want {
			t.Errorf("Favicon(%s) = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBase64Chunked(t *testing.T) {
	// Matches Python base64.encodebytes: 76-col lines, each with a trailing newline.
	if got := string(base64Chunked([]byte("abc"))); got != "YWJj\n" {
		t.Errorf("base64Chunked(abc) = %q, want %q", got, "YWJj\n")
	}
	if base64Chunked(nil) != nil {
		t.Error("base64Chunked(nil) should be nil (empty)")
	}
	// Long input wraps at 76 columns; every line but the content is ≤76 chars.
	long := make([]byte, 300)
	out := string(base64Chunked(long))
	for _, line := range splitLines(out) {
		if len(line) > 76 {
			t.Errorf("line exceeds 76 columns: %d", len(line))
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
