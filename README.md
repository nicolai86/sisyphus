# sisyphus

get PRs for Node dependencies updates

## TODO

- [ ] connect all the loose ends 
- [ ] cleanup & configurability
- [ ] documentation
- [ ] tests
- [ ] infrastructure

## rough cut

```
# build docker images required
$ pushd cmd/dependency-worker/javascript; docker build -t dep-check-js .; popd

# run server to authorize via github, grant access to repos
$ go run cmd/server/main.go -template-path $(pwd)/cmd/server/templates -data-path $(pwd)/tmp

# detect .sisyphus file in enabled repos
$ go run cmd/repos-worker/main.go -data-path=$(pwd)/tmp

# run dependency update job
$ go run cmd/dependency-worker/main.go -build-path=/tmp/build/65048608/76e0c3a9c6b91565e940d9ac110398f7 -language=javascript

# run PR job
$ go run cmd/pull-request-worker/main.go -build-path=/tmp/build/65048608/76e0c3a9c6b91565e940d9ac110398f7 -language=javascript -config-path=./tmp/65048608.json
```

## design idea

The system polls dependency managers (rubygems, npm) for project based updates
and creates PRs to keep up.

The system consists of some long running services and some one-shot services, connected via 
a event bus. Data is stored on the file system, so no database is required.

the current design is made for small self hosted setups: 1 physical VM, running docker,
small set of repos.

long running services:
- server  
  the server provides a UI for users to login & administrate the repos monitored.  
  it needs to be accessible at all times.

one-shot services:
- repos-worker  
  this worker needs to run when a repository is enabled, to extract the sisyphus 
  configuration. 
  Also, it needs to run when the configuration changes.
  Right now, it's designed as a long running service which polls every other hour
- dependency-worker  
  this worker checks if the dependencies are up to date.
  this worker needs to run periodically for enabled subtrees within a repository.
  Right now it's executed manually atm
- pull-request-worker  
  this worker takes a modified dependency information and opens a PR on github.
  This worker is executed manually atm
