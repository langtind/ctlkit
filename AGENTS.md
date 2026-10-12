# AGENTS.md

ctlkit is the Go module every `*ctl` tool is built on. It holds the command table, the dispatcher, help, the `schema` command, error kinds, JSON output, field selection, confirmation, input checks and the skill installer.

## Rules

- [CONVENTIONS.md](CONVENTIONS.md) is the contract. A change to behaviour changes that file and `ctltest.Contract` in the same commit.
- Every tool depends on this module. Keep exported names stable. A rename or a changed output is a breaking change for all of them.
- Nothing about one service goes in here. If only one tool needs it, it belongs in that tool.
- Dependencies are `go-keyring` and `x/term`. Add another only when it removes real code.
- No long dash characters in code, docs or strings.

## Layout

- `program.go` holds `Program`, the dispatcher and the error report.
- `command.go` holds the table types, flag parsing and `Input`.
- `call.go` holds `Call`: printing, `--fields`, confirmation and secrets.
- `errors.go` holds the error kinds and their exit codes. The `kinds` table is the one list, read by help and schema.
- `help.go`, `schema.go` and `skill.go` hold the built-in commands.
- `validate.go` holds the input checks.
- `httpx/` holds the HTTP plumbing, `keystore/` the keyring, `ctltest/` the contract test.
- `ctlkit_test.go` builds a small tool, `petctl`, and runs the contract and the behaviour tests against it.

## Commands

```bash
go test -race ./...
go vet ./...
gofmt -l .
```

CI also runs `golangci-lint` with the settings in `.golangci.yml`.

A release is a `v*` tag. Tools pick it up with `go get github.com/langtind/ctlkit@latest`.
