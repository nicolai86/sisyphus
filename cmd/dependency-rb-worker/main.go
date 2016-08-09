package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"
	"github.com/google/go-github/github"
	"github.com/libgit2/git2go"
	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/github/pr"
	"github.com/nicolai86/sisyphus/storage"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

var (
	dataPath    string
	natsURL     string
	fileStorage storage.RepositoryReader
	nc          *nats.Conn
)

type config struct {
	Path     string
	Language string
}

type repoConfig struct {
	Config       config
	RepositoryID string
}

func init() {
	flag.StringVar(&dataPath, "data-path", "", "data directory")
	flag.StringVar(&natsURL, "nats", "tcp://127.0.0.1:4222", "nats server URL")
	flag.Parse()

	fileStorage = storage.NewFileStorage(dataPath)
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

func fileContent(g *git.Repository, path string) ([]byte, error) {
	head, err := g.References.Lookup("refs/remotes/origin/master")
	if err != nil {
		return nil, err
	}

	commit, err := g.LookupCommit(head.Target())
	if err != nil {
		return nil, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	t, err := tree.EntryByPath(path)
	if err != nil {
		return nil, err
	}

	if t.Filemode != git.FilemodeBlob {
		return nil, fmt.Errorf("Not a blob")
	}
	blob, err := g.LookupBlob(t.Id)
	if err != nil {
		return nil, err
	}

	return blob.Contents(), nil
}

var filesToExtract = []string{"Gemfile", "Gemfile.lock"}

func checkDependencies(r storage.Repository, c config) {
	log.Printf("looking for %q (%q): %q", c.Path, c.Language, filesToExtract)

	repo := repoFrom(r)
	data := []byte(fmt.Sprintf("%s-%s", c.Path, c.Language))
	cachePath := fmt.Sprintf("/tmp/build/%s/%x", r.ID, md5.Sum(data))
	os.MkdirAll(cachePath, 0700)
	for _, file := range filesToExtract {
		content, err := fileContent(repo, fmt.Sprintf("%s/%s", c.Path, file))
		if err != nil {
			panic(err)
		}

		log.Printf("build %q\n", cachePath)
		f, err := os.OpenFile(fmt.Sprintf("%s/%s", cachePath, file), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
		f.Write(content)

	}

	runDependencyCheck(r, c, cachePath)
}

func runDependencyCheck(r storage.Repository, c config, buildPath string) {
	// docker run --rm -v $(pwd)/outdated.log:/home/checker/outdated.log:rw -v $(pwd)/Gemfile:/home/checker/Gemfile:ro -v $(pwd)/Gemfile.lock:/home/checker/Gemfile.lock -it dep-check-rb
	f, _ := os.OpenFile(fmt.Sprintf("%s/outdated.log", buildPath), os.O_CREATE|os.O_TRUNC, 0600)
	f.Close()

	cli, err := client.NewClient("unix:///var/run/docker.sock", "v1.22", nil, nil)
	if err != nil {
		panic(err)
	}

	func() {
		container, err := cli.ContainerCreate(context.Background(), &container.Config{
			Image: "dep-check-rb",
		}, &container.HostConfig{
			AutoRemove: true,
			Binds: []string{
				fmt.Sprintf("%s/outdated.log:/home/checker/outdated.log:rw", buildPath),
				fmt.Sprintf("%s/Gemfile:/home/checker/Gemfile:ro", buildPath),
				fmt.Sprintf("%s/Gemfile.lock:/home/checker/Gemfile.lock:ro", buildPath),
			},
		}, nil, "")
		if err != nil {
			log.Fatalf(err.Error())
		}

		if err := cli.ContainerStart(context.Background(), container.ID); err != nil {
			log.Fatalf(err.Error())
		}

		cli.ContainerWait(context.Background(), container.ID)

		cli.ContainerRemove(context.Background(), types.ContainerRemoveOptions{
			ContainerID: container.ID,
		})
	}()

	f2, _ := os.Open(fmt.Sprintf("%s/outdated.log", buildPath))
	defer f2.Close()
	var dependencies = ParseLog(f2)

	if len(dependencies.Updates) == 0 {
		log.Printf("Nothing to do for %q %q %q", r.ID, c.Path, c.Language)
		return
	}

	f3, _ := os.Open(fmt.Sprintf("%s/Gemfile", buildPath))
	defer f3.Close()
	var b = bytes.Buffer{}
	UpdateGemfile(dependencies, f3, &b)

	var changedDependencies = []string{}
	for dep := range dependencies.Updates {
		changedDependencies = append(changedDependencies, dep)
	}
	if hasPR(r, c, changedDependencies) {
		log.Printf("%s has an open PR for %q\n", r.ID, changedDependencies)
		return
	}

	f4, _ := os.OpenFile(fmt.Sprintf("%s/Gemfile", buildPath), os.O_TRUNC|os.O_WRONLY, 0600)
	defer f4.Close()
	b.WriteTo(f4)

	func() {
		container, err := cli.ContainerCreate(context.Background(), &container.Config{
			Image:      "dep-check-rb",
			Entrypoint: strslice.StrSlice([]string{"bundle", "update"}),
		}, &container.HostConfig{
			AutoRemove: true,
			Binds: []string{
				fmt.Sprintf("%s/Gemfile:/home/checker/Gemfile:rw", buildPath),
				fmt.Sprintf("%s/Gemfile.lock:/home/checker/Gemfile.lock:rw", buildPath),
			},
		}, nil, "")
		if err != nil {
			log.Fatalf(err.Error())
		}

		if err := cli.ContainerStart(context.Background(), container.ID); err != nil {
			log.Fatalf(err.Error())
		}

		cli.ContainerWait(context.Background(), container.ID)

		cli.ContainerRemove(context.Background(), types.ContainerRemoveOptions{
			ContainerID: container.ID,
		})
	}()

	log.Printf("pushing new branch to remote…\n")
	branch := pushChangesToRemote(r, c, buildPath)
	log.Printf("creating PR\n")
	createPR(r, c, branch, changedDependencies)
}

func pushChangesToRemote(r storage.Repository, c config, buildPath string) string {
	updatedGemfile := fmt.Sprintf("%s/Gemfile", buildPath)
	updatedGemfileLock := fmt.Sprintf("%s/Gemfile.lock", buildPath)
	if _, err := os.Stat(updatedGemfile); err != nil {
		log.Fatalf("The updated package.new.json file is missing…\n")
	}
	branch := fmt.Sprintf("greenkeep/%x", md5.Sum([]byte(time.Now().String())))
	log.Printf("Operating branch is %q\n", branch)

	log.Printf("Cleaning current branch\n")
	cmd := exec.Command("git", "clean", "-fd")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("Unable to change branch: %q\n", err)
	}

	log.Printf("Ensuring we're on master\n")
	cmd = exec.Command("git", "checkout", "master")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	cmd = exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatalf("Unable to change branch: %q\n", err)
	}

	log.Printf("Moving new Gemfile into place\n")
	if err := os.Rename(updatedGemfile, fmt.Sprintf("/tmp/%s/%s/Gemfile", r.ID, c.Path)); err != nil {
		log.Fatalf("Unable to move file: %q\n", err)
	}

	log.Printf("Adding Gemfile to stage\n")
	cmd = exec.Command("git", "add", fmt.Sprintf("/tmp/%s/%s/Gemfile", r.ID, c.Path))
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	log.Printf("Moving new Gemfile into place\n")
	if err := os.Rename(updatedGemfileLock, fmt.Sprintf("/tmp/%s/%s/Gemfile.lock", r.ID, c.Path)); err != nil {
		log.Fatalf("Unable to move file: %q\n", err)
	}

	log.Printf("Adding Gemfile to stage\n")
	cmd = exec.Command("git", "add", fmt.Sprintf("/tmp/%s/%s/Gemfile.lock", r.ID, c.Path))
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	log.Printf("Creating commit\n")
	cmd = exec.Command("git", "commit", "-m", "'update rb dependencies'")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	log.Printf("Pushing to remote\n")
	cmd = exec.Command("git", "push", "-f")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	log.Printf("Reverting to master\n")
	cmd = exec.Command("git", "checkout", "master")
	cmd.Dir = fmt.Sprintf("/tmp/%s", r.ID)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	return branch
}

func hasPR(r storage.Repository, c config, modifications []string) bool {
	owner := strings.Split(r.FullName, "/")[0]
	repo := strings.Split(r.FullName, "/")[1]
	return pr.PullRequestExists(r.AccessToken, owner, repo, func(pr *github.PullRequest) bool {
		index := strings.Index(*pr.Body, fmt.Sprintf("```\n# %s dependencies in %s\n", c.Language, c.Path))
		if index == -1 {
			return false
		}

		parts := strings.Split(strings.Split(*pr.Body, fmt.Sprintf("```\n# %s dependencies in %s\n", c.Language, c.Path))[1], "```")[0]
		for _, mod := range modifications {
			if strings.Index(parts, mod) != -1 {
				return true
			}
		}

		return false
	})
}

func stringPtr(str string) *string {
	return &str
}

func createPR(r storage.Repository, c config, branch string, modifications []string) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: r.AccessToken},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)

	owner := strings.Split(r.FullName, "/")[0]
	repo := strings.Split(r.FullName, "/")[1]

	out, _ := json.MarshalIndent(modifications, "", "\t")

	client.PullRequests.Create(owner, repo, &github.NewPullRequest{
		Title: stringPtr("Update your RB dependencies"),
		Head:  stringPtr(branch),
		Base:  stringPtr("master"),
		Body: stringPtr(
			fmt.Sprintf(
				`This PR updates dependencies, which have not been covered by your versions so far: %s`,
				fmt.Sprintf("\n\n ```\n# %s dependencies in %s\n%s\n```", c.Language, c.Path, out),
			),
		),
	})
}

func main() {
	log.Printf("greenkeepr dependency worker for ruby running")

	nc1, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc1.Close()
	nc = nc1

	nc.Subscribe("greenkeep-ruby", func(msg *nats.Msg) {
		repos, err := fileStorage.Load()
		if err != nil {
			log.Fatalf("Failed to read repo storages: %q\n", err)
		}

		var rc repoConfig
		if err := json.NewDecoder(bytes.NewBuffer(msg.Data)).Decode(&rc); err != nil {
			log.Fatal(err)
		}
		log.Printf("received request for %q\n", rc.RepositoryID)

		var r storage.Repository
		for _, repo := range repos {
			if repo.ID == rc.RepositoryID {
				r = repo
				break
			}
		}

		configPath := fmt.Sprintf("%s/%s/%s.json", dataPath, "greenkeep", r.ID)
		if _, err := os.Stat(configPath); err != nil {
			log.Printf("%q does not exist. skipping", configPath)
			return
		}

		go checkDependencies(r, rc.Config)
	})
	nc.Flush()

	select {}
}
