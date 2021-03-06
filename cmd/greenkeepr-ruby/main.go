package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/strslice"
	"github.com/google/go-github/github"
	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/github/pr"
	"github.com/nicolai86/sisyphus/storage"
	"golang.org/x/net/context"
)

var (
	natsURL     string
	fileStorage storage.RepositoryReaderWriter
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
	var (
		dataPath      string
		bucket        string
		encryptionKey string
	)
	flag.StringVar(&bucket, "s3-bucket", "", "s3 storage bucket")
	flag.StringVar(&dataPath, "data-path", "", "data directory")
	flag.StringVar(&encryptionKey, "encryption-key", "", "store everything encrypted")
	flag.StringVar(&natsURL, "nats", "tcp://127.0.0.1:4222", "nats server URL")
	flag.Parse()

	if dataPath != "" {
		fileStorage = storage.NewFileStorage(dataPath)
	}
	if bucket != "" {
		fileStorage = storage.NewS3Storage(bucket)
	}
	if encryptionKey != "" {
		fileStorage = storage.NewAESStorage(encryptionKey, fileStorage)
	}
}

var filesToExtract = []string{"Gemfile", "Gemfile.lock"}

func checkDependencies(r storage.Repository, c config) {
	log.Printf("looking for %q (%q): %q", c.Path, c.Language, filesToExtract)

	owner := strings.Split(r.FullName, "/")[0]
	repoName := strings.Split(r.FullName, "/")[1]

	data := []byte(fmt.Sprintf("%s-%s", c.Path, c.Language))
	cachePath := fmt.Sprintf("/tmp/build/%s/%x", r.ID, md5.Sum(data))
	os.MkdirAll(cachePath, 0700)
	for _, file := range filesToExtract {
		uri := fmt.Sprintf("https://%s@raw.githubusercontent.com/%s/%s/master/%s/%s", r.AccessToken, owner, repoName, c.Path, file)
		resp, _ := http.Get(uri)
		f, _ := os.OpenFile(fmt.Sprintf("%s/%s", cachePath, file), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		io.Copy(f, resp.Body)
		resp.Body.Close()
	}

	runDependencyCheck(r, c, cachePath)
}

func runDependencyCheck(r storage.Repository, c config, buildPath string) {
	// docker run --rm -v $(pwd)/outdated.log:/home/checker/outdated.log:rw -v $(pwd)/Gemfile:/home/checker/Gemfile:ro -v $(pwd)/Gemfile.lock:/home/checker/Gemfile.lock -it dep-check-rb
	f, _ := os.OpenFile(fmt.Sprintf("%s/outdated.log", buildPath), os.O_CREATE|os.O_TRUNC, 0600)
	f.Close()

	cli, err := client.NewEnvClient()
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
	owner := strings.Split(r.FullName, "/")[0]
	repo := strings.Split(r.FullName, "/")[1]
	branch, err := pr.PublishChanges(r.AccessToken, owner, repo, []pr.UpdateFile{
		pr.UpdateFile{
			Source:      fmt.Sprintf("%s/Gemfile", buildPath),
			Destination: fmt.Sprintf("%s/Gemfile", c.Path),
		},
		pr.UpdateFile{
			Source:      fmt.Sprintf("%s/Gemfile.lock", buildPath),
			Destination: fmt.Sprintf("%s/Gemfile.lock", c.Path),
		},
	})
	if err != nil {
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

func createPR(r storage.Repository, c config, branch string, modifications []string) {
	owner := strings.Split(r.FullName, "/")[0]
	repo := strings.Split(r.FullName, "/")[1]
	out, _ := json.MarshalIndent(modifications, "", "\t")
	pr.CreatePullRequest(
		r.AccessToken,
		owner,
		repo,
		fmt.Sprintf("Update %s dependencies in %q", c.Language, c.Path),
		branch,
		fmt.Sprintf(
			`This PR updates dependencies, which have not been covered by your versions so far: %s`,
			fmt.Sprintf("\n\n ```\n# %s dependencies in %s\n%s\n```", c.Language, c.Path, out),
		),
	)
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

		go checkDependencies(r, rc.Config)
	})
	nc.Flush()

	select {}
}
