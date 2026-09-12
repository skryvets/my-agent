package tools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchReturnsTheStatusAndTheBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		io.WriteString(w, "the body")
	}))
	defer server.Close()

	fetch := Fetch{HTTP: server.Client()}
	out, err := fetch.Call(context.Background(), []byte(`{"url":"`+server.URL+`"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "200 OK") || !strings.Contains(out, "the body") {
		t.Errorf("out = %q", out)
	}
	if fetch.Name() != "fetch" || fetch.Description() == "" {
		t.Errorf("name = %q", fetch.Name())
	}
	if properties, _ := fetch.Parameters()["properties"].(map[string]any); properties["url"] == nil {
		t.Errorf("parameters = %#v", fetch.Parameters())
	}
}

func TestFetchCutsALongBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Repeat("a", outputLimit*2))
	}))
	defer server.Close()

	out, err := Fetch{HTTP: server.Client()}.Call(context.Background(), []byte(`{"url":"`+server.URL+`"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(out, "bytes cut") {
		t.Error("a long body was not cut")
	}
}

func TestFetchReportsItsErrors(t *testing.T) {
	cases := map[string]string{
		"bad arguments": `{oops`,
		"empty url":     `{}`,
		"bad url":       `{"url":"http://%zz"}`,
		"no server":     `{"url":"http://127.0.0.1:1"}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := (Fetch{}).Call(context.Background(), []byte(args)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
