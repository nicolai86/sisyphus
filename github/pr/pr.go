package pr

import (
	"crypto/md5"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/google/go-github/github"
	"github.com/nicolai86/sisyphus/github/repo"
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

type UpdateFile struct {
	Source      string
	Destination string
}

func PublishChanges(accessToken, owner, repoName string, updates []UpdateFile) (string, error) {
	for _, update := range updates {
		if _, err := os.Stat(update.Source); err != nil {
			return "", err
		}
	}

	dir, _ := repo.Clone(accessToken, owner, repoName)
	branch := fmt.Sprintf("greenkeep/%x", md5.Sum([]byte(time.Now().String())))
	if err := func() error {
		cmds := [][]string{
			[]string{"git", "checkout", "-b", branch},
		}
		for _, args := range cmds {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = os.Environ()
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		}
		return nil
	}(); err != nil {
		return "", err
	}

	for _, update := range updates {
		if err := os.Rename(update.Source, fmt.Sprintf("%s/%s", dir, update.Destination)); err != nil {
			return "", err
		}
		cmd := exec.Command("git", "add", fmt.Sprintf("%s/%s", dir, update.Destination))
		cmd.Dir = dir
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	if err := func() error {
		cmds := [][]string{
			[]string{"git", "commit", "-m", "'update dependencies'"},
			[]string{"git", "push", "-f", fmt.Sprintf("https://%s@github.com/%s/%s.git", accessToken, owner, repoName)},
			[]string{"git", "checkout", "master"},
		}
		for _, args := range cmds {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = os.Environ()
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		}
		return nil
	}(); err != nil {
		return "", err
	}

	return branch, nil
}
