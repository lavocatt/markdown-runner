# markdown-runner — conventions for the bot (coder and reviewer)

markdown-runner is a Go CLI that executes the code chunks of Markdown files as pipelines (stages,
parallel chunks, teardown, breakpoints). The README is itself executed in CI: it is documentation
AND a test.

## Layout
- `main.go`: flags (pflag) and wiring. `config/`: the options.
- `parser/`: Markdown -> stages and chunks (the JSON chunk header, its schema).
- `runner/`: runs stages, breakpoints (`-B`, `--start-from`), teardown.
- `chunk/`: one chunk's command (timeouts, process-tree kill, env).
- `view/`: output (`default` = pterm, `ci` = plain). `stage/`, `runnercontext/`.
- `completion/`: bash completion and its own test suite (`completion/tests/run_all_tests.sh`).
- `test/run.sh` + `test/cases/*.md`: integration tests running the built binary.

## Rules
- **Read before you write.** Find the existing pattern and follow it. No new dependency
  (`go.mod`/`go.sum` are off-limits), no reformatting of code you didn't change, no drive-by
  refactors: change what the issue needs.
- **A test with every change**: a Go test (testify is available) that FAILS without your change, in
  the package's `_test.go`. Visible CLI behaviour -> also a case in `test/cases/` + `test/run.sh`
  when it fits the existing ones. The reviewer runs your tests against main's code: if they pass
  there, they prove nothing and the PR goes back.
- **Never weaken a test** (no deleted or loosened assertion). If behaviour was MEANT to change, say
  so in the summary.
- **Docs**: a new or changed flag is documented in README.md (the `--help` block in README is an
  executed chunk: keep it exact) and in the bash completion.
- **Format**: `gofmt` every Go file you touch; `go vet ./...` clean.
- **Checks to run before you finish**: `go test ./...`, `bash test/run.sh`,
  `./completion/tests/run_all_tests.sh`. The harness runs them again and refuses the round if one fails.
- **Off-limits** (refused by the harness): `.github/`, `.bot/`, `go.mod`, `go.sum`, `LICENSE`.
- Commit messages are written by the harness as `[#<issue>] <subject>`: give a good subject.
- English everywhere.
