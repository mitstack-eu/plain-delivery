# plain-delivery

This repository holds the commands that connect mitstack's repositories to
the board, Plain, the work tracker where every piece of work is an issue with
a key such as `MIT-123`. Two commands judge a pull request before it merges,
and a third records on the board which issues a push to `main` delivered. This
page is for the mitstack engineer who maintains those commands or the CI that
runs them.

| Path | What it does |
|---|---|
| `cmd/pr-keys` | Fails a pull request unless at least one of its commits and its title each name a board key |
| `cmd/pr-delivery` | Fails a pull request whose code lands before its tests, or whose keys are not issues claimed on the board |
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

## How the delivery check judges a pull request

`pr-delivery` holds a pull request to two rules: its failing tests land
before the code that passes them, and every key it names is an issue someone
has claimed. It reads the commits from the merge base of `--base` and
`--head` to `--head`, oldest first, leaving out merge commits.

The first rule sorts each changed path into one of three classes, from the one
table in `cmd/pr-delivery/classify.go`.

| Class | Examples |
|---|---|
| test | `*_test.go`, `*_test.sh`, `*.test.ts`, an OpenTofu test `*.tftest.hcl`, the `terraform.tfvars` that `tofu test` loads from a `tests/` directory, a Playwright spec under `app/e2e/`, anything under a `testdata/` directory |
| other | Markdown, `docs/`, a block pin under `blocks/pins/`, a submodule pin, `go.mod`, `go.sum`, `package-lock.json`, a workflow change that only moves action versions, and a Dockerfile change that only moves module versions |
| code | Every path the table does not name |

A workflow under `.github/workflows/` is code, with one exception that
`classifyPatch` in `cmd/pr-delivery/classify.go` reads from the commit's
patch. When every changed line is a `uses:` line, replaced one for one, and
only the version after `@` and its trailing comment differ, the file counts as
other, like a `go.sum` line. Dependabot's Actions bump changes exactly that,
as in this illustration, whose hashes are placeholders rather than real
releases:

```diff
-      - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v7.0.0
+      - uses: actions/checkout@08c6903cd8c0fde910a37f88322edcfb5dd907a8 # v7.0.1
```

The rule holds for every author. Any other changed line, such as an added
`with:` input, a new step or a different action, keeps the file code.

A file named `Dockerfile` is code too, with a second exception that
`modulePinsOnly`, in the same file, reads from the patch. A module move often
adds lines and reflows the comment above them, so this rule judges each added
and removed line on its own. The file counts as other when every such line is
one of three kinds:

1. A `--replace M=M@V` line for xcaddy, the tool that builds Caddy with
   chosen modules, naming the same module M on both sides of the `=`.
2. A `go get` argument line of one or more `M@V` words, which may follow
   `go get` itself.
3. A comment.

Here M is a Go module path, whose first element holds a dot as Go requires,
and V a full version such as `v1.47.0`, so `@latest` does not count. A line
may end in a semicolon, a backslash or both. This patch, which adds a module
after the last `--replace` of a build, counts as other:

```diff
-      --replace github.com/go-jose/go-jose/v4=github.com/go-jose/go-jose/v4@v4.1.6
+      --replace github.com/go-jose/go-jose/v4=github.com/go-jose/go-jose/v4@v4.1.6 \
+      --replace go.opentelemetry.io/otel=go.opentelemetry.io/otel@v1.47.0
```

Any other changed line keeps the file code. That covers a `--replace` onto
another module, a `--with` plugin, text after a version and a `RUN` line. It
also covers a comment shaped like a parser directive, such as `# syntax=`,
wherever it sits. A parser directive is a comment at the head of a Dockerfile
that tells Docker how to read the rest of the file, and the patch alone cannot
show whether a line heads the file. A path whose name only starts with
`Dockerfile`, such as a `Dockerfile.tmpl` template, keeps the class the table
gives it.

The check fails in three cases. The first commit to change a test or code
changes code alone. One commit changes a test and code together. Code changes
while no commit changes a test. A change of Markdown or pins alone has no
test commit and passes. A file moved with no edit counts as neither a test
nor code, since it changes no behaviour and no assertion.

The second rule takes every key the title and the commits name, delivering or
referencing, as `pr-keys` reads them. The title and at least one commit must
each name one. The command then asks the board for each key's status in one
`GET /v1/issues?keys=...&fields=status` request, and passes only a key that is
`In Progress` or `Pull Request`. A key the board does not hold fails, and so
does one in any other status, such as `Backlog`, `Ready`, `Done` or
`Removed`, by name and status. A run with no
`TRACKER_BASE_URL` fails too, because no board can then say the key is
claimed.

Two GitHub Apps open pull requests with no board issue behind them:
`dependabot[bot]`, which bumps dependencies, and `mitstack-ci[bot]`, which
moves the workspace's platform submodule pin. They are listed by login in
`keylessApps` in `cmd/pr-delivery/run.go`, the one place the list lives. The
workflow passes the author's login from the event as `PR_AUTHOR`, or the
command takes it as `--author`. For a listed login, the command judges
neither the keys nor their status and asks the board nothing, while it still
judges the commit order. Every other author is judged in full, another App
included. GitHub gives a login ending in `[bot]` only to an App, so a person
cannot open a pull request under a listed login. Adding an App to the list
takes a change to that file, and no label, trailer or variable extends it.

Run it on a branch before you open its pull request, with the board address
and token from your environment. `origin/main` stands for the base the pull
request will target:

```sh
TRACKER_BASE_URL=https://plain.apps.mitstack.dev TRACKER_API_TOKEN=... \
PR_TITLE="MIT-123: fix the thing" \
  go run ./cmd/pr-delivery --repo . --base origin/main --head HEAD
```

A failing run prints one line per failure, such as
`pr-delivery: MIT-123 is Done on the board, and a pull request may name only an issue that is In Progress or Pull Request`,
and exits 1. A missing `--base` or `--head` exits 2.

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

The runbook `docs/kit-delivery-recorder-credential.md` in
mitstack-eu/mitstack-workspace, a repository private to mitstack, says where
those two values come from and how the token is rotated.

## How a repository runs the commands

A repository pins a release tag of this module, so the commands it runs
change only when it edits that pin. board-automation and plain-block install
`board-delivery` in their `record delivery` job:

```sh
go install github.com/mitstack-eu/plain-delivery/cmd/board-delivery@v0.1.0
```

talos-cluster-kit runs `pr-keys` and `board-delivery` with `go run` at the
same pin:

```sh
go run github.com/mitstack-eu/plain-delivery/cmd/pr-keys@v0.1.0 --repo . --base "$BASE_SHA" --head "$HEAD_SHA"
go run github.com/mitstack-eu/plain-delivery/cmd/board-delivery@v0.1.0 --repo . --before "$BEFORE_SHA" --after "$GITHUB_SHA"
```

`pr-delivery` is built to run with `go run` at a release tag from block-kit's
own `ci.yml`, block-kit's reusable `publish-block.yml` for every block that
calls it, the platform pipeline and the workspace pipeline. Each passes
`TRACKER_BASE_URL` and `TRACKER_API_TOKEN` through the environment, as
`record delivery` does. `publish-block.yml` runs it only when the calling
repository's organisation or repository defines the `TRACKER_BASE_URL`
variable, so a builder's block in an organisation with no board runs the
checks it ran before.

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

The module's one dependency is block-kit's `safe` package, which redacts the
token from a board answer and keeps it on one line. `go.mod` pins block-kit to
one commit, so a bump is an edit there. `ci_test.go` reads
`.github/workflows/ci.yml` and fails when a job stops running these commands
the way the sections above describe.
