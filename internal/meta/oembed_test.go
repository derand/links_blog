package meta

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchOEmbed(t *testing.T) {
	answers := map[string]string{ // the asked page's URL → JSON
		"https://www.reddit.com/r/flask/comments/2321oc/x/": `{"author_name":"executex","provider_name":"reddit",
			"title":"Easiest and fastest way to host Flask Python","type":"rich"}`,
		"https://www.tiktok.com/@zory_stopmotion": `{"type":"rich","title":"Zoryana Stop Motion's Creator Profile",
			"author_url":"https://www.tiktok.com/@zory_stopmotion","author_name":"Zoryana Stop Motion","embed_type":"profile"}`,
		"https://www.tiktok.com/@ann/video/1": `{"type":"video","title":"a  cat #cats",
			"author_url":"https://www.tiktok.com/@ann","author_name":"Ann","author_unique_id":"ann","embed_type":"video"}`,
		"https://www.tiktok.com/@ann/video/2": `{"type":"video","provider_name":"TikTok","embed_type":"video"}`,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/oembed", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("url")
		switch page {
		case "https://www.reddit.com/r/flask/comments/gone/x/":
			http.Error(w, "not found", 404)
		case "https://www.tiktok.com/@nobody":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			w.Write([]byte(`{"message":"Something went wrong","code":400}`))
		case "https://www.reddit.com/r/flask/comments/html/x/":
			w.Write([]byte(`<title>Reddit</title>`))
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Write([]byte(answers[page]))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := newFetcher(time.Second, allowAll)
	f.redditAPI, f.tiktokAPI = srv.URL+"/oembed", srv.URL+"/oembed"
	ctx := context.Background()
	for page, want := range map[string]Metadata{
		"https://www.reddit.com/r/flask/comments/2321oc/x/": {Title: "Easiest and fastest way to host Flask Python", Author: "executex"},
		"https://www.tiktok.com/@zory_stopmotion":           {Title: "Zoryana Stop Motion", Author: "zory_stopmotion"},
		"https://www.tiktok.com/@ann/video/1":               {Title: "a cat #cats", Author: "ann"},
	} {
		if got, err := f.Fetch(ctx, page); err != nil || got != want {
			t.Errorf("%s: %+v, %v; want %+v", page, got, err, want)
		}
	}
	for page, permanent := range map[string]bool{
		"https://www.tiktok.com/@ann/video/2":             true, // deleted video
		"https://www.reddit.com/r/flask/comments/gone/x/": true,
		"https://www.tiktok.com/@nobody":                  true,
		"https://www.reddit.com/r/flask/comments/html/x/": false,
	} {
		if m, err := f.Fetch(ctx, page); err == nil || Permanent(err) != permanent {
			t.Errorf("%s: %+v, %v; permanent %v, want %v", page, m, err, Permanent(err), permanent)
		}
	}
	var se *StatusError
	if _, err := f.Fetch(ctx, "https://www.reddit.com/r/flask/comments/gone/x/"); !errors.As(err, &se) || se.Code != 404 {
		t.Errorf("deleted post: %v, want status 404", err)
	}
}
