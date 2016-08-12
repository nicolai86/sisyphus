package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/github/repo"
	"github.com/nicolai86/sisyphus/storage"
)

var (
	natsURL     string
	dataPath    string
	fileStorage storage.RepositoryReaderWriter
)

func init() {
	flag.StringVar(&dataPath, "data-path", "", "path to store data")
	flag.StringVar(&natsURL, "nats", "tcp://127.0.0.1:4222", "nats server URL")
	flag.Parse()

	fileStorage = storage.NewFileStorage(dataPath)
}

func cachePluginSettings(r storage.Repository, plugin string) {
	log.Printf("caching %q for %s\n", plugin, r.ID)

	owner := strings.Split(r.FullName, "/")[0]
	repoName := strings.Split(r.FullName, "/")[1]
	tmpDir, _ := repo.Clone(r.AccessToken, owner, repoName)

	pluginCacheDir := fmt.Sprintf("%s/%s", dataPath, plugin)
	os.MkdirAll(pluginCacheDir, 0700)
	pluginCache := fmt.Sprintf("%s/%s.json", pluginCacheDir, r.ID)

	f, _ := os.Open(fmt.Sprintf("%s/.sisyphus", tmpDir))

	var configs map[string]interface{}
	json.NewDecoder(strings.NewReader(string(f))).Decode(&configs)

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

	nc, err := nats.Connect(natsURL)
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
