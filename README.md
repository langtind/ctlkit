# ctlkit

The command line that every `*ctl` tool is built on.

A tool fills in a table of commands. ctlkit parses the flags from the table, prints help from it, and publishes it with the `schema` command, so the three cannot drift apart. It also supplies error kinds with their exit codes, JSON output, field selection, confirmation, input checks and the skill installer.

The tools are written for AI agents first. [CONVENTIONS.md](CONVENTIONS.md) describes the contract they share. [DISCOVERY.md](DISCOVERY.md) describes how to find out how a service works before its client is written: where to look, what to find out about login and tokens, and what earlier tools got wrong.

[API.md](API.md) has everything a tool uses on one page, [UPGRADING.md](UPGRADING.md) says what each version adds and what it replaces in a tool, and [`scripts/`](scripts/) holds the scripts for reading a web client, a cookie login and the shape of an answer.

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
| `ctlkit` | `Program`, the command table, `Call`, error kinds, `CheckID`, `Quote` |
| `ctlkit/httpx` | `Do`, which turns an HTTP status into an error kind and never retries. `BrowserCookies` and `CookieCatcher` for a login that lives in cookies. `Form` for a multipart body with JSON and files |
| `ctlkit/keystore` | One secret in the system keyring or an encrypted file, per profile, and `Memory` for tests |
| `ctlkit/session` | A login that renews itself: access token, refresh token, `Extra` for what else it has, and renewal under a lock |
| `ctlkit/ctltest` | `Contract`, which holds a tool to the conventions, `NewFake`, a service that does not exist, and the checks every tool runs |

## What a tool gets

- **Errors with a kind.** `ctlkit.NotFoundf(...)`, `ctlkit.Authf(...)`, `ctlkit.Usagef(...)` or `ctlkit.Errorf(kind, ...)`. Each kind has its own exit code. Add `.WithHint(...)` to say what to do next. With `--json` the error is a JSON document on stdout.
- **`Call.Print(v, render)`.** Prints `v` as JSON with `--json` and calls `render` otherwise. It applies `--fields` and prints an empty list as `[]`.
- **`Call.Confirm(question)`.** `--yes` answers it. Without a terminal it refuses and names the flag.
- **`Call.Secret(label, stdinFlag)`.** Reads a password from stdin or from a prompt without echo.
- **`Call.Save(data, name)`.** Writes a fetched file under its own name or at `--output`, never outside the current directory by a name from the service, and never over a file that exists.
- **`Command.Long`.** Help that does not fit in a summary, shown by `help <command>` and in the schema.
- **`Arg{ID: true}`.** Refuses an id that holds a query string, an encoded character or a path.

## On a server or in a container

The system keyring needs a desktop session. A Linux server or a container has none (there is no Secret Service on D-Bus), so `keystore.Keyring` fails there. Use the file store instead. Every tool built on ctlkit reads the same variables:

```bash
export CTLKIT_KEYSTORE_DIR=/data/keystore              # one directory for all the tools: /data/keystore/<program>/
export CTLKIT_KEYSTORE_KEY="$(openssl rand -base64 32)"  # make it once and keep it with the machine's other secrets
```

`CTLKIT_KEYSTORE=file` chooses the file store under `$XDG_STATE_HOME/<program>` (or `~/.local/state/<program>`) instead of a directory of its own. `CTLKIT_KEYSTORE=keyring` is the default.

- Each secret is one file, readable by its owner only, sealed with AES-256-GCM. The program and the name of the secret are sealed in with it, so a file copied to another name or tool does not open.
- A file is replaced in one step, a new file renamed over the old one, so a run that dies while it writes leaves the old login whole. A service that rotates refresh tokens must never lose the new one.
- The renew lock is in the same directory, so runs that share the files also share the lock. It is an operating system lock, so a run that is killed lets go of it at once.
- A directory that others can write to (mode with `g+w` or `o+w`) is refused, not changed: it may be a volume set up on purpose. Make it private with `chmod go-w`.
- With the wrong key nothing opens, and nothing is overwritten. Set the right key, or remove the file and log in again.
- There is no way to store in the clear. The key protects a copy of the directory, such as a backup. It does not protect against someone who can read the key, so keep it out of the directory and out of the image.

In a container, log in once with a terminal attached, and let cron do the rest:

```bash
docker exec -it <container> <program> login
```

Never copy a login from another machine. A service that rotates its refresh tokens ends the login on both when the copy renews.

## Profiles

`--profile <name>`, before or after the command, or `<PROGRAM>_PROFILE=<name>`, keeps a login of its own under that name: two people's accounts on one machine. Names are lower-case letters, digits and dashes. The default profile is `default` and keeps the names a login had before profiles. All profiles of a program share one renew lock.

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
