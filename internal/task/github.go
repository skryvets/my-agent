package task

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	githubAPI = "https://api.github.com"

	// apiVersion is the REST version this code was written against.
	apiVersion = "2022-11-28"
)

// Repo names one repository on GitHub.
type Repo struct {
	Owner string
	Name  string
}

func (r Repo) String() string { return r.Owner + "/" + r.Name }

// ParseRepo reads owner/name, a full URL, or a git address.
func ParseRepo(text string) (Repo, error) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(text), ".git")
	trimmed = strings.TrimPrefix(trimmed, "https://github.com/")
	trimmed = strings.TrimPrefix(trimmed, "git@github.com:")
	trimmed = strings.Trim(trimmed, "/")

	owner, name, found := strings.Cut(trimmed, "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return Repo{}, fmt.Errorf("%q is not a repository, write owner/name", text)
	}
	return Repo{Owner: owner, Name: name}, nil
}

// GitHub is the slice of the REST API this agent uses.
type GitHub struct {
	Token string
	// BaseURL is the API root. An empty BaseURL means github.com.
	BaseURL string
	HTTP    *http.Client
}

// DefaultBranch is the branch a pull request is opened against.
func (g GitHub) DefaultBranch(ctx context.Context, repo Repo) (string, error) {
	var answer struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := g.call(ctx, http.MethodGet, "/repos/"+repo.String(), nil, &answer); err != nil {
		return "", err
	}
	if answer.DefaultBranch == "" {
		return "", fmt.Errorf("%s names no default branch", repo)
	}
	return answer.DefaultBranch, nil
}

// OpenPullRequest opens one pull request and returns its web address.
func (g GitHub) OpenPullRequest(ctx context.Context, repo Repo, title, head, base, body string) (string, error) {
	request := map[string]any{"title": title, "head": head, "base": base, "body": body}

	var answer struct {
		URL string `json:"html_url"`
	}
	if err := g.call(ctx, http.MethodPost, "/repos/"+repo.String()+"/pulls", request, &answer); err != nil {
		return "", err
	}
	return answer.URL, nil
}

func (g GitHub) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	root := g.BaseURL
	if root == "" {
		root = githubAPI
	}
	req, err := http.NewRequestWithContext(ctx, method, root+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}

	client := g.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return githubError(method, path, resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func githubError(method, path string, resp *http.Response) error {
	var answer struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err := json.Unmarshal(data, &answer); err != nil || answer.Message == "" {
		return fmt.Errorf("github %s %s: %s", method, path, resp.Status)
	}

	message := answer.Message
	for _, detail := range answer.Errors {
		if detail.Message != "" {
			message += ": " + detail.Message
		}
	}
	return fmt.Errorf("github %s %s: %s: %s", method, path, resp.Status, message)
}
