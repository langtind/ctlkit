# Upgrading

What each version of ctlkit adds, what it replaces in a tool, and where to look. Nothing here has broken a tool yet: every version so far only adds.

To move a tool up:

```bash
go get github.com/langtind/ctlkit@latest && go mod tidy
go test -race ./... && go vet ./...
```

Then go through the versions between the old and the new below, and replace the tool's own code with what ctlkit has. Run `refresh` twice against the real service afterwards if the login code was touched.

## v0.8.0

| New | Replaces in a tool |
| --- | --- |
| The file store: `CTLKIT_KEYSTORE=file` or `CTLKIT_KEYSTORE_DIR`, with `CTLKIT_KEYSTORE_KEY` | Nothing. A tool that stores with `keystore.Keyring` runs on a server or in a container without a keyring. See "On a server or in a container" in the README |
| `--profile <name>` on every command, and `<PROGRAM>_PROFILE` | Nothing. A tool keeps one login per profile. The default profile keeps the old names, so a stored login still reads |
| `keystore.Describe`, `keystore.CurrentProfile` | "the system keyring" written in `login` and `status`. Say where the login is with `Describe`, and show the profile in `status` |
| `session.LockFile` in the store's directory with the file store | Nothing |

A command of the tool's own may not be called `--profile`. The contract test says so.

## v0.7.0

| New | Replaces in a tool |
| --- | --- |
| `ctlkit.ContentType(path, data)`, used by `Call.Save` | `http.DetectContentType` for a file the tool saves itself. Save now says a `.docx` or an `.xlsx` is one, not `application/zip` |

## v0.6.0

| New | Replaces in a tool |
| --- | --- |
| `keystore.Keyring` keeps a long secret in parts | A store of the tool's own that splits a long cookie over several keyring entries, such as `internal/cli/store.go` in artifikctl. A secret that fits is stored as before, so a stored login still reads |

## v0.5.0

| New | Replaces in a tool |
| --- | --- |
| `ctltest.NewFake`, with `On`, `Handle`, `Calls`, `Sent`, `Changes` | The fake service and the list of calls in `cli_test.go` |
| `ctltest.ChangeNeedsAYes` | The tests that a change sends nothing with `--dry-run` or without `--yes` |
| `ctltest.RefreshRenewsEveryTime` | Nothing. Add it: it finds a renewal that does nothing while the token is good |
| `ctlkit.Write[T]` with `Check` and `Do` | Look before, send once, look after, for a write that is not safe to repeat |
| `httpx.ReadFiles` and `httpx.FileLimits` | Reading and checking the files a flag named |
| `ctlkit.ParseDate`, `ctlkit.DateLayout` | Parsing a `yyyy-mm-dd` flag |
| `ctlkit.LocalTime` | Reading a time that a service wrote without a zone |
| `scripts/` | The scripts an agent wrote to read a web client, a cookie login and the shape of an answer |
| `API.md` | Reading the source for a signature |

## v0.4.0

| New | Replaces in a tool |
| --- | --- |
| `httpx.Form`, `httpx.FormFile` | Building a multipart body by hand |
| `ctlkit.Repeat` | A `List` flag that takes paths or free text. A `List` splits on commas |

## v0.3.0

| New | Replaces in a tool |
| --- | --- |
| `session.Session.Extra`, `Manager.RenewFrom` | Packing a third part of a login into the refresh token |
| `session.JWTExpiry` | Reading `exp` out of a token |
| `httpx.BrowserCookies`, `httpx.CookieCatcher` | Reading a browser's storage state, and a jar that catches `Set-Cookie` |
| `ctlkit.Command.Long` | Steps that were put in a hint |
| `ctlkit.OutputFlag`, `Call.Save`, `ctlkit.FileName` | Writing a fetched file |
| `ctlkit.Quote` | Showing free text from a service as a block quote |
| `Call.ReadAll` | Reading all of stdin |

## v0.2.0

`session.Manager`: a login that renews itself, in the keyring, under a lock. It replaces a bare token in `keystore.Keyring`. A bare token that is already stored is read as an access token that cannot be renewed.
