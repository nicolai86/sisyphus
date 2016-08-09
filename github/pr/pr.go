package pr

import (
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// PullRequestExists wraps a simple loop to validate that a PR exists which fulfills
// some arbitrary requirement
func PullRequestExists(accessToken, owner, repo string, exists func(*github.PullRequest) bool) bool {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)

	prs, _, _ := client.PullRequests.List(owner, repo, nil)

	for _, pr := range prs {
		if exists(pr) {
			return true
		}
	}

	return false
}

func stringPtr(str string) *string {
	return &str
}

func CreatePullRequest(accessToken, owner, repo, title, branch, body string) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)

	client.PullRequests.Create(owner, repo, &github.NewPullRequest{
		Title: stringPtr(title),
		Head:  stringPtr(branch),
		Base:  stringPtr("master"),
		Body:  stringPtr(body),
	})
}
