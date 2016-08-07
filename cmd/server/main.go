package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/go-github/github"
	"github.com/nats-io/nats"
	"github.com/nicolai86/sisyphus/storage"
	"github.com/nicolai86/sisyphus/uuid"
	"golang.org/x/oauth2"
)

func HTTPLogger(service string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[%s] %s %s %s", service, r.Method, r.URL, time.Since(start).String())
	})
}

type githubCallback struct {
	ClientID     string
	ClientSecret string
	Code         string
	RedirectURI  string
	State        string
}

type tokenData struct {
	Username    string
	AccessToken string
}

type githubOrganization struct {
	*github.Organization
	Repositories []*github.Repository
}

type loggedIndexData struct {
	Repositories  []*github.Repository
	Organizations []*githubOrganization
}

var (
	templatePath          string
	fileStorage           storage.RepositoryReaderWriter
	temporaryAccessTokens map[string]string
	conf                  *oauth2.Config
)

func allRepos(client *github.Client, accessToken string) []*github.Repository {
	var allRepos []*github.Repository
	opt := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 25},
	}
	for {
		repos, resp, err := client.Repositories.List("", opt)
		if err != nil {
			log.Fatalf("%q", err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}
	return allRepos
}

func allReposByOrg(client *github.Client, orgName string) []*github.Repository {
	var allRepos []*github.Repository
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 25},
	}
	for {
		repos, resp, err := client.Repositories.ListByOrg(orgName, opt)
		if err != nil {
			log.Fatalf("%q", err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.ListOptions.Page = resp.NextPage
	}
	return allRepos
}

func allOrgs(client *github.Client, accessToken string) []*githubOrganization {
	var allOrgs []*githubOrganization
	opt := &github.ListOptions{PerPage: 25}
	for {
		orgs, resp, err := client.Organizations.List("", opt)
		if err != nil {
			log.Fatalf("%q", err)
		}
		for _, org := range orgs {
			o := githubOrganization{
				Organization: org,
				Repositories: make([]*github.Repository, 0),
			}
			o.Repositories = allReposByOrg(client, *org.Login)
			allOrgs = append(allOrgs, &o)
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allOrgs
}

func renderAnonymousIndex(state string, w http.ResponseWriter) {
	index, err := template.New("anonymous.tpl").Funcs(template.FuncMap{
		"authURL": func() string {
			// TODO persist state for comparison
			return conf.AuthCodeURL(state, oauth2.AccessTypeOnline)
		},
	}).ParseFiles(fmt.Sprintf("%s/index/anonymous.tpl", templatePath))
	if err != nil {
		fmt.Printf("%#v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var b bytes.Buffer
	if index.Execute(&b, nil); err != nil {
		fmt.Printf("%#v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, &b)
}

func renderSignedInIndex(accessToken string, w http.ResponseWriter) {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)

	var repos = allRepos(client, accessToken)
	var orgs = allOrgs(client, accessToken)

	var data = loggedIndexData{
		Organizations: orgs,
		Repositories:  repos,
	}

	loggedInIndex, err := template.New("signed-in.tpl").Funcs(template.FuncMap{
		"enabled": func(repoName, service string) bool {
			repos, err := fileStorage.Load()
			if err != nil {
				return false
			}

			for _, repo := range repos {
				if repo.FullName == repoName {
					for _, plugin := range repo.Plugins {
						if plugin == service {
							return true
						}
					}
					return false
				}
			}
			return false
		},
	}).ParseFiles(fmt.Sprintf("%s/index/signed-in.tpl", templatePath))
	if err != nil {
		fmt.Printf("%#v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var b bytes.Buffer
	if err := loggedInIndex.Execute(&b, data); err != nil {
		fmt.Printf("%#v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, &b)
}

func init() {
	var dataPath string
	flag.StringVar(&templatePath, "template-path", "", "path to templates")
	flag.StringVar(&dataPath, "data-path", "", "path to store data")
	flag.Parse()

	fileStorage = storage.NewFileStorage(dataPath)
	temporaryAccessTokens = make(map[string]string)
	conf = &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		Scopes:       []string{"user", "repo", "admin:repo_hook"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		},
	}
}

func main() {
	log.Printf("greenkeepr server listening")

	nc, err := nats.Connect("tcp://127.0.0.1:4222")
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()

	srv := http.Server{
		ReadTimeout:  4 * time.Second,
		WriteTimeout: 6 * time.Second,
		Addr:         ":3000",
		Handler: HTTPLogger("greenkeepr", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == "GET" && req.URL.Path == "/" {
				c, err := req.Cookie("id")
				if err != nil || c == nil {
					renderAnonymousIndex("state", w)
				} else {
					if _, ok := temporaryAccessTokens[c.Value]; !ok {
						renderAnonymousIndex("state", w)
					} else {
						renderSignedInIndex(temporaryAccessTokens[c.Value], w)
					}
				}
				return
			}

			if req.URL.Path == "/toggle" {
				c, _ := req.Cookie("id")
				req.ParseForm()
				vals := req.Form
				repo := storage.Repository{
					ID:          vals.Get("repository_id"),
					FullName:    vals.Get("repository_name"),
					AccessToken: temporaryAccessTokens[c.Value],
					GitURL:      vals.Get("repository_git_url"),
				}

				if vals.Get("action") == "disable" {
					repo.Plugins = []string{}
				} else {
					repo.Plugins = []string{vals.Get("service")}
				}

				if err := fileStorage.Store(repo); err != nil {
					log.Fatalf("Failed to store repo: %q\n", err)
				}

				nc.Publish("toggle-repository", []byte(repo.ID))
				nc.Flush()

				http.Redirect(w, req, "/", http.StatusFound)
				return
			}

			if req.URL.Path == "/logout" {
				cookie := http.Cookie{
					Name:    "id",
					Value:   "",
					Expires: time.Now().Add(-1 * 365 * 24 * time.Hour),
					Path:    "/",
					Domain:  "localhost",
				}
				http.SetCookie(w, &cookie)

				renderAnonymousIndex("state", w)
				return
			}

			if req.URL.Path == "/github/callback" {
				c, _ := req.Cookie("id")
				if c != nil {
					if _, ok := temporaryAccessTokens[c.Value]; ok {
						http.Redirect(w, req, "/", http.StatusFound)
						return
					}
				}

				var data githubCallback
				query := req.URL.Query()
				data.ClientID = query.Get("client_id")
				data.ClientSecret = query.Get("client_secret")
				data.Code = query.Get("code")
				data.RedirectURI = query.Get("redirect_uri")
				data.State = query.Get("state")

				tok, err := conf.Exchange(oauth2.NoContext, data.Code)
				// TODO compare state
				if err != nil {
					log.Fatal(err)
				}

				// TODO persist the state…
				id, _ := uuid.New()
				cookie := http.Cookie{
					Name:    "id",
					Value:   id,
					Expires: time.Now().Add(365 * 24 * time.Hour),
					Path:    "/",
					Domain:  "localhost",
				}
				http.SetCookie(w, &cookie)

				temporaryAccessTokens[id] = tok.AccessToken

				renderSignedInIndex(temporaryAccessTokens[id], w)
				return
			}
		})),
	}
	srv.ListenAndServe()
	// TODO add support to activate specific repositories
	// TODO add support for webhooks -> schedule worker
}
