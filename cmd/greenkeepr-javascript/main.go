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

func init() {
	var (
		dataPath      string
		bucket        string
		encryptionKey string
	)
	flag.StringVar(&dataPath, "data-path", "", "data directory")
	flag.StringVar(&bucket, "s3-bucket", "", "s3 storage bucket")
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

type versionInfo struct {
	Wanted string
	Latest string
}

type packageJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Description          string            `json:"description"`
	Keywords             interface{}       `json:"keywords,omitempty"`
	Homepage             string            `json:"homepage,omitempty"`
	Bugs                 interface{}       `json:"bugs,omitempty"`
	License              string            `json:"license,omitempty"`
	Author               string            `json:"author"`
	Contributors         interface{}       `json:"contributors,omitempty"`
	Files                interface{}       `json:"files,omitempty"`
	Main                 string            `json:"main"`
	Bin                  interface{}       `json:"bin,omitempty"`
	Man                  interface{}       `json:"man,omitempty"`
	Repository           interface{}       `json:"repository,omitempty"`
	Scripts              interface{}       `json:"scripts,omitempty"`
	Config               interface{}       `json:"config,omitempty"`
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`
	BundledDependencies  map[string]string `json:"bundledDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	Engines              interface{}       `json:"engines,omitempty"`
	EngineStrict         interface{}       `json:"engineStrict,omitempty"`
	OS                   interface{}       `json:"os,omitempty"`
	CPU                  interface{}       `json:"cpu,omitempty"`
	PreferGlobal         *bool             `json:"preferGlobal,omitempty"`
	Private              *bool             `json:"private,omitempty"`
	PublishConfig        interface{}       `json:"publishConfig,omitempty"`
}

type config struct {
	Path     string
	Language string
}

type repoConfig struct {
	Config       config
	RepositoryID string
}

var filesToExtract = []string{"package.json"}

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
	// docker run --rm -v $(pwd)/outdated.json:/home/checker/outdated.json:rw -v $(pwd)/package.json:/home/checker/package.json:ro -t dep-check-js
	f, _ := os.OpenFile(fmt.Sprintf("%s/outdated.json", buildPath), os.O_CREATE|os.O_TRUNC, 0600)
	f.Close()

	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	container, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image: "dep-check-js",
	}, &container.HostConfig{
		AutoRemove: true,
		Binds: []string{
			fmt.Sprintf("%s/outdated.json:/home/checker/outdated.json:rw", buildPath),
			fmt.Sprintf("%s/package.json:/home/checker/package.json:ro", buildPath),
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

	var dependencies = map[string]versionInfo{}
	f2, _ := os.Open(fmt.Sprintf("%s/outdated.json", buildPath))
	defer f2.Close()
	json.NewDecoder(f2).Decode(&dependencies)

	f3, _ := os.Open(fmt.Sprintf("%s/package.json", buildPath))
	defer f3.Close()
	var p packageJSON
	json.NewDecoder(f3).Decode(&p)

	var changedDependencies = []string{}
	for name, dep := range dependencies {
		if dep.Latest != dep.Wanted {
			changedDependencies = append(changedDependencies, name)
		}
	}
	if len(changedDependencies) == 0 {
		log.Printf("Nothing to do for %q %q %q", r.ID, c.Path, c.Language)
		return
	}

	for _, name := range changedDependencies {
		p.Dependencies[name] = dependencies[name].Latest
	}

	f5, err := os.OpenFile(fmt.Sprintf("%s/package.new.json", buildPath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	out, _ := json.MarshalIndent(p, "", "  ")
	f5.Write(out)

	if hasPR(r, c, changedDependencies) {
		log.Printf("%s has an open PR for %q\n", r.ID, changedDependencies)
		return
	}
	log.Printf("pushing new branch to remote…\n")
	branch := pushChangesToRemote(r, c, buildPath)
	log.Printf("creating PR\n")
	createPR(r, c, branch, changedDependencies)
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

func pushChangesToRemote(r storage.Repository, c config, buildPath string) string {
	owner := strings.Split(r.FullName, "/")[0]
	repo := strings.Split(r.FullName, "/")[1]
	branch, err := pr.PublishChanges(r.AccessToken, owner, repo, []pr.UpdateFile{
		pr.UpdateFile{
			Source:      fmt.Sprintf("%s/package.new.json", buildPath),
			Destination: fmt.Sprintf("%s/package.json", c.Path),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return branch
}

func main() {
	log.Printf("greenkeepr dependency worker for javascript running")

	nc1, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc1.Close()
	nc = nc1

	nc.Subscribe("greenkeep-javascript", func(msg *nats.Msg) {
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
