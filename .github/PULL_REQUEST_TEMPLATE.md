## What this does

## Test plan

- [ ] `go build ./... && go vet ./... && golangci-lint run ./...`
- [ ] `go test ./... -race`
- [ ] `reuse lint`

## Checklist

- [ ] Every commit is signed off (DCO): `git commit -s`
- [ ] Commits are GPG-signed
- [ ] PR title follows [Conventional Commits](https://www.conventionalcommits.org/) (this repo squash-merges, so the title becomes the permanent history entry)
- [ ] New source files have an SPDX header, or an entry in `REUSE.toml`
