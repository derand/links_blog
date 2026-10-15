package links

import (
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://example.com", "example.com"},
		{"https://example.com/", "example.com"},
		{"  HTTPS://WWW.Example.COM/Path/  ", "example.com/Path"},
		{"example.com/a/b/", "example.com/a/b"},
		{"example.com:8080/x", "example.com:8080/x"},
		{"http://example.com:80/x", "example.com/x"},
		{"https://example.com:443/x", "example.com/x"},
		{"http://example.com:8080/x", "example.com:8080/x"},
		{"https://example.com/?b=2&a=1", "example.com?a=1&b=2"},
		{"https://example.com/p?utm_source=x&id=5&UTM_medium=y&fbclid=z", "example.com/p?id=5"},
		{"https://example.com/p?utm_source=x", "example.com/p"},
		{"https://example.com/p#section-2", "example.com/p#section-2"},
		{"https://example.com/p#", "example.com/p"},
		{"https://uk.wikipedia.org/wiki/%D0%9A%D0%B8%D1%97%D0%B2", "uk.wikipedia.org/wiki/%D0%9A%D0%B8%D1%97%D0%B2"},
		{"https://uk.wikipedia.org/wiki/Київ", "uk.wikipedia.org/wiki/%D0%9A%D0%B8%D1%97%D0%B2"},
	}
	for _, tt := range tests {
		got, err := Normalize(tt.in)
		if err != nil {
			t.Errorf("Normalize(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeErrors(t *testing.T) {
	for _, in := range []string{"", "   ", "ftp://example.com/file", "mailto:me@example.com", "http://"} {
		if got, err := Normalize(in); err == nil {
			t.Errorf("Normalize(%q) = %q, want error", in, got)
		}
	}
}

func TestDay(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	// 2016-09-07 22:30 UTC is already 2016-09-08 in Kyiv (UTC+3 in summer).
	ts := time.Date(2016, 9, 7, 22, 30, 0, 0, time.UTC)
	if got := Day(ts, kyiv); got != "2016-09-08" {
		t.Errorf("Day = %q, want 2016-09-08", got)
	}
	if got := Day(ts, time.UTC); got != "2016-09-07" {
		t.Errorf("Day UTC = %q, want 2016-09-07", got)
	}
}
