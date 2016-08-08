package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/libgit2/git2go"
	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/storage"
	"golang.org/x/net/context"
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

	f4, _ := os.OpenFile(fmt.Sprintf("%s/Gemfile", buildPath), os.O_TRUNC|os.O_WRONLY, 0600)
	defer f4.Close()
	b.WriteTo(f4)

	// for _, name := range changedDependencies {
	// 	p.Dependencies[name] = dependencies[name].Latest
	// }

	// f5, err := os.OpenFile(fmt.Sprintf("%s/package.new.json", buildPath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// out, _ := json.MarshalIndent(p, "", "  ")
	// f5.Write(out)

	// if hasPR(r, c, buildPath, changedDependencies) {
	// 	log.Printf("%s has an open PR for %q\n", r.ID, changedDependencies)
	// 	return
	// }
	// log.Printf("pushing new branch to remote…\n")
	// branch := pushChangesToRemote(r, c, buildPath)
	// log.Printf("creating PR\n")
	// createPR(r, c, branch, changedDependencies)
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

		checkDependencies(r, rc.Config)
	})
	nc.Flush()

	select {}
}
