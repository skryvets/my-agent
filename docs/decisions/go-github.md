# Decisions - the GitHub calls move to google/go-github

## Why a library at all
**Q:** `github.go` hand-rolled the REST call, the API version header, bearer auth and the error parse. Why does it move now?
**A:** The user asked to replace that client with `google/go-github`, the same cut as the Docker client: stop owning the transport. The two calls stay `DefaultBranch` and `OpenPullRequest`. Callers, including `main.go`, keep the `GitHub` struct.
**Source:** user (2026-09-21)

## Which version
**Q:** Which module version?
**A:** [github.com/google/go-github/v89](https://pkg.go.dev/github.com/google/go-github/v89/github) `v89.0.0`, the version named in the request. The latest release on 2026-09-21 is `v92.0.0`. The v92 notes do not change `Repositories.Get` or `PullRequests.Create`. `v89` is pinned because that is the version the request named. The module brings `github.com/google/go-querystring` as an indirect dependency.
**Source:** user (2026-09-21)

## The API version
**Q:** Which REST version does the client send?
**A:** The library default, `2022-11-28`, set as `apiVersionDefault` in `NewClient`. That is the version the hand-rolled client sent in `X-GitHub-Api-Version`. No per-request override.
**Source:** assumed default (2026-09-21)
**Docs:** https://raw.githubusercontent.com/google/go-github/v89.0.0/github/github.go (`api20221128`, `NewClient`)

## The two calls
**Q:** Which methods replace the hand-rolled paths?
**A:** `Repositories.Get` for the default branch, `PullRequests.Create` with `NewPullRequest` for the pull request. The web address is `PullRequest.GetHTMLURL`. An empty default branch is still an error, as before.
**Source:** assumed default (2026-09-21)
**Docs:** https://docs.github.com/rest/repos/repos?apiVersion=2022-11-28#get-a-repository and https://docs.github.com/rest/pulls/pulls?apiVersion=2022-11-28#create-a-pull-request, as cited on the methods in https://raw.githubusercontent.com/google/go-github/v89.0.0/github/repos.go and https://raw.githubusercontent.com/google/go-github/v89.0.0/github/pulls.go

## The API root
**Q:** How does a test still point the client at an `httptest` server?
**A:** `BaseURL` stays the API root and is passed to `WithURLs`. `WithEnterpriseURLs` is not used, because it appends `/api/v3/` when the host is not `api.`, and the tests expect `/repos/...`. An empty `BaseURL` leaves the library default, `https://api.github.com/`.
**Source:** assumed default (2026-09-21)
**Docs:** https://raw.githubusercontent.com/google/go-github/v89.0.0/github/github.go (`WithURLs`, `WithEnterpriseURLs`) and https://raw.githubusercontent.com/google/go-github/v89.0.0/github/url.go (`parseURL`)

## Errors and rate limits
**Q:** Do we still format `github METHOD path: status: message`, and do we retry a rate limit?
**A:** No format of our own, and no retry. The library error already carries the message GitHub sent, including a validation detail such as `No commits between`. A primary rate limit comes back as `*github.RateLimitError`, a secondary one as `*github.AbuseRateLimitError`. The client also skips a call when it already knows the primary limit is exhausted. That is the handling the library gives. This agent does not sleep and try again.
**Source:** assumed default (2026-09-21)
**Docs:** https://raw.githubusercontent.com/google/go-github/v89.0.0/github/github.go (`CheckResponse`, `RateLimitError`, `checkRateLimitBeforeDo`)

## The Accept header
**Q:** The old test required `Accept: application/vnd.github+json`. The library sends `application/vnd.github.v3+json` on `PullRequests.Create`. Change the test?
**A:** Yes. The header belongs to the library now. The test still checks that a media type, the bearer token and `2022-11-28` go out. `Repositories.Get` replaces `Accept` with preview media types of its own. The product behavior, the branch and the pull request URL, does not change.
**Source:** assumed default (2026-09-21)
**Docs:** https://raw.githubusercontent.com/google/go-github/v89.0.0/github/github.go (`mediaTypeV3`, `NewRequest`) and https://raw.githubusercontent.com/google/go-github/v89.0.0/github/pulls.go (`Create`)
