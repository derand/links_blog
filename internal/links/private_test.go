package links

import "testing"

func TestPrivate(t *testing.T) {
	for url, want := range map[string]bool{
		"https://discord.com/channels/199479709736632320/1054166210859773963/1482878897816211516": true,
		"https://canary.discord.com/channels/1/2":                                                 true,
		"https://discordapp.com/channels/1/2/3":                                                   true,
		"https://t.me/c/1729400185/1549120":                                                       true,
		"https://www.telegram.me/c/1/2":                                                           true,
		"https://discord.com/invite/abc":                                                          false,
		"https://discord.com/developers/docs/intro":                                               false,
		"https://t.me/Coin_Post/15792":                                                            false,
		"https://t.me/cryptoforto":                                                                false,
		"https://example.com/channels/1/2":                                                        false,
		"https://t.me/canal/1":                                                                    false,
	} {
		if got := Private(url); got != want {
			t.Errorf("Private(%q) = %v, want %v", url, got, want)
		}
	}
}
