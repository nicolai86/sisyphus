package main

import (
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/storage"
)

var (
	fileStorage storage.RepositoryReader
	natsURL     string
)

func init() {
	var dataPath string
	flag.StringVar(&dataPath, "data-path", "", "path to store data")
	flag.StringVar(&natsURL, "nats", "tcp://127.0.0.1:4222", "nats server URL")
	flag.Parse()

	fileStorage = storage.NewFileStorage(dataPath)
}

func main() {
	log.Printf("greenkeepr repo schedule worker running")

	nc, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	for {
		select {
		case <-time.After(time.Second * 5):
			repos, err := fileStorage.Load()
			if err != nil {
				log.Fatal(err)
			}

			for _, repo := range repos {
				for _, plugin := range repo.Plugins {
					log.Printf("scheduling %q for %s\n", plugin, repo.ID)
					out, _ := json.Marshal(repo)
					nc.Publish(plugin, out)
				}
				nc.Flush()
			}

		}
	}
}
