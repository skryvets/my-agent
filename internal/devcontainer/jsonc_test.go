package devcontainer

import (
	"encoding/json"
	"testing"
)

func TestStandardizeKeepsWhatIsInsideAString(t *testing.T) {
	input := `{
		"url": "https://example.com/a//b", // a line comment
		/* a block
		   comment */ "quote": "say \"hi\", // not a comment",
		"list": [1, 2, ],
		"end": "/* kept */",
	}`
	var got map[string]any
	if err := json.Unmarshal(standardize([]byte(input)), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, standardize([]byte(input)))
	}
	if got["url"] != "https://example.com/a//b" {
		t.Errorf("url = %#v", got["url"])
	}
	if got["quote"] != `say "hi", // not a comment` {
		t.Errorf("quote = %#v", got["quote"])
	}
	if got["end"] != "/* kept */" {
		t.Errorf("end = %#v", got["end"])
	}
	if list, _ := got["list"].([]any); len(list) != 2 {
		t.Errorf("list = %#v", got["list"])
	}
}

func TestStandardizeSurvivesAnUnclosedCommentOrString(t *testing.T) {
	if got := string(standardize([]byte(`{"a": 1} /* open`))); got != `{"a": 1} ` {
		t.Errorf("got %q", got)
	}
	if got := string(standardize([]byte(`{"a": "open`))); got != `{"a": "open` {
		t.Errorf("got %q", got)
	}
	if got := string(standardize([]byte(`{"a": 1} // end`))); got != `{"a": 1} ` {
		t.Errorf("got %q", got)
	}
	if got := string(standardize([]byte(`[1,`))); got != `[1,` {
		t.Errorf("got %q", got)
	}
}
