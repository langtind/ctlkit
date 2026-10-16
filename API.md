# The API on one page

Everything a tool uses from ctlkit, with one example each. Read this before you open the source. `go doc github.com/langtind/ctlkit/<package>` has the rest.

## ctlkit: the command line

```go
p := ctlkit.New("petctl", "look after the pets", build)   // build is ctlkit.Build{Version, Commit, Date}
p.URL, p.Skill, p.Hint = "https://github.com/langtind/petctl", skillFS, hint
p.Commands = []ctlkit.Command{{
	Name:    "pets",                  // one word, or two: "pet show"
	Summary: "List the pets",
	Long:    "Shown by help and schema. Full sentences.",
	Args:    []ctlkit.Arg{{Name: "pet-id", Required: true, ID: true, Help: "the pet"}},
	Flags:   []ctlkit.Flag{ctlkit.LimitFlag(20, 100), ctlkit.JSONFlag, ctlkit.FieldsFlag},
	Mutates: false,                   // true needs DryRunFlag and YesFlag
	Run:     func(ctx context.Context, c *ctlkit.Call) error { ... },
}}
os.Exit(p.Main())
```

**Flags.** `Flag{Name, Kind, Value, Default, Enum, Help}`. `Help` is a fragment: lower case, no full stop.

| Kind | Read with | Takes |
| --- | --- | --- |
| `Bool` | `c.Bool("yes")` | `--yes` |
| `String` | `c.String("from")` | one value, or one of `Enum` |
| `Int` | `c.Int("limit")` | a number |
| `List` | `c.List("fields")` | names or ids. Repeats, splits on commas, drops repeats |
| `Repeat` | `c.List("attach")` | paths or free text. Repeats, takes each value whole |

Ready made: `JSONFlag`, `FieldsFlag`, `YesFlag`, `DryRunFlag`, `OutputFlag`, `LimitFlag(default, highest)`.

**Call.** What `Run` gets.

| Call | Does |
| --- | --- |
| `c.Arg(0)`, `c.Given("from")`, `c.Has("json")` | The input. `Given` tells a flag that was set from its default |
| `c.Print(v, func(w io.Writer) { ... })` | `v` as JSON with `--json`, else the Markdown the function writes. Applies `--fields` |
| `c.PrintJSON(v)` | One JSON document, for a command that prints nothing else |
| `c.Note("...")` | A line on stderr that is not data |
| `c.DryRun()`, `c.JSON()` | Whether the flag was given |
| `c.Confirm("Rename thing t1 to \"Kedge\"?")` | Asks. `--yes` answers. Without a terminal it refuses |
| `c.Ask("Email", "email")` | Prompts for what a flag did not give |
| `c.Secret("Password", "password-stdin")` | A secret from stdin or a prompt without echo |
| `c.ReadAll()` | The rest of stdin, such as a browser's storage state |
| `c.Save(data, name)` | Writes a fetched file at `--output` or under its own name. Never replaces |

**Errors.** Return them. The kind decides the exit code.

```go
return ctlkit.NotFoundf("no pet has the id %q", id).WithHint(`run "petctl pets" to see the ids.`)
```

| Maker | Kind | Exit |
| --- | --- | --- |
| `fmt.Errorf`, `errors.New` | `failure` | 1 |
| `ctlkit.Usagef` | `usage` | 2 |
| `ctlkit.NotFoundf` | `not_found` | 3 |
| `ctlkit.Authf` | `auth` | 4 |
| `ctlkit.Errorf(ctlkit.KindConflict, ...)` | `conflict` | 5 |
| `ctlkit.Errorf(ctlkit.KindRateLimit, ...)` | `rate_limit` | 6 |
| `ctlkit.Errorf(ctlkit.KindUnavailable, ...)` | `unavailable` | 7 |

`ctlkit.KindOf(err)` reads the kind. `%w` keeps it through a wrap.

**A write that is not safe to repeat.**

