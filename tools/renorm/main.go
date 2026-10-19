// Command renorm is a one-off tool that brings stored links up to the current
// links.Canonical and links.Normalize: it rewrites each link's URL and dedup
// key, leaving metadata and everything else as it is.
//
//	go run ./tools/renorm -db data/linksblog.db         # only shows the changes
//	go run ./tools/renorm -db data/linksblog.db -apply  # writes them in one transaction
//
// A link whose new key another link already has is a duplicate nobody noticed:
// it is shown and left as it is, for the author to sort out on the site.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"

	"github.com/derand/links_blog/internal/links"
	"github.com/derand/links_blog/internal/store"
)

func main() {
	dbPath := flag.String("db", "data/linksblog.db", "SQLite database")
	apply := flag.Bool("apply", false, "write the changes; without it they are only shown")
	flag.Parse()
	if err := run(*dbPath, *apply); err != nil {
		log.Fatal(err)
	}
}

func run(dbPath string, apply bool) error {
	ctx := context.Background()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	us, err := st.LinkURLs(ctx)
	if err != nil {
		return err
	}
	p := plan(us)

	old := map[int64]store.LinkURL{}
	for _, u := range us {
		old[u.ID] = u
	}
	for _, w := range p.warnings {
		fmt.Println("WARN", w)
	}
	for _, c := range p.changes {
		o := old[c.ID]
		fmt.Printf("#%d %s\n", c.ID, o.URL)
		if c.URL != o.URL {
			fmt.Printf("    url -> %s\n", c.URL)
		}
		if c.URLNorm != o.URLNorm {
			fmt.Printf("    key %s -> %s\n", o.URLNorm, c.URLNorm)
		}
	}
	for _, g := range p.conflicts {
		fmt.Printf("CONFLICT %s:\n", g.key)
		for _, id := range g.ids {
			fmt.Printf("    #%d %s\n", id, old[id].URL)
		}
	}
	fmt.Printf("\n%d links, %d to change, %d conflicts\n", len(us), len(p.changes), len(p.conflicts))
	if !apply || len(p.changes) == 0 {
		return nil
	}
	if err := st.SetLinkURLs(ctx, p.changes); err != nil {
		return err
	}
	fmt.Println("applied")
	return nil
}

type conflict struct {
	key string
	ids []int64 // links that would share the key
}

type result struct {
	changes   []store.LinkURL
	conflicts []conflict
	warnings  []string
}

// plan returns the links whose URL or key changes. A link whose new key would
// be shared with another link stays as it is and is reported as a conflict.
func plan(us []store.LinkURL) result {
	var r result
	next := map[int64]store.LinkURL{}
	for _, u := range us {
		url := links.Canonical(u.URL)
		norm, err := links.Normalize(url)
		if err != nil {
			r.warnings = append(r.warnings, fmt.Sprintf("#%d %s: %v", u.ID, u.URL, err))
			continue
		}
		if url != u.URL || norm != u.URLNorm {
			next[u.ID] = store.LinkURL{ID: u.ID, URL: url, URLNorm: norm}
		}
	}

	// Keeping a link as it is may clash with yet another changed link, so
	// repeat until the keys are unique.
	reported := map[string]bool{}
	for {
		byKey := map[string][]int64{}
		for _, u := range us {
			key := u.URLNorm
			if n, ok := next[u.ID]; ok {
				key = n.URLNorm
			}
			byKey[key] = append(byKey[key], u.ID)
		}
		clash := false
		for key, ids := range byKey {
			if len(ids) < 2 {
				continue
			}
			for _, id := range ids {
				if _, ok := next[id]; ok {
					delete(next, id)
					clash = true
				}
			}
			if !reported[key] {
				reported[key] = true
				r.conflicts = append(r.conflicts, conflict{key, ids})
			}
		}
		if !clash {
			break
		}
	}
	sort.Slice(r.conflicts, func(i, j int) bool { return r.conflicts[i].key < r.conflicts[j].key })

	for _, u := range us {
		if n, ok := next[u.ID]; ok {
			r.changes = append(r.changes, n)
		}
	}
	return r
}
