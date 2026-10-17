package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/derand/links_blog/internal/auth"
	"github.com/derand/links_blog/internal/bot/discord"
	"github.com/derand/links_blog/internal/service"
	"github.com/derand/links_blog/internal/store"
	"github.com/derand/links_blog/internal/web"
)

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// envList splits a comma-separated environment variable.
func envList(name string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(name), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// startDiscord runs the Discord bot in the background. Its failures are logged
// and do not stop the site.
func startDiscord(ctx context.Context, token string, links service.LinkService) {
	b, err := discord.New(discord.Config{Token: token, AllowedUsers: envList("DISCORD_ALLOWED_USERS")}, links)
	if err != nil {
		slog.Error("discord bot is disabled", "err", err)
		return
	}
	go func() {
		if err := b.Run(ctx); err != nil {
			slog.Error("discord bot stopped", "err", err)
		}
	}()
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := dbFlag(fs)
	addr := fs.String("addr", env("LINKSBLOG_ADDR", ":8080"), "listen address (env LINKSBLOG_ADDR)")
	fs.Usage = func() {
		fs.PrintDefaults()
		os.Stderr.WriteString(`
Environment:
  TZ                        time zone of the blog's days, e.g. Europe/Kyiv
                            (default: the system's local time; UTC in the container)
  LINKSBLOG_BASE_URL        public URL of the site, e.g. https://links.example.com
  LINKSBLOG_SESSION_SECRET  random string of 32+ bytes for signing cookies (required)
  LINKSBLOG_ADMIN_EMAILS    comma-separated Google accounts allowed to sign in
  GOOGLE_CLIENT_ID          Google OAuth client
  GOOGLE_CLIENT_SECRET
Sign-in is disabled unless the base URL, admin emails and Google client are set.
  DISCORD_TOKEN             Discord bot token; the bot runs only when it is set
  DISCORD_ALLOWED_USERS     comma-separated Discord user IDs whose messages are accepted
`)
	}
	fs.Parse(args)

	a, err := auth.New(auth.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		BaseURL:      os.Getenv("LINKSBLOG_BASE_URL"),
		Admins:       envList("LINKSBLOG_ADMIN_EMAILS"),
		Secret:       []byte(os.Getenv("LINKSBLOG_SESSION_SECRET")),
	})
	if err != nil {
		return errors.New("LINKSBLOG_SESSION_SECRET: " + err.Error())
	}
	if !a.LoginEnabled() {
		slog.Warn("sign-in is disabled: set LINKSBLOG_BASE_URL, LINKSBLOG_ADMIN_EMAILS, GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if n, err := st.CountLinks(ctx); err == nil {
		slog.Info("database opened", "path", *dbPath, "links", n)
	}

	zone, _ := time.Now().Zone()
	slog.Info("time zone of the days", "zone", zone)
	h, err := web.New(st, a, time.Local)
	if err != nil {
		return err
	}
	if token := os.Getenv("DISCORD_TOKEN"); token != "" {
		startDiscord(ctx, token, service.New(st, time.Local))
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", *addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
