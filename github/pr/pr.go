package pr

import (
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"

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

type UpdateFile struct {
	Source      string
	Destination string
}

func PublishChanges(accessToken, owner, repo string, updates []UpdateFile) (string, error) {
	for _, update := range updates {
		if _, err := os.Stat(update.Source); err != nil {
			return "", err
		}
	}

	dir, err := ioutil.TempDir("", fmt.Sprintf("gh-%s-%s", owner, repo))
	if err != nil {
		return "", err
	}

	cmd := exec.Command("git", "clone", fmt.Sprintf("git://github.com/%s/%s.git", owner, repo), dir, "--depth", "1")
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	branch := fmt.Sprintf("greenkeep/%x", md5.Sum([]byte(time.Now().String())))

	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return "", err
	}

	for _, update := range updates {
		if err := os.Rename(update.Source, fmt.Sprintf("%s/%s", dir, update.Destination)); err != nil {
			return "", err
		}
		cmd = exec.Command("git", "add", fmt.Sprintf("%s/%s", dir, update.Destination))
		cmd.Dir = dir
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	log.Printf("Creating commit\n")
	cmd = exec.Command("git", "commit", "-m", "'update dependencies'")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return "", err
	}

	log.Printf("Pushing to remote\n")
	cmd = exec.Command("git", "push", "-f")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return "", err
	}

	cmd = exec.Command("git", "checkout", "master")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return branch, nil
}
