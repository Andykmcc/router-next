# router-next

A public transit routing engine written in Go. It parses GTFS feeds and
builds a RAPTOR timetable for fast, multi-round transit journey queries.

## Setup

Requires Go 1.26+.

```sh
git clone git@github.com:bikehopper/router-next.git
cd router-next
make setup  # installs git hooks via lefthook
```

## Development

Run the tests:

```sh
make test
```

Build a RAPTOR table from a GTFS zip (prints the table's in-memory size):

```sh
go run ./cmd/raptor path/to/gtfs.zip
```

A sample San Francisco Muni feed is bundled at
`cmd/raptor/testdata/gtfs_04162026.zip` for testing.
:eyes:

## Working with upstream

This repo is a fork of [bikehopper/router-next](https://github.com/bikehopper/router-next)
and needs to stay interoperable with it.

### One-time setup

Add the upstream repo as a second remote:

```bash
git remote add upstream git@github.com:bikehopper/router-next.git
```

Verify with `git remote -v` — you should see `origin` (your fork) and `upstream`
(bikehopper).

### Pull a branch from upstream

```bash
git fetch upstream
git switch -c their-branch upstream/their-branch
```

`git fetch upstream` only updates the `upstream/*` remote-tracking refs; it never
touches your working tree or local branches, so it is safe to run anytime.

### Keep your `main` in sync

```bash
git switch main
git fetch upstream
git merge --ff-only upstream/main   # or: git rebase upstream/main
git push origin main
```