```go
write := ctlkit.Write[Application]{
	What: "an application for 15 October",
	List: "skolectl applications",
	Find: func(ctx context.Context) (*Application, error) { ... },   // nil when it is not there
	Send: func(ctx context.Context) error { ... },                   // called once
}
if err := write.Check(ctx); err != nil { return err }     // conflict when it is there. A dry run calls it too
if err := c.Confirm("..."); err != nil { return err }
made, err := write.Do(ctx)                                 // sends once, then looks it up
```

**Small things.** `ctlkit.CheckID(what, value)`, `ctlkit.ContentType(path, data)` for what a saved file holds (a `.docx` or an `.xlsx` by its extension, not as a zip), `ctlkit.Quote(text)` for free text from a service, `ctlkit.FileName(name, data)`, `ctlkit.ParseDate("from", value, zone)`, `ctlkit.LocalTime(text, zone)` for a time written without a zone.

## httpx: requests

```go
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
body, err := httpx.Do(client, req, "Kennel", userAgent)   // a status outside 2xx is an error with a kind
if httpx.Status(err) == http.StatusUnauthorized { ... }    // renew and send once more
```

`httpx.Do` never retries. `httpx.NewClient()` has the shared timeout.

```go
var form httpx.Form
form.JSON("request", request)
for _, file := range files { form.File("attachments", file) }
body, contentType, err := form.Body()

files, err := httpx.ReadFiles("attach", c.List("attach"),
	httpx.FileLimits{Max: 3, MaxBytes: 5 << 20, Types: []string{"application/pdf", "image/png"}})

cookies, err := httpx.BrowserCookies(state, "app.example.com")   // from a browser's storage state

caught := httpx.CookieCatcher{}                                   // see what Set-Cookie an answer has
client := *c.HTTP
client.Jar = caught
```

## session and keystore: the login

```go
login := &session.Manager{
	Store:    keystore.Keyring("petctl", "token"),
	LockPath: session.LockFile("petctl"),
	Renew:    client.RenewSession,   // func(ctx, refreshToken string) (session.Session, error)
	Margin:   time.Minute,           // default five minutes. Lower it for a short access token
}
login.Save(session.Session{AccessToken, AccessExpires, RefreshToken, RefreshExpires, Extra})
token, err := login.Token(ctx)            // renews first when it is about to run out
token, err = login.Refresh(ctx, stale)    // after the service answered 401 to stale
s, err := login.Load()                    // without renewing, for status
login.Clear()
```

`RenewFrom func(ctx, stored session.Session)` is `Renew` for a service that needs more than the refresh token, such as a cookie kept in `Extra`. A renewal that returns no refresh token or no `Extra` keeps the stored ones. `session.ErrNotLoggedIn` and `session.ErrExpired` are what `errors.Is` finds. `session.JWTExpiry(token)` reads `exp`. `keystore.Keyring` keeps a secret too long for one keychain entry, over 1800 bytes, in parts (`token`, `token.1`, `token.2`). `keystore.Memory` is the store for tests.

## ctltest: the tests

```go
func TestContract(t *testing.T) {
	ctltest.Contract(t, build, "../../skills/petctl/SKILL.md", "../../README.md")
}

fake := ctltest.NewFake(t)                                  // a service that does not exist
fake.On("GET /pets", http.StatusOK, `[{"id":"p1"}]`)
fake.Handle("POST /login/refresh", func(c ctltest.Call) ctltest.Answer { ... })
code, stdout, stderr := ctltest.Run(build(), stdin, "pets", "--json")
ctltest.OneJSON(t, stdout, &pets)                           // one JSON document and nothing else
fake.Sent()                                                 // "GET /pets, GET /pets/p1"
fake.Calls[0].Token, .Cookies, .Body, .Form, .Files         // what was sent
fake.Changes("POST /login/refresh")                         // requests that were not a GET

ctltest.ChangeNeedsAYes(t, build, fake, []string{"rename", "p1", "Rex"}, "POST /login/refresh")
ctltest.RefreshRenewsEveryTime(t, build, stored)            // stored returns the session in the store
```

`ctltest.ErrorDocument` is the JSON a failed `--json` run prints. The template's `cli_test.go` shows all of it in use.
