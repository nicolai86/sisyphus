package pr

import (
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

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
