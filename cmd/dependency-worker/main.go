package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/storage"
)

var (
	dataPath    string
	fileStorage storage.RepositoryReader
	nc          *nats.Conn
)

func init() {
	flag.StringVar(&dataPath, "data-path", "", "data directory")
	flag.Parse()

	fileStorage = storage.NewFileStorage(dataPath)
}

type config struct {
	Path     string
	Language string
}

type repoConfig struct {
	Config       config
	RepositoryID string
}

func main() {
	log.Printf("greenkeepr dependency worker running")

	nc1, err := nats.Connect("tcp://127.0.0.1:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc1.Close()
	nc = nc1

	nc.Subscribe("greenkeep", func(msg *nats.Msg) {
		repos, err := fileStorage.Load()
		if err != nil {
			log.Fatalf("Failed to read repo storages: %q\n", err)
		}
		repoID := string(msg.Data)
		var r storage.Repository
		for _, repo := range repos {
			if repo.ID == repoID {
				r = repo
				break
			}
		}

		configPath := fmt.Sprintf("%s/%s/%s.json", dataPath, "greenkeep", repoID)
		if _, err := os.Stat(configPath); err != nil {
			log.Printf("%q does not exist. skipping", configPath)
			return
		}

		var cs []config
		f, _ := os.Open(configPath)
		json.NewDecoder(f).Decode(&cs)

		for _, c := range cs {
			log.Printf("fan-out for %q and %q (%q)", repoID, c.Language, c.Path)
			b, err := json.Marshal(&repoConfig{
				Config:       c,
				RepositoryID: r.ID,
			})
			if err != nil {
				log.Fatal(err)
			}
			nc.Publish(fmt.Sprintf("greenkeep-%s", c.Language), b)
			nc.Flush()
		}
	})
	nc.Flush()

	select {}
}
