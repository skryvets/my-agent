package task

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRepoReadsEveryShape(t *testing.T) {
	want := Repo{Owner: "skryvets", Name: "my-agent"}
	for _, text := range []string{
		"skryvets/my-agent",
		"  skryvets/my-agent  ",
		"https://github.com/skryvets/my-agent",
		"https://github.com/skryvets/my-agent.git",
		"git@github.com:skryvets/my-agent.git",
	} {
		got, err := ParseRepo(text)
		if err != nil {
			t.Errorf("ParseRepo(%q): %v", text, err)
			continue
		}
		if got != want {
			t.Errorf("ParseRepo(%q) = %#v", text, got)
		}
	}
	if got := want.String(); got != "skryvets/my-agent" {
		t.Errorf("String = %q", got)
	}
}

func TestParseRepoRefusesWhatIsNotOne(t *testing.T) {
	for _, text := range []string{"", "my-agent", "a/b/c", "/my-agent", "skryvets/", "skryvets/my-agent\n\nI", "skryvets/my agent"} {
		if _, err := ParseRepo(text); err == nil {
			t.Errorf("ParseRepo(%q) was accepted", text)
		}
	}
}

func TestGitHubOpensAPullRequest(t *testing.T) {
	var body map[string]any
	var headers http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		switch r.URL.Path {
		case "/repos/skryvets/my-agent":
			io.WriteString(w, `{"default_branch":"main"}`)
		case "/repos/skryvets/my-agent/pulls":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"html_url":"https://github.com/skryvets/my-agent/pull/7"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	github := GitHub{Token: "gh-token", BaseURL: server.URL, HTTP: server.Client()}
	repo := Repo{Owner: "skryvets", Name: "my-agent"}
	ctx := context.Background()

	base, err := github.DefaultBranch(ctx, repo)
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if base != "main" {
		t.Errorf("base = %q", base)
	}

	url, err := github.OpenPullRequest(ctx, repo, "Fix the lint warning", "my-agent/1", base, "why")
	if err != nil {
		t.Fatalf("OpenPullRequest: %v", err)
	}
	if url != "https://github.com/skryvets/my-agent/pull/7" {
		t.Errorf("url = %q", url)
	}
	if body["title"] != "Fix the lint warning" || body["head"] != "my-agent/1" || body["base"] != "main" {
		t.Errorf("body = %#v", body)
	}
	if headers.Get("Authorization") != "Bearer gh-token" {
		t.Errorf("Authorization = %q", headers.Get("Authorization"))
	}
	if headers.Get("Accept") != "application/vnd.github.v3+json" {
		t.Errorf("Accept = %q", headers.Get("Accept"))
	}
	if headers.Get("X-GitHub-Api-Version") != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q", headers.Get("X-GitHub-Api-Version"))
	}
}

func TestGitHubReportsWhatItWasTold(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/a/b":
			io.WriteString(w, `{"default_branch":""}`)
		case "/repos/a/c/pulls":
			w.WriteHeader(http.StatusUnprocessableEntity)
			io.WriteString(w, `{"message":"Validation Failed","errors":[{"message":"No commits between main and my-agent/1"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, "not json")
		}
	}))
	defer server.Close()

	github := GitHub{BaseURL: server.URL, HTTP: server.Client()}
	ctx := context.Background()

	if _, err := github.DefaultBranch(ctx, Repo{Owner: "a", Name: "b"}); err == nil {
		t.Error("expected an error for a repository with no default branch")
	}

	_, err := github.OpenPullRequest(ctx, Repo{Owner: "a", Name: "c"}, "t", "h", "b", "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "No commits between") {
		t.Errorf("err = %v, want the detail github gave", err)
	}

	if _, err := github.DefaultBranch(ctx, Repo{Owner: "x", Name: "y"}); err == nil {
		t.Error("expected an error for an answer that is not json")
	}
}

func TestGitHubUsesThePublicAPIWhenTheRootIsEmpty(t *testing.T) {
	var got *http.Request
	github := GitHub{
		Token: "gh-token",
		HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			got = r
			return nil, context.Canceled
		})},
	}
	_, err := github.DefaultBranch(context.Background(), Repo{Owner: "a", Name: "b"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got == nil {
		t.Fatal("no request")
	}
	if got.URL.String() != "https://api.github.com/repos/a/b" {
		t.Errorf("url = %q", got.URL)
	}
	if got.Header.Get("Authorization") != "Bearer gh-token" {
		t.Errorf("Authorization = %q", got.Header.Get("Authorization"))
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (fn roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestGitHubReportsAServerThatIsNotThere(t *testing.T) {
	github := GitHub{BaseURL: "http://127.0.0.1:1"}
	if _, err := github.DefaultBranch(context.Background(), Repo{Owner: "a", Name: "b"}); err == nil {
		t.Fatal("expected an error")
	}
	github.BaseURL = "http://%zz"
	if _, err := github.DefaultBranch(context.Background(), Repo{Owner: "a", Name: "b"}); err == nil {
		t.Fatal("expected an error for an address that cannot be read")
	}
	if _, err := github.OpenPullRequest(context.Background(), Repo{Owner: "a", Name: "b"}, "t", "h", "b", ""); err == nil {
		t.Fatal("expected an error for an address that cannot be read")
	}
}
