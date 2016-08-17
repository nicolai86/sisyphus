# sisyphus

get PRs for Ruby & Node dependency updates - for your mono repo.

## rough cut

```
# build all required docker images
$ ./scripts/build.sh

# export your github client envs GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET
$ source .env

# start everything locally
$ docker-compose up
```

next, add a `.sisyphus` file to a repo of your choice and enable the repo in your 
sisyphus web ui:

```
{
  "greenkeep": [
    {
      "path": "path/a",
      "language": "javascript"
    },
    {
      "path": "path/b",
      "language": "ruby"
    }
  ]
}
```

## TODO

- [x] automate the workflow, no manual jobs
- [ ] cleanup & configurability
- [ ] documentation
- [ ] tests
- [ ] infrastructure
