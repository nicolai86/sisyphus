package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/oauth2"

	"github.com/google/go-github/github"
	git "github.com/libgit2/git2go"
	"github.com/nicolai86/greenkeepr/storage"
)

var (
	configPath string
	buildPath  string
	language   string
)

func init() {
	flag.StringVar(&buildPath, "build-path", "", "directory to run builds in")
	flag.StringVar(&configPath, "config-path", "", "path to config.json")
	flag.StringVar(&language, "language", "", "language to use [javascript,ruby]")
	flag.Parse()
}

func repoFrom(r storage.Repository) *git.Repository {
	cloneOptions := &git.CloneOptions{
		Bare:           false,
		CheckoutBranch: "master",
	}
	cachePath := fmt.Sprintf("/tmp/%s", r.ID)
	if _, err := os.Stat(cachePath); err != nil {
		repo, err := git.Clone(r.GitURL, cachePath, cloneOptions)
		if err != nil {
			log.Panic(err)
		}
		return repo
	}

	repo, err := git.OpenRepository(cachePath)
	if err != nil {
		log.Fatal(err)
	}

	remote, err := repo.Remotes.Lookup("origin")
	if err != nil {
		log.Fatal(err)
	}

	if err := remote.Fetch([]string{}, nil, ""); err != nil {
		log.Fatal(err)
	}

	return repo
}

func credentialsCallback(url string, username string, allowedTypes git.CredType) (git.ErrorCode, *git.Cred) {
	ret, cred := git.NewCredSshKeyFromAgent(username)
	return git.ErrorCode(ret), &cred
}

func certificateCheckCallback(cert *git.Certificate, valid bool, hostname string) git.ErrorCode {
	if hostname != "github.com" {
		return git.ErrUser
	}
	return git.ErrOk
}

type config struct {
	Path     string
	Language string
}

func stringPtr(str string) *string {
	return &str
}

func main() {
	if language != "javascript" {
		log.Fatalf("Only JS is supported at the moment")
	}

	f, _ := os.Open(configPath)
	var r storage.Repository
	json.NewDecoder(f).Decode(&r)

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: r.AccessToken},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)

	owner := strings.Split(r.FullName, "/")[0]
	repo := strings.Split(r.FullName, "/")[1]

	updatedPackage := fmt.Sprintf("%s/package.new.json", buildPath)
	if _, err := os.Stat(updatedPackage); err != nil {
		log.Fatalf("package.new.json missing. Did the dependency worker run?")
	}

	var c config
	cf, _ := os.Open(fmt.Sprintf("%s/config.json", buildPath))
	json.NewDecoder(cf).Decode(&c)

	prs, _, err := client.PullRequests.List(owner, repo, nil)
	log.Printf("%#v, %#v", prs, err)
	// TODO get PRs for repo

	cmd := exec.Command("git", "checkout", "-b", "greenkeep/1")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	os.Rename(updatedPackage, fmt.Sprintf("/tmp/%s/%s/package.json", r.ID, c.Path))

	cmd = exec.Command("git", "add", fmt.Sprintf("/tmp/%s/%s/package.json", r.ID, c.Path))
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	cmd = exec.Command("git", "commit", "-m", "'update js dependencies'")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	cmd = exec.Command("git", "push")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	client.PullRequests.Create(owner, repo, &github.NewPullRequest{
		Title: stringPtr("Update your JS dependencies"),
		Head:  stringPtr("greenkeep/1"),
		Base:  stringPtr("master"),
		Body:  stringPtr("See diff, ……"),
	})

	cmd = exec.Command("git", "checkout", "master")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
