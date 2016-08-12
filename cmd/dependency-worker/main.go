package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/storage"
)

var (
	natsURL     string
	fileStorage storage.RepositoryReader
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

type config struct {
	Path     string
	Language string
}

type repoConfig struct {
	Config       config
	RepositoryID string
}

type greenkeepConfig struct {
	Greenkeep []config `json:"greenkeep"`
}

func main() {
	log.Printf("greenkeepr dependency worker running")

	nc1, err := nats.Connect(natsURL)
	if err != nil {
		log.Fatal(err)
	}
	defer nc1.Close()
	nc = nc1

	nc.Subscribe("greenkeep", func(msg *nats.Msg) {
		var r storage.Repository
		json.Unmarshal(msg.Data, &r)

		owner := strings.Split(r.FullName, "/")[0]
		repoName := strings.Split(r.FullName, "/")[1]
		uri := fmt.Sprintf("https://%s@raw.githubusercontent.com/%s/%s/master/.sisyphus", r.AccessToken, owner, repoName)
		resp, _ := http.Get(uri)
		bs, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		var m greenkeepConfig
		json.Unmarshal(bs, &m)

		for _, c := range m.Greenkeep {
			log.Printf("fan-out for %q and %q (%q)", r.ID, c.Language, c.Path)
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
