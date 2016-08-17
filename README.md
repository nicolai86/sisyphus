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

## overview

sisyphus is designed to regular check github repositories based on plugin definitions.
Right now it ships with a single plugin, `greenkeep`, which checks your github
repository for dependency updates & creates a PR if a new version is available.

it's designed to be mono-repo friendly, and also assumes that you work in a github-flow similar manner:
master is the source and destination for all PRs.

when a user enables or disables a repository, the accompanied configuration is stored
in a pluggable configuration backend, which also supports encryption if so desired,
out of the box.

## TODO

- [x] automate the workflow, no manual jobs
- [ ] cleanup & configurability
- [ ] documentation
- [ ] tests
- [ ] infrastructure
