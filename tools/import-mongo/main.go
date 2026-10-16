// Command import-mongo is a one-off tool that moves the links collection of the
// old Flask + MongoDB blog into a new, empty SQLite database.
//
//	mongoexport --uri "$MONGODB_CONNECTION" --collection links --jsonArray --out data/links.json
//	go run ./tools/import-mongo -in data/links.json -db data/linksblog.db
//
// Both plain and --jsonArray output of mongoexport are accepted.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/derand/links_blog/internal/links"
	"github.com/derand/links_blog/internal/store"
)

func main() {
	in := flag.String("in", "", "mongoexport output file (required)")
	dbPath := flag.String("db", "data/linksblog.db", "SQLite database to create; must be empty")
	tz := flag.String("tz", "Local", "time zone of the blog's days, e.g. Europe/Kyiv")
	flag.Parse()
	if *in == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*in, *dbPath, *tz); err != nil {
		log.Fatal(err)
	}
}

func run(in, dbPath, tz string) error {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	docs, err := readDocs(data)
	if err != nil {
		return err
	}
	res := convert(docs, loc)
	for _, w := range res.warnings {
		fmt.Println("WARN", w)
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	ctx := context.Background()
	s, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer s.Close()
	if n, err := s.CountLinks(ctx); err != nil {
		return err
	} else if n > 0 {
		return fmt.Errorf("%s already has %d links; import needs an empty database", dbPath, n)
	}
	if err := s.InsertLinks(ctx, res.links); err != nil {
		return err
	}
	n, err := s.CountLinks(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("documents read:   %d\n", len(docs))
	fmt.Printf("links imported:   %d (hidden: %d)\n", len(res.links), res.hidden)
	fmt.Printf("skipped:          %d\n", res.skipped)
	if len(res.ignoredFields) > 0 {
		fmt.Printf("ignored fields:   %s\n", strings.Join(res.ignoredFields, ", "))
	}
	if n != len(res.links) {
		return fmt.Errorf("database has %d links, expected %d", n, len(res.links))
	}
	fmt.Printf("database check:   ok, %d links in %s\n", n, dbPath)
	return nil
}

type doc = map[string]any

// readDocs parses mongoexport output: a JSON array or one document per line.
func readDocs(data []byte) ([]doc, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		var docs []doc
		err := dec.Decode(&docs)
		return docs, err
	}
	var docs []doc
	for {
		var d doc
		err := dec.Decode(&d)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", len(docs)+1, err)
		}
		docs = append(docs, d)
	}
}

type result struct {
	links         []*store.Link
	hidden        int
	skipped       int
	warnings      []string
	ignoredFields []string
}

var knownFields = map[string]bool{
	"_id": true, "url": true, "description": true, "tags": true,
	"day": true, "created": true, "hidden": true,
}

func convert(docs []doc, loc *time.Location) result {
	var res result
	warn := func(format string, args ...any) {
		res.warnings = append(res.warnings, fmt.Sprintf(format, args...))
	}
	ignored := map[string]bool{}

	for i, d := range docs {
		for k := range d {
			if !knownFields[k] {
				ignored[k] = true
			}
		}
		id := oid(d["_id"])
		if id == "" {
			id = "#" + strconv.Itoa(i+1)
		}
		rawURL, _ := d["url"].(string)
		rawURL = strings.TrimSpace(rawURL)
		norm, err := links.Normalize(rawURL)
		if err != nil {
			warn("%s: skipped, bad url %q: %v", id, rawURL, err)
			res.skipped++
			continue
		}

		created, _ := number(d["created"])
		day, err := mongoDay(d["day"])
		if err != nil {
			if created <= 0 {
				warn("%s: skipped, %v and no created time (%s)", id, err, rawURL)
				res.skipped++
				continue
			}
			day = links.Day(time.Unix(created, 0), loc)
			warn("%s: %v, day taken from created time: %s", id, err, day)
		}
		createdAt := time.Unix(created, 0)
		if created <= 0 {
			// Old posts had no creation time; use the start of their day.
			t, _ := time.ParseInLocation(links.DayLayout, day, loc)
			createdAt = t
		}

		description, _ := d["description"].(string)
		l := &store.Link{
			URL:         rawURL,
			URLNorm:     norm,
			Description: strings.TrimSpace(description),
			Day:         day,
			CreatedAt:   createdAt,
			Hidden:      truthy(d["hidden"]),
			Source:      "mongo",
			SourceRef:   oid(d["_id"]),
			Tags:        tags(d["tags"]),
		}
		res.links = append(res.links, l)
	}

	// Stable chronological IDs: oldest first, like the old ordering within a day.
	sort.SliceStable(res.links, func(i, j int) bool {
		a, b := res.links[i], res.links[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})

	// Same link posted twice: keep the first one.
	seen := map[string]*store.Link{}
	kept := res.links[:0]
	for _, l := range res.links {
		if first, ok := seen[l.URLNorm]; ok {
			warn("%s: skipped, duplicate of %s posted %s (%s)", l.SourceRef, first.SourceRef, first.Day, l.URL)
			res.skipped++
			continue
		}
		seen[l.URLNorm] = l
		kept = append(kept, l)
		if l.Hidden {
			res.hidden++
		}
	}
	res.links = kept

	for k := range ignored {
		res.ignoredFields = append(res.ignoredFields, k)
	}
	slices.Sort(res.ignoredFields)
	return res
}

// mongoDay converts the old YYYYDDD day number (year*1000 + day of year).
func mongoDay(v any) (string, error) {
	n, ok := number(v)
	if !ok {
		return "", fmt.Errorf("no day")
	}
	year, yday := int(n/1000), int(n%1000)
	if year < 1990 || year > 2100 || yday < 1 || yday > 366 {
		return "", fmt.Errorf("bad day %d", n)
	}
	t := time.Date(year, time.January, yday, 0, 0, 0, 0, time.UTC)
	if t.Year() != year {
		return "", fmt.Errorf("bad day %d", n)
	}
	return t.Format(links.DayLayout), nil
}

// number reads an integer from JSON, including MongoDB extended JSON wrappers.
func number(v any) (int64, bool) {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n, true
		}
		if f, err := x.Float64(); err == nil {
			return int64(f), true
		}
	case map[string]any:
		for _, k := range []string{"$numberInt", "$numberLong", "$numberDouble"} {
			if s, ok := x[k].(string); ok {
				return number(json.Number(s))
			}
		}
	}
	return 0, false
}

func oid(v any) string {
	if m, ok := v.(map[string]any); ok {
		s, _ := m["$oid"].(string)
		return s
	}
	s, _ := v.(string)
	return s
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.ToLower(strings.TrimSpace(x))
		return x != "" && x != "false" && x != "0" && x != "off"
	case json.Number:
		n, _ := number(x)
		return n != 0
	}
	return false
}

func tags(v any) []string {
	arr, _ := v.([]any)
	var out []string
	for _, t := range arr {
		s, _ := t.(string)
		s = strings.TrimSpace(s)
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
