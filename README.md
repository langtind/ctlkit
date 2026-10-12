# ctlkit

The command line that every `*ctl` tool is built on.

A tool fills in a table of commands. ctlkit parses the flags from the table, prints help from it, and publishes it with the `schema` command, so the three cannot drift apart. It also supplies error kinds with their exit codes, JSON output, field selection, confirmation, input checks and the skill installer.

The tools are written for AI agents first. [CONVENTIONS.md](CONVENTIONS.md) describes the contract they share.

To start a new tool, use [ctl-template](https://github.com/langtind/ctl-template).

## A command

```go
p := ctlkit.New("petctl", "look after the pets at the kennel", build)
p.Commands = []ctlkit.Command{
	{
		Name:    "pets",
		Summary: "List the pets",
		Flags:   []ctlkit.Flag{ctlkit.LimitFlag(20, 100), ctlkit.JSONFlag, ctlkit.FieldsFlag},
		Run: func(ctx context.Context, c *ctlkit.Call) error {
			pets, err := client.Pets(ctx, c.Int("limit"))
			if err != nil {
				return err
			}
			return c.Print(pets, func(w io.Writer) { renderPets(w, pets) })
		},
	},
}
os.Exit(p.Main())
```

That gives `petctl pets`, `petctl pets --json --fields id,name`, `petctl pets --help`, `petctl help`, `petctl schema`, `petctl version` and `petctl install-skill`.

## Packages

| Package | Holds |
| --- | --- |
| `ctlkit` | `Program`, the command table, `Call`, error kinds, `CheckID` |
| `ctlkit/httpx` | `Do`, which turns an HTTP status into an error kind and never retries |
| `ctlkit/keystore` | One secret in the system keyring, and `Memory` for tests |
| `ctlkit/ctltest` | `Contract`, which holds a tool to the conventions, and `Run` for a tool's own tests |

## What a tool gets

- **Errors with a kind.** `ctlkit.NotFoundf(...)`, `ctlkit.Authf(...)`, `ctlkit.Usagef(...)` or `ctlkit.Errorf(kind, ...)`. Each kind has its own exit code. Add `.WithHint(...)` to say what to do next. With `--json` the error is a JSON document on stdout.
- **`Call.Print(v, render)`.** Prints `v` as JSON with `--json` and calls `render` otherwise. It applies `--fields` and prints an empty list as `[]`.
- **`Call.Confirm(question)`.** `--yes` answers it. Without a terminal it refuses and names the flag.
- **`Call.Secret(label, stdinFlag)`.** Reads a password from stdin or from a prompt without echo.
- **`Arg{ID: true}`.** Refuses an id that holds a query string, an encoded character or a path.

## Test a tool

```go
func TestContract(t *testing.T) {
	ctltest.Contract(t, newProgram, "../../skills/petctl/SKILL.md", "../../README.md")
}
```

`Contract` checks the table, help for every command, the schema, that every advertised flag is accepted and no other, that a changing command takes `--dry-run` and `--yes`, the JSON error document, and that every command in the docs exists.

## Develop

```bash
go test -race ./...
go vet ./...
gofmt -l .
```
