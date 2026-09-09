package telegram

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitMessage(t *testing.T) {
	cases := map[string]struct {
		text  string
		limit int
		want  []string
	}{
		"short":                {"hello", 10, []string{"hello"}},
		"empty":                {"", 10, []string{""}},
		"splits on blank":      {"one\n\ntwo", 5, []string{"one", "two"}},
		"splits on space":      {"one two three", 8, []string{"one two", "three"}},
		"no separator":         {"abcdefgh", 4, []string{"abcd", "efgh"}},
		"keeps runes together": {strings.Repeat("é", 5), 2, []string{"éé", "éé", "é"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := splitMessage(tc.text, tc.limit)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			for _, part := range got {
				if len([]rune(part)) > tc.limit {
					t.Errorf("part %q exceeds limit %d", part, tc.limit)
				}
			}
		})
	}
}
