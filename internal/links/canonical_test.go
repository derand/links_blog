package links

import "testing"

func TestCanonical(t *testing.T) {
	tests := []struct{ in, want string }{
		// Left as written.
		{"https://example.com/a?b=1#c", "https://example.com/a?b=1#c"},
		{"http://www.example.com/", "http://www.example.com/"},
		{"https://uk.wikipedia.org/wiki/Київ", "https://uk.wikipedia.org/wiki/Київ"},
		{"https://youtu.be/dQw4w9WgXcQ?t=44", "https://youtu.be/dQw4w9WgXcQ?t=44"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLabc", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLabc"},
		{"https://www.youtube.com/playlist?list=LL", "https://www.youtube.com/playlist?list=LL"},
		{"https://example.com/?t=5&s=1", "https://example.com/?t=5&s=1"},
		{"not a url", "not a url"},
		// Tracking parameters, on any site.
		{"https://example.com/p?from=x&utm_campaign=y#top", "https://example.com/p?from=x#top"},
		{"https://example.com/Київ?fbclid=1", "https://example.com/Київ"},
		// X and its mirrors.
		{"https://x.com/someone/status/123?t=abc&s=33", "https://x.com/someone/status/123"},
		{"https://x.com/someone/status/123?s=46&t=l", "https://x.com/someone/status/123"},
		{"https://fixupx.com/i/status/123", "https://x.com/i/status/123"},
		{"https://fxtwitter.com/someone/status/123?s=20", "https://x.com/someone/status/123"},
		{"http://www.twitter.com/someone", "https://x.com/someone"},
		{"https://mobile.twitter.com/someone/status/123", "https://x.com/someone/status/123"},
		// YouTube.
		{"https://youtu.be/dQw4w9WgXcQ?si=abc", "https://youtu.be/dQw4w9WgXcQ"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=LL&index=4&t=44s&ab_channel=X",
			"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=44s"},
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&pp=x&feature=share&lc=Ugw1", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&lc=Ugw1"},
	}
	for _, tt := range tests {
		if got := Canonical(tt.in); got != tt.want {
			t.Errorf("Canonical(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The kept URL must have the same dedup key as the URL it came from.
func TestCanonicalKeepsKey(t *testing.T) {
	for _, in := range []string{
		"https://fixupx.com/someone/status/123?s=20",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=LL&index=4&t=44s",
		"https://example.com/p?from=x&utm_campaign=y",
		"https://twitter.com/someone?t=x",
	} {
		a, _ := Normalize(in)
		b, _ := Normalize(Canonical(in))
		if a != b {
			t.Errorf("%q: key %q, after Canonical %q", in, a, b)
		}
	}
}
