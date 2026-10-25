package links

import (
	"net/url"
	"testing"
)

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

func TestOEmbedPages(t *testing.T) {
	for raw, want := range map[string]string{
		"https://www.reddit.com/r/flask/comments/2321oc/easiest_and_fastest_way_to_host_flask_python/": "reddit",
		"https://old.reddit.com/r/flask/comments/2321oc/":                                              "reddit",
		"https://www.reddit.com/r/flask/comments/2321oc/comment/cgsb2ka/":                              "reddit",
		"https://www.reddit.com/r/CryptoCurrency/":                                                     "",
		"https://www.reddit.com/r/BASE/s/9PvTKUSIFl":                                                   "",
		"https://www.reddit.com/user/executex/":                                                        "",
		"https://www.tiktok.com/@zory_stopmotion":                                                      "tiktok",
		"https://www.tiktok.com/@makashovskiy?source=h5_m":                                             "tiktok",
		"https://www.tiktok.com/@scout2015/video/6718335390845095173":                                  "tiktok",
		"https://www.tiktok.com/explore":                                                               "",
		"https://vm.tiktok.com/ZMabc123/":                                                              "",
		"https://example.com/@nick":                                                                    "",
	} {
		u, _ := url.Parse(raw)
		got := ""
		switch {
		case RedditPost(u):
			got = "reddit"
		case TikTokPage(u):
			got = "tiktok"
		}
		if got != want {
			t.Errorf("%s: %q, want %q", raw, got, want)
		}
	}
}
