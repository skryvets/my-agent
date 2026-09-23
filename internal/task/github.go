package task

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/google/go-github/v92/github"
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
	if !found || owner == "" || name == "" || strings.Contains(name, "/") || strings.ContainsFunc(trimmed, unicode.IsSpace) {
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
	client, err := g.client()
	if err != nil {
		return "", err
	}
	remote, _, err := client.Repositories.Get(ctx, repo.Owner, repo.Name)
	if err != nil {
		return "", err
	}
	if remote.GetDefaultBranch() == "" {
		return "", fmt.Errorf("%s names no default branch", repo)
	}
	return remote.GetDefaultBranch(), nil
}

// OpenPullRequest opens one pull request and returns its web address.
func (g GitHub) OpenPullRequest(ctx context.Context, repo Repo, title, head, base, body string) (string, error) {
	client, err := g.client()
	if err != nil {
		return "", err
	}
	pull, _, err := client.PullRequests.Create(ctx, repo.Owner, repo.Name, github.CreatePullRequest{
		Title: github.Ptr(title),
		Head:  head,
		Base:  base,
		Body:  github.Ptr(body),
	})
	if err != nil {
		return "", err
	}
	return pull.GetHTMLURL(), nil
}

// client builds a go-github client. BaseURL is the API root, so WithURLs is
// used rather than WithEnterpriseURLs, which would append /api/v3/.
func (g GitHub) client() (*github.Client, error) {
	var opts []github.ClientOptionsFunc
	if g.HTTP != nil {
		opts = append(opts, github.WithHTTPClient(g.HTTP))
	}
	if g.Token != "" {
		opts = append(opts, github.WithAuthToken(g.Token))
	}
	if g.BaseURL != "" {
		base := g.BaseURL
		opts = append(opts, github.WithURLs(&base, nil))
	}
	return github.NewClient(opts...)
}
