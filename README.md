# sisyphus

get PRs for Ruby & Node dependency updates - for your mono repo.

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
$ go run cmd/frontend/main.go -template-path $(pwd)/cmd/server/templates -data-path $(pwd)/tmp

# run dependency update worker
$ go run cmd/greenkeepr-master/main.go -data-path=$(pwd)/tmp
$ go run cmd/greenkeepr-javascript/main.go -data-path=$(pwd)/tmp
$ go run cmd/greenkeepr-ruby/parse.go cmd/dependency-rb-worker/main.go -data-path=$(pwd)/tmp

# run scheduler 
$ go run cmd/repository-scheduler/main.go -data-path=$(pwd)/tmp
```
