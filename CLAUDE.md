# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

A public transit router in Go: parses a GTFS feed and builds a RAPTOR timetable for multi-round journey queries.

## Remotes

- **`origin` (`Andykmcc/router-next`) is the primary remote.** Push branches and open PRs there. `gh` already defaults to it.
- `upstream` (`bikehopper/router-next`) is the original project. Don't push or open PRs there unless asked. The README's clone URL points at upstream; ignore it.

## Commands

Go 1.26.1 (`.go-version`). The module is named `router`, so imports are `router/pkg/...`.

```sh
go build ./...
go test ./...
go test ./cmd/raptor -run TestRaptorBuild     # snapshot test; parses the 64 MB bundled feed (~3s)
go test ./pkg/utils -run TestSnapshotStr
go vet ./...
golangci-lint run ./...
make setup                                     # lefthook install
make build                                     # binary ./router-next
```

CLI (`cmd/raptor/build.go`) needs a subcommand. The README's `go run ./cmd/raptor path/to/gtfs.zip` is out of date.

```sh
go run ./cmd/raptor build -gtfs cmd/raptor/testdata/gtfs_04162026.zip   # builds for today's date, prints table size
go run ./cmd/raptor serve -gtfs <zip> -port 3456                         # debug server
```

`-gtfs` defaults to `./gtfs.zip`. Run `serve` from the repo root: it serves `./static`, and `static/index.html` hardcodes `http://localhost:3456`. The server only exposes `GET /routes/{routeId}/stops`.

## Lint, hooks, CI

- Pre-commit (lefthook) runs `golangci-lint run --fix ./... ; git add {staged_files}` and `go vet ./...` in parallel. Because of the `;`, lint failures never block a commit, but `--fix` may rewrite and re-stage files. Vet failures do block.
- `.golangci.yaml` (v2): `default: fast` plus `wsl_v5` (blank-line rules, `branch-max-lines: 2`) and `exhaustruct` (struct literals must set every field).
- Main already has 13 lint issues (10 `wsl_v5`, plus `gocognit`, `testpackage`, `whitespace`). Don't add new ones.
- CI (`.github/workflows/test.yml`) runs only `go build` and `go test` on pushes and PRs to `main`. It doesn't lint.

## Architecture

Pipeline: GTFS zip → `gtfs.ParseGtfs` → `*gtfs.GTFSTable` → `raptor.BuildRaptorTable(table, date)` → `*raptor.RaptorTable` → `rt.Route(start, end, startTime)` → `[]Journey`.

**`pkg/gtfs`** parses routes, trips, calendar, calendar_dates, stops, stop_times and transfers into structs keyed by string IDs (`GTFSStopID` etc.). It ignores every other feed file, including `frequencies.txt`.
- A missing file gives a nil slice, not an error.
- Rows that fail to parse are silently dropped (TODO in `parseCSVFile`).
- Times are `types.Timestamp`, seconds since midnight; hours past 24 are allowed.
- Dates are `GTFSDate` strings (`YYYYMMDD`) compared as strings.
- `TripsForDate` applies calendar plus calendar_dates exceptions.

**`pkg/types`** holds the dense numeric IDs (`StopID`, `RouteID`), `Timestamp` and `INFINITY`.

**`pkg/raptor`** builds and queries the timetable. Each table is built for one service date.
- A RAPTOR route is a unique ordered stop sequence, **not** a GTFS route. One GTFS route usually becomes several RAPTOR routes.
- Routes are sorted by their stop-sequence key string, and trips within a route by first departure. That fixed ordering keeps the snapshots stable.
- ID spaces:
  - `StopID` is the index into `gtfsTable.Stops`.
  - `RouteID` is the index into the sorted RAPTOR routes.
  - `RaptorTripID` is a build-time index into the `TripsForDate(date)` result.
- The layout is flat arrays plus offset arrays (CSR style). Each `First*Of*` offset array has length N+1 with a trailing sentinel.
  - `StopEventsByRoute` stores each route's trips row-major: trip `t`, stop `i` of route `r` is at `FirstStopEventOfRoute[r] + t*numStopsInRoute + i`.
  - Use the accessors in `table.go` (`StopsForRoute`, `StopEventsForTrip`, `TripInRoute`, `RoutesForStop`) rather than indexing directly.
- Adding a table field means touching:
  - `iterateOverRaptorRoutes` or `groupRouteSegments` (`build.go`)
  - the `RaptorTable` struct and `Sizeof` (`table.go`)
  - `SnapshotString`, if the field should be snapshot-tested
- Transfers: the only ones used are same-stop `transfers.txt` rows with `transfer_type == 2`. They become a per-stop `MinTransferTime` that is added before boarding. There are no walking transfers between stops yet (TODO in `route.go`; work in progress on the `knn-transfers` branch).
- `Route` runs round-based RAPTOR (`MAX_ROUNDS = 10`) and returns the Pareto set over (number of legs, arrival time). It has no tests, and neither the CLI nor the debug server calls it.

## Testing

- `TestRaptorBuild` builds tables from the bundled feed for 2026-04-09 and 2026-04-20. It compares `SnapshotString()` with `cmd/raptor/snapshots/<date>.txt`.
  - **On a mismatch the test fails and also overwrites the snapshot**, so a second run passes. Check `git diff cmd/raptor/snapshots/` before accepting a change.
  - Snapshots cover only `MinTransferTime` and the ID and offset arrays. They don't cover `Stops`, `Routes`, `TripsByRoute`, `StopEventsByRoute` or `RouteSegmentsByStop`.
- The bundled feed `cmd/raptor/testdata/gtfs_04162026.zip` is committed directly (not LFS). It's the 511 SF Bay regional feed with many agencies, not only SF Muni as the README says.

## Known issues (unfixed)

- `BuildRaptorTable` passes all of `gtfsTable.Trips` to `iterateOverRaptorRoutes`. That function indexes the list with `RaptorTripID`s, which are positions in `TripsForDate(date)`. As a result, `TripsByRoute` and `Routes` can hold the wrong trips and routes. The snapshots don't catch this.
- In `groupRaptorTrips`, a stop time whose `stop_id` is unknown logs a WARN but is still added, with stop ID 0.
