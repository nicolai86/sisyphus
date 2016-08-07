# sisyphus

get PRs for Node dependencies updates

## TODO

- [x] automate the workflow, no manual jobs
- [ ] cleanup & configurability
- [ ] documentation
- [ ] tests
- [ ] infrastructure

## rough cut

```
# build docker images required
$ pushd cmd/dependency-worker/javascript; docker build -t dep-check-js .; popd

# install gnatsd
$ go get github.com/nats-io/gnatsd
$ gnatsd -D -V

# run server to authorize via github, grant access to repos
$ go run cmd/server/main.go -template-path $(pwd)/cmd/server/templates -data-path $(pwd)/tmp

# detect .sisyphus file in enabled repos
$ go run cmd/repos-worker/main.go -data-path=$(pwd)/tmp

# run dependency update worker
$ go run cmd/dependency-worker/main.go -data-path=$(pwd)/tmp

# run scheduler 
$ go run cmd/repo-schedule-worker/main.go -data-path=$(pwd)/tmp
```
