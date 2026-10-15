package web

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// testdata/pagination_v1.json is the output of pagination_dict() from the
// Python version (tag v1.0.0-flask-mongo) for every page of 1..15 pages,
// with center_side_count=2 and url_prefix='/x'.
func TestPaginationMatchesV1(t *testing.T) {
	data, err := os.ReadFile("testdata/pagination_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Page, Pages int
		Items       [][3]string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	urlFor := func(i int) string {
		if i == 0 {
			return "/x"
		}
		return fmt.Sprintf("/x?p=%d", i+1)
	}
	for _, c := range cases {
		var want []pageLink
		for _, it := range c.Items {
			want = append(want, pageLink{Title: it[0], URL: it[1], Class: it[2]})
		}
		if got := pagination(c.Page, c.Pages, 2, urlFor); !reflect.DeepEqual(got, want) {
			t.Errorf("page %d of %d:\n got %v\nwant %v", c.Page, c.Pages, got, want)
		}
	}
	if len(cases) != 120 {
		t.Errorf("fixture has %d cases, want 120", len(cases))
	}
}
