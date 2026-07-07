# Contributing to memaudit

Thanks for considering a contribution.

## Developer Certificate of Origin

Every commit must be signed off (DCO, not a CLA): add a `Signed-off-by`
trailer to each commit message, e.g.

    git commit -s -m "your message"

By signing off you certify the text in [DCO](DCO). The DCO GitHub App checks
every pull request and blocks merging on an unsigned commit.

## Pull request titles

This repo only allows squash merging, so a PR's title becomes its permanent
entry in `main`'s history — and the changelog is generated from that
history. PR titles must follow [Conventional Commits](https://www.conventionalcommits.org/),
e.g. `feat: add PSI collector` or `fix(spool): rotate on size, not just time`.
CI checks this on every pull request; commit messages inside the branch
itself don't matter, since squashing discards them.

## License headers

New source files need an SPDX header:

    // SPDX-FileCopyrightText: <year> <you or your org>
    // SPDX-License-Identifier: Apache-2.0

Files that can't carry a header (binary fixtures, generated files) get an
entry in `REUSE.toml` instead. `task reuse-lint` checks compliance; it also
runs in CI.

## Local checks

This project uses [Task](https://taskfile.dev) instead of Make:

    task build   # go build ./...
    task test    # go test ./... -race
    task lint    # golangci-lint run
    task vet     # go vet ./...

All of these run in CI on every pull request.
