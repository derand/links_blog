package links

import (
	"reflect"
	"testing"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		in   string
		want Query
	}{
		{"", Query{}},
		{"   ", Query{}},
		{`tag:"neural network" яндекс`, Query{Tags: []string{"neural network"}, Terms: []string{"яндекс"}}},
		{`"tag:neural network"`, Query{Tags: []string{"neural network"}}},
		{`TAG:Go Date:2016-09 foo "two words"`, Query{Tags: []string{"Go"}, Dates: []string{"2016-09"}, Terms: []string{"foo", "two words"}}},
		{"date:2016 date:2017-01-02", Query{Dates: []string{"2016", "2017-01-02"}}},
		{"it's", Query{Terms: []string{"its"}}},
		{`'single quoted' x`, Query{Terms: []string{"single quoted", "x"}}},
		{"tag: foo", Query{Terms: []string{"foo"}}},
	}
	for _, tt := range tests {
		got, err := ParseQuery(tt.in)
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseQuery(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseQueryErrors(t *testing.T) {
	for _, in := range []string{`a "b c`, "date:16-09", "date:2016-9", "date:yesterday"} {
		if got, err := ParseQuery(in); err == nil {
			t.Errorf("ParseQuery(%q) = %+v, want error", in, got)
		}
	}
}
