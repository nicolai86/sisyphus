package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"golang.org/x/net/context"
)

var (
	buildPath string
	language  string
)

func init() {
	flag.StringVar(&buildPath, "build-path", "", "directory to run builds in")
	flag.StringVar(&language, "language", "", "language to use [javascript,ruby]")
	flag.Parse()
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

func main() {
	log.Printf("greenkeepr dependency worker running")

	// TODO ruby
	if language != "javascript" {
		log.Fatalf("Only javascript supported at this moment")
	}

	// docker run --rm -v $(pwd)/outdated.json:/home/checker/outdated.json:rw -v $(pwd)/package.json:/home/checker/package.json:ro -t dep-check-js
	f, _ := os.OpenFile(fmt.Sprintf("%s/outdated.json", buildPath), os.O_CREATE|os.O_TRUNC, 0600)
	f.Close()

	cli, err := client.NewClient("unix:///var/run/docker.sock", "v1.22", nil, nil)
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

	for name, dep := range dependencies {
		p.Dependencies[name] = dep.Latest
	}

	f5, err := os.OpenFile(fmt.Sprintf("%s/package.new.json", buildPath), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	out, _ := json.MarshalIndent(p, "", "  ")
	f5.Write(out)

	// TODO check for PRs
	// TODO if no pr exists -> create one
}
