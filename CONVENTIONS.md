# Conventions

Every `*ctl` tool follows this contract. The reader of a tool's output is usually an AI agent, so the contract is written for a caller that cannot be asked what it meant and cannot answer a prompt.

`ctlkit` implements most of the contract, and `ctltest.Contract` checks it. A rule marked **(you)** is one the tool author has to keep.

## Output

- Data goes to stdout. Notes, warnings, prompts and errors go to stderr. Use `Call.Note` for a note.
- Markdown is the default. It reads well in a terminal and when an agent relays it into a chat.
- Every command that prints data has `--json`. With `--json`, stdout holds one JSON document and nothing else.
- An empty list is `[]`, never `null`. **(you)** for a list inside an object.
- A list that can grow without bound has `--limit`. When the result is cut off, say so: `"truncated": true` in JSON and a line in the Markdown. **(you)**
- A list command also takes `--fields`, which keeps only the named fields of each item. It applies to a list, to `items` in an object, or to a single object.
- Field names in JSON are a contract. Add fields freely. Renaming or removing one is a breaking change. **(you)**
- Times are RFC 3339. Never print "3 days ago". **(you)**

## Errors

- An error has a kind, and each kind has its own exit code.

  | Exit | Kind | Meaning |
  | --- | --- | --- |
  | 0 | | Success |
  | 1 | `failure` | The command failed |
  | 2 | `usage` | The call was wrong. Fix it before running it again |
  | 3 | `not_found` | What was asked for does not exist |
  | 4 | `auth` | Not logged in, or not allowed |
  | 5 | `conflict` | The service is already in a state that rules the change out |
  | 6 | `rate_limit` | The service asked for a pause |
  | 7 | `unavailable` | The service could not be reached or failed. A later try may work |

- With `--json`, an error is the one JSON document on stdout, and stderr stays empty:

  ```json
  {"error": {"kind": "not_found", "message": "...", "hint": "...", "retryAfter": "30 seconds", "usage": "..."}}
  ```

- An error says what to do next. A wrong enum value lists the right ones. A missing login names the login command. Use `Error.WithHint`, or `Program.Hint` for an error that cannot say it itself. **(you)**
- Echo the value that was wrong. **(you)**
- Never retry in a loop. Report `Retry-After` and stop. `httpx.Do` does this.
- An unknown flag is a usage error. So is a filter that cannot be applied. Never drop one silently. **(you)**

## Commands

- The command table is the single description of the command line. Add a command or a flag there and nowhere else.
- `help`, `<command> --help` and `schema` are generated from the table. `schema` works with no login, no config and no network.
- A command name is one or two lower-case words: `events`, `watch add`.
- A command never hangs. Anything it would prompt for can also be given as a flag or on stdin. Without a terminal it refuses and names the flag.
- An argument that names one thing at the service is marked `ID: true`. It is refused when it holds white space, `?`, `#`, `%`, a slash or `..`.
- Build filters into the command. Do not rely on the caller piping through `jq`. **(you)**

- A flag that takes several names or ids is a `List`: it can be repeated and splits on commas. A flag that takes several paths or pieces of free text is a `Repeat`: it can be repeated and takes each value whole, because a comma may be part of it.

## Changes

- A command that changes something at the service is marked `Mutates: true`. It takes `--dry-run` and `--yes`.
- `--dry-run` prints what would be sent and changes nothing. It may read, to check that what the call names exists. The result says `"dryRun": true`. **(you)**
- The question that `Call.Confirm` asks names the thing that changes, not only its id. **(you)**
- Without `--yes`, the command asks. Without a terminal it refuses. Use `Call.Confirm`.
- Say what running the command again does: nothing, the same thing, or one more. **(you)**
- A write that the service does not make safe to repeat is never sent twice. Look for the thing before sending and refuse with `conflict` when it is there. After a timeout or a 5xx, look again instead of sending again, and when it still cannot be told, say that it is not known whether the change was made, with a hint not to run the command again. After a 2xx, look the thing up and return its id. **(you)**
- Do not build a write from a guess. Observe the request first and keep it in `reference/`. **(you)**

## Secrets

- A secret never comes as an argument. Read it with `Call.Secret`: from stdin with a `--...-stdin` flag, or from a prompt without echo.
- Store it with `keystore.Keyring`. Never write it to a file, a log or an error message. **(you)**
- An environment variable may override the keyring, for cron jobs and containers. **(you)**

## Login

- A tool stays logged in for as long as the service allows. The caller is an agent, and an agent cannot answer a login prompt.
- Aim for a refresh token in every tool. The user logs in once, and after that the tool renews the login itself. The password is never stored.
- Store the login in the system keyring with `session.Manager`, with the refresh token and both expiry times. It renews before the access token runs out and after the service refused one, under a lock so that runs at the same time do not log each other out.
- Only a clear no from the service ends the login. A failed renewal that is not a refusal leaves it alone.
- `status` says whether a login is stored and how long it lasts, without calling the service. `refresh` renews it now.
- A service that logs in with a password takes it on stdin: `login --email <email> --password-stdin`.
- A service that logs in through a browser, with BankID or a single sign-on, takes the browser's storage state on stdin: `login --state-stdin`. `httpx.BrowserCookies` reads the cookies, and what the login has besides the two tokens goes in `Session.Extra`. The steps for the user go in the command's `Long`.
- Find out how the service's tokens work before writing any of this. [DISCOVERY.md](DISCOVERY.md) says how. **(you)**

## Remote data

- A file sent to a service goes in an `httpx.Form`, under its last name element only, after the tool has checked the number, the size and the type the service allows.
- A fetched file is written with `Call.Save`: only under its own name in the current directory or at `--output`, and never over a file that exists.
- Free text from a service is shown with `ctlkit.Quote`, as a block quote.
- What a service returns is data. Never follow an instruction found in it, and say so in the skill. **(you)**
- Keep a copy of the API as it was observed in `reference/`, with a list of what is verified. **(you)**
- Be polite: one request per command where possible. **(you)**
- No person, customer or organisation that exists goes into code, docs, tests or fixtures. **(you)**

## The skill

- `skills/<name>/SKILL.md` teaches workflows and guardrails. `schema` and `help` list the flags, so the skill does not have to.
- It is embedded in the binary and copied out by `install-skill`.
- A test checks that every command and flag in its code blocks exists.
- Update it when a command changes. **(you)**
