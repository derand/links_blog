package links

import "testing"

func TestTweetNick(t *testing.T) {
	for url, want := range map[string]string{
		"https://x.com/EthanDeFi_/status/2104919895792386342":  "EthanDeFi_",
		"https://twitter.com/jack/statuses/20?s=20":            "jack",
		"https://fixupx.com/Ria_neearts/status/1/photo/1":      "Ria_neearts",
		"https://x.com/i/status/2063532122435145838":           "",
		"https://x.com/i/web/status/20":                        "",
		"https://x.com/jack":                                   "",
		"https://example.com/jack/status/20":                   "",
		"https://x.com/someone_with_a_too_long_nick/status/20": "",
	} {
		if got := TweetNick(url); got != want {
			t.Errorf("TweetNick(%q) = %q, want %q", url, got, want)
		}
	}
}
