#!/usr/bin/env bash

set -eu

pushd cmd/greenkeepr-javascript/checker
docker build -t dep-check-js .
popd

pushd cmd/greenkeepr-ruby/checker
docker build -t dep-check-rb .
popd

export GOOS=linux
export GOARCH=amd64

go build -o bin/frontend ./cmd/frontend/main.go
go build -o bin/repository-scheduler ./cmd/repository-scheduler
go build -o bin/greenkeepr-master ./cmd/greenkeepr-master/main.go
go build -o bin/greenkeepr-javascript ./cmd/greenkeepr-javascript/main.go
go build -o bin/greenkeepr-ruby ./cmd/greenkeepr-ruby/*.go

for binary in $(find bin/ -type f); do
  chmod +x $binary
done

services=(frontend repository-scheduler greenkeepr-master greenkeepr-javascript greenkeepr-ruby)
for service in ${services[@]}; do
  docker-compose build $service
done
