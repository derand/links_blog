package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/derand/links_blog/internal/links"
)

// seed inserts links described as "day hidden url description tags...".
func seed(t *testing.T, s *Store) {
	t.Helper()
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	data := []struct {
		day    string
		hidden bool
		url    string
		desc   string
		tags   []string
	}{
		{"2016-08-19", false, "https://a.com", "Перший пост про мережі", []string{"neural network"}},
		{"2016-08-19", false, "https://b.com", "Second on the same day", []string{"go"}},
		{"2016-09-01", true, "https://c.com", "Hidden only day", []string{"go"}},
		{"2017-01-02", false, "https://d.com", "Golang tips", []string{"go", "development"}},
		{"2017-01-02", true, "https://e.com", "Hidden with visible neighbour", nil},
		{"2021-12-01", false, "https://f.com", "Latest", nil},
	}
	var ls []*Link
	for i, d := range data {
		ls = append(ls, &Link{URL: d.url, URLNorm: d.url[8:], Description: d.desc, Day: d.day,
			CreatedAt: base.Add(time.Duration(i) * time.Hour), Hidden: d.hidden, Tags: d.tags})
	}
	if err := s.InsertLinks(context.Background(), ls); err != nil {
		t.Fatal(err)
	}
}

func urls(ls []*Link) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.URL[8:])
	}
	return out
}

func TestDays(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	seed(t, s)

	days, total, err := s.Days(ctx, 0, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2021-12-01", "2017-01-02"}; !reflect.DeepEqual(days, want) || total != 3 {
		t.Errorf("visible: days = %v, total = %d; want %v, 3", days, total, want)
	}
	days, total, _ = s.Days(ctx, 2, 2, true)
	if want := []string{"2016-09-01", "2016-08-19"}; !reflect.DeepEqual(days, want) || total != 4 {
		t.Errorf("with hidden: days = %v, total = %d; want %v, 4", days, total, want)
	}
	days, _, _ = s.Days(ctx, 10, 2, true)
	if len(days) != 0 {
		t.Errorf("past the end: %v", days)
	}
}

func TestLinksOnDays(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	seed(t, s)

	ls, err := s.LinksOnDays(ctx, []string{"2017-01-02", "2016-08-19"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"d.com", "a.com", "b.com"}; !reflect.DeepEqual(urls(ls), want) {
		t.Errorf("visible = %v, want %v", urls(ls), want)
	}
	if want := []string{"go", "development"}; !reflect.DeepEqual(ls[0].Tags, want) {
		t.Errorf("tags = %v, want %v", ls[0].Tags, want)
	}
	ls, _ = s.LinksOnDays(ctx, []string{"2017-01-02"}, true)
	if want := []string{"d.com", "e.com"}; !reflect.DeepEqual(urls(ls), want) {
		t.Errorf("with hidden = %v, want %v", urls(ls), want)
	}
	if ls, err := s.LinksOnDays(ctx, nil, true); err != nil || ls != nil {
		t.Errorf("no days: %v, %v", ls, err)
	}
}

func TestSearch(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	seed(t, s)

	tests := []struct {
		q          string
		withHidden bool
		want       []string
	}{
		{"", false, []string{"f.com", "d.com", "b.com", "a.com"}},
		{"", true, []string{"f.com", "e.com", "d.com", "c.com", "b.com", "a.com"}},
		{"tag:go", false, []string{"d.com", "b.com"}},
		{"tag:go", true, []string{"d.com", "c.com", "b.com"}},
		{"tag:go tag:development", false, []string{"d.com"}},
		{"tag:Go", false, nil}, // tags match exactly, as before
		{`tag:"neural network"`, false, []string{"a.com"}},
		{"мережі", false, []string{"a.com"}},
		{"МЕРЕЖ", false, []string{"a.com"}}, // prefix, case-insensitive
		{"golang", false, []string{"d.com"}},
		{"gol", false, []string{"d.com"}},
		{"network", false, []string{"a.com"}}, // tags are in the text index too
		{"d.com", false, []string{"d.com"}},   // and the URL
		{`"same day"`, false, []string{"b.com"}},
		{`"day same"`, false, nil},
		{"hidden", false, nil},
		{"hidden", true, []string{"e.com", "c.com"}},
		{"date:2016", true, []string{"c.com", "b.com", "a.com"}},
		{"date:2016-08", false, []string{"b.com", "a.com"}},
		{"date:2016-08-19 date:2021", false, []string{"f.com", "b.com", "a.com"}},
		{"date:2017 tag:go golang", false, []string{"d.com"}},
		{"- ...", false, nil},
	}
	for _, tt := range tests {
		q, err := links.ParseQuery(tt.q)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", tt.q, err)
		}
		ls, total, err := s.Search(ctx, q, 0, 50, tt.withHidden)
		if err != nil {
			t.Errorf("Search(%q): %v", tt.q, err)
			continue
		}
		if !reflect.DeepEqual(urls(ls), tt.want) || total != len(tt.want) {
			t.Errorf("Search(%q, hidden=%v) = %v (total %d), want %v", tt.q, tt.withHidden, urls(ls), total, tt.want)
		}
	}
}

func TestSearchPaging(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	seed(t, s)

	ls, total, err := s.Search(ctx, links.Query{}, 2, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b.com", "a.com"}; !reflect.DeepEqual(urls(ls), want) || total != 4 {
		t.Errorf("page 2 = %v (total %d), want %v", urls(ls), total, want)
	}
	ls, total, _ = s.Search(ctx, links.Query{}, 4, 2, false)
	if ls != nil || total != 4 {
		t.Errorf("past the end = %v (total %d)", urls(ls), total)
	}
}

func TestTagNames(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	names, err := s.TagNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"go", "development", "neural network"}; !reflect.DeepEqual(names, want) {
		t.Errorf("TagNames = %v; want %v", names, want)
	}
}
