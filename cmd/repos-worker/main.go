package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	git "github.com/libgit2/git2go"
	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/storage"
)

var (
	dataPath    string
	fileStorage storage.RepositoryReaderWriter
)

func init() {
	flag.StringVar(&dataPath, "data-path", "", "path to store data")
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

func cachePluginSettings(r storage.Repository, plugin string) {
	log.Printf("caching %q for %s\n", plugin, r.ID)

	var repo = repoFrom(r)
	defer repo.Free()

	pluginCacheDir := fmt.Sprintf("%s/%s", dataPath, plugin)
	os.MkdirAll(pluginCacheDir, 0700)
	pluginCache := fmt.Sprintf("%s/%s.json", pluginCacheDir, r.ID)

	bs, err := fileContent(repo, ".sisyphus")
	if err != nil {
		log.Printf("Error reading .sisyphus file: %q\n", err)
		// TODO remove previous cached file
		return
	}

	var configs map[string]interface{}
	json.NewDecoder(strings.NewReader(string(bs))).Decode(&configs)

	f, _ := os.OpenFile(pluginCache, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	json.NewEncoder(f).Encode(configs[plugin])
}

func cacheAllPluginSettings(r storage.Repository) {
	log.Printf("scheduling %d plugins for %s\n", len(r.Plugins), r.ID)
	for _, plugin := range r.Plugins {
		cachePluginSettings(r, plugin)
	}
}

func prepareAllRepos() {
	repos, err := fileStorage.Load()
	if err != nil {
		log.Fatalf("can't load repos: %q\n", err)
	}
	log.Printf("scheduling updates for %d repos…\n", len(repos))

	for _, repo := range repos {
		cacheAllPluginSettings(repo)
	}
}

func main() {
	log.Printf("greenkeepr repo worker running")

	nc, err := nats.Connect("tcp://127.0.0.1:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	go prepareAllRepos()

	nc.Subscribe("toggle-repository", func(msg *nats.Msg) {
		repos, err := fileStorage.Load()
		if err != nil {
			log.Fatalf("Failed to read repo storages: %q\n", err)
		}
		repoID := string(msg.Data)
		for _, repo := range repos {
			if repo.ID == repoID {
				cacheAllPluginSettings(repo)
				break
			}
		}
	})
	nc.Flush()

	select {}
}
