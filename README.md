# plain-delivery

This repository holds the commands that connect mitstack's repositories to
the board, Plain, the work tracker where every piece of work is an issue with
a key such as `MIT-123`. One command gates a pull request on a board key, and
the other records on the board which issues a push to `main` delivered. This
page is for the mitstack engineer who maintains those commands or the CI that
runs them.

| Path | What it does |
|---|---|
| `cmd/pr-keys` | Fails a pull request unless at least one of its commits and its title each name a board key |
| `cmd/board-delivery` | Records once, on the board's `/v1/delivered` endpoint, the keys a push to `main` delivered |
| `boardkeys` | Derives which keys a commit subject or body delivers, and which it only references |

## How a key counts

A key counts in two forms, and `boardkeys` is the one place that tells them
apart.

A delivering key leads the subject or title in a key list closed by a colon,
as in `MIT-123: fix the thing` or `MIT-123, MIT-124: fix two things`. It may
instead sit on its own `Jira:` trailer line in the body. A delivering key
moves its issue to Done once the change reaches `main`.

A reference closes the subject or title in a group, as in
`fix the thing (refs MIT-123)`, or sits on a `Jira-refs:` trailer line. It
names the issue and leaves its status alone. A change whose issue also needs a
release in another repository takes a reference, so the issue closes only when
that later delivery lands.

A key anywhere else, such as in the middle of a subject or inside a merge
commit's default branch name, is prose and counts as neither.

## How the key gate judges a pull request

`pr-keys` passes a pull request only when at least one commit in its
`base..head` range and its own title each name a key, delivering or
referencing. Both halves are checked because either can reach `main`: a merge
or a rebase lands the commits, while a squash lands only the title. The title
comes from `--title`, or from the `PR_TITLE` environment variable when the
flag is absent.

## How board-delivery records a push

`board-delivery` reads the commits of one range and posts the delivering keys
they hold to the board once. The range runs from the local `board-synced` git
tag, the last commit whose keys reached the board, to the pushed commit. When
that tag is missing, the range starts at the push event's `before` sha, and
when neither is usable it is the pushed commit alone.

Only after the board accepts the write does the command move `board-synced` to
the pushed commit and push the tag to `origin`, so the next run starts where
this one ended. A push that delivers no key records nothing and succeeds. A
push with keys and no `TRACKER_BASE_URL` fails, because a delivery that
silently skipped the board would look the same as one that reached it. The
token in `TRACKER_API_TOKEN` is sent as a bearer token, and any board answer
the command prints has that token redacted.

The workspace runbook `docs/kit-delivery-recorder-credential.md` in
mitstack-eu/mitstack-workspace says where those two values come from and how
the token is rotated.

## How a repository runs the commands

A repository pins a release tag of this module, so the commands it runs
change only when it edits that pin. board-automation and plain-block install
`board-delivery` in their `record delivery` job:

```sh
go install github.com/mitstack-eu/plain-delivery/cmd/board-delivery@v0.1.0
```

block-kit and talos-cluster-kit run both commands with `go run` at the same
pin:

```sh
go run github.com/mitstack-eu/plain-delivery/cmd/pr-keys@v0.1.0 --repo . --base "$BASE_SHA" --head "$HEAD_SHA"
go run github.com/mitstack-eu/plain-delivery/cmd/board-delivery@v0.1.0 --repo . --before "$BEFORE_SHA" --after "$GITHUB_SHA"
```

This repository's own CI runs the commands from its checkout, as
`go run ./cmd/pr-keys` and `go run ./cmd/board-delivery`, so a change here is
gated and recorded by the code it changes.

## How a release is cut

A release is a `vX.Y.Z` tag on a commit of `main`, cut by hand with
`git tag` and `git push origin <tag>`. A fix takes the next patch version. A
change that breaks how a repository calls a command takes the next minor
version while the module is below v1, as block-kit does. Each repository above
then moves its pin by editing its own `.github/workflows/ci.yml`.

## Testing

```sh
go vet ./... && go test ./...
```

The module uses the standard library alone. `ci_test.go` reads
`.github/workflows/ci.yml` and fails when a job stops running these commands
the way the sections above describe.
