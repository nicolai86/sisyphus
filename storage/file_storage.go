package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileStorage struct {
	DataDirectory string
}

func NewFileStorage(dataDirectory string) FileStorage {
	return FileStorage{
		DataDirectory: dataDirectory,
	}
}

func (f FileStorage) Store(r Repository) error {
	file, err := os.OpenFile(
		fmt.Sprintf("%s/%s.json", f.DataDirectory, r.ID),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		0600,
	)
	if err != nil {
		return err
	}

	return json.NewEncoder(file).Encode(r)
}

func (f FileStorage) Load() ([]Repository, error) {
	var repos []Repository

	err := filepath.Walk(f.DataDirectory, func(path string, _ os.FileInfo, _ error) error {
		if strings.HasSuffix(path, ".json") {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			var repo Repository
			if err := json.NewDecoder(file).Decode(&repo); err != nil {
				return err
			}
			repos = append(repos, repo)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return repos, nil
}
