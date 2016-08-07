package main

import (
	"crypto/md5"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	git "github.com/libgit2/git2go"
	"github.com/nicolai86/sisyphus/storage"
)

var (
	fileStorage storage.RepositoryReaderWriter
)

func init() {
	var dataPath string
	flag.StringVar(&dataPath, "data-path", "", "path to store data")
	flag.Parse()

	fileStorage = storage.NewFileStorage(dataPath)
}

type config struct {
	Path     string
	Language string
}

var filesByLanguage = map[string][]string{
	"ruby":       []string{"Gemfile", "Gemfile.lock"},
	"javascript": []string{"package.json"},
}

func schedulePluginConfig(r storage.Repository, plugin string, c config) {
	// this should normally be a differnt binary
	if plugin != "greenkeep" {
		return
	}

	filesToExtract := filesByLanguage[c.Language]
	log.Printf("looking up %q for %q (%q): %q", plugin, c.Path, c.Language, filesToExtract)

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
	f, _ := os.OpenFile(fmt.Sprintf("%s/%s", cachePath, "config.json"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 700)
	json.NewEncoder(f).Encode(&c)
	// TODO schedule dependency-worker
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

func schedulePlugin(r storage.Repository, plugin string) {
	// TODO this should normally call a different binary
	if plugin != "greenkeep" {
		log.Printf("Unknown plugin %q\n", plugin)
		return
	}

	log.Printf("scheduling %q\n", plugin)

	var repo = repoFrom(r)
	defer repo.Free()

	bs, err := fileContent(repo, ".sisyphus")
	if err != nil {
		panic(err)
	}

	var configs map[string][]config
	json.NewDecoder(strings.NewReader(string(bs))).Decode(&configs)

	c := configs[plugin]
	for _, cc := range c {
		schedulePluginConfig(r, plugin, cc)
	}
}

func schedulePlugins(r storage.Repository) {
	for _, plugin := range r.Plugins {
		schedulePlugin(r, plugin)
	}
}

func scheduleUpdates() {
	repos, err := fileStorage.Load()
	if err != nil {
		log.Fatalf("can't load repos: %q\n", err)
	}
	log.Printf("scheduling updates for %d repos…\n", len(repos))

	for _, repo := range repos {
		schedulePlugins(repo)
	}
}

func main() {
	log.Printf("greenkeepr repo worker running")

	scheduleUpdates()
	for {
		select {
		case <-time.After(2 * time.Hour):
			scheduleUpdates()
		}
	}
}
