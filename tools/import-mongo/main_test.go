package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/derand/links_blog/internal/store"
)

// Shapes seen in the old database, in mongoexport's relaxed and canonical JSON.
const sample = `
{"_id":{"$oid":"57b9f673dcba0f5b14f0fb35"},"created":1473323452,"day":2016252,"url":"https://Example.com/a/","description":" Приклад ","tags":["go","sqlite","go",""],"hidden":false}
{"_id":{"$oid":"57b9f673dcba0f5b14f0fb36"},"created":{"$numberLong":"1473409852"},"day":{"$numberInt":"2016253"},"url":"http://b.example.org/x?utm_source=tw","description":"hidden one","hidden":true,"short_url":"b.example.org"}
{"_id":{"$oid":"57b9f673dcba0f5b14f0fb37"},"created":0,"day":2016001,"url":"https://c.example.org","description":"no created time"}
{"_id":{"$oid":"57b9f673dcba0f5b14f0fb38"},"created":1473496252,"day":0,"url":"https://d.example.org","description":"no day"}
{"_id":{"$oid":"57b9f673dcba0f5b14f0fb39"},"created":0,"day":0,"url":"https://e.example.org","description":"nothing"}
{"_id":{"$oid":"57b9f673dcba0f5b14f0fb3a"},"created":1473500000,"day":2016253,"url":"https://www.example.com/a","description":"duplicate"}
{"_id":{"$oid":"57b9f673dcba0f5b14f0fb3b"},"created":1473500000,"day":2016253,"url":"","description":"no url"}
`

func kyiv(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestReadDocsFormats(t *testing.T) {
	lines, err := readDocs([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 7 {
		t.Fatalf("got %d docs, want 7", len(lines))
	}
	arr, err := readDocs([]byte(" [" + strings.Join(strings.Split(strings.TrimSpace(sample), "\n"), ",") + "]"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, arr) {
		t.Error("array and line formats differ")
	}
}

func TestMongoDay(t *testing.T) {
	ok := map[int]string{2016001: "2016-01-01", 2016252: "2016-09-08", 2016366: "2016-12-31", 2017365: "2017-12-31"}
	for in, want := range ok {
		got, err := mongoDay(jsonNumber(in))
		if err != nil || got != want {
			t.Errorf("mongoDay(%d) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []int{0, 2016000, 2017366, 2016999, 1234} {
		if got, err := mongoDay(jsonNumber(in)); err == nil {
			t.Errorf("mongoDay(%d) = %q, want error", in, got)
		}
	}
}

func TestConvert(t *testing.T) {
	docs, err := readDocs([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	res := convert(docs, kyiv(t))

	var got []string
	for _, l := range res.links {
		got = append(got, l.SourceRef[len(l.SourceRef)-2:]+" "+l.Day+" "+l.URLNorm)
	}
	want := []string{
		"37 2016-01-01 c.example.org",
		"35 2016-09-08 example.com/a",
		"36 2016-09-09 b.example.org/x",
		"38 2016-09-10 d.example.org",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links:\n got %q\nwant %q", got, want)
	}
	if res.skipped != 3 || res.hidden != 1 {
		t.Errorf("skipped = %d, hidden = %d; want 3, 1", res.skipped, res.hidden)
	}
	if !reflect.DeepEqual(res.ignoredFields, []string{"short_url"}) {
		t.Errorf("ignored fields = %v", res.ignoredFields)
	}
	if len(res.warnings) != 4 {
		t.Errorf("warnings = %q, want 4", res.warnings)
	}

	a := res.links[1]
	if a.URL != "https://Example.com/a/" || a.Description != "Приклад" || a.Hidden || a.Source != "mongo" ||
		!reflect.DeepEqual(a.Tags, []string{"go", "sqlite"}) || a.CreatedAt.Unix() != 1473323452 {
		t.Errorf("unexpected link: %+v", a)
	}
	c := res.links[0]
	if want := time.Date(2016, 1, 1, 0, 0, 0, 0, kyiv(t)); !c.CreatedAt.Equal(want) {
		t.Errorf("created at = %v, want start of day %v", c.CreatedAt, want)
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "links.json")
	if err := os.WriteFile(in, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "sub", "linksblog.db")
	if err := run(in, db, "Europe/Kyiv"); err != nil {
		t.Fatal(err)
	}
	if err := run(in, db, "Europe/Kyiv"); err == nil || !strings.Contains(err.Error(), "empty database") {
		t.Errorf("second import: err = %v, want refusal", err)
	}

	ctx := context.Background()
	s, err := store.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	l, err := s.LinkByURLNorm(ctx, "b.example.org/x")
	if err != nil || l == nil {
		t.Fatalf("link not found: %v", err)
	}
	if !l.Hidden || l.SourceRef != "57b9f673dcba0f5b14f0fb36" || l.Day != "2016-09-09" {
		t.Errorf("unexpected link: %+v", l)
	}
}

func jsonNumber(n int) json.Number { return json.Number(strconv.Itoa(n)) }
