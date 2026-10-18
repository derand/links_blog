package links

import (
	"net/url"
	"strings"
)

// privatePaths are links that only members can open: messages in Discord
// servers and in private Telegram chats. Host without "www.", path prefix.
var privatePaths = map[string]string{
	"discord.com":        "/channels/",
	"ptb.discord.com":    "/channels/",
	"canary.discord.com": "/channels/",
	"discordapp.com":     "/channels/",
	"t.me":               "/c/",
	"telegram.me":        "/c/",
}

// Private reports whether a stranger can't open the link: such links are
// hidden when added and their pages are not fetched.
func Private(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	prefix, ok := privatePaths[strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")]
	return ok && strings.HasPrefix(u.Path, prefix)
}
