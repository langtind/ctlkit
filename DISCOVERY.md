# Discovery

How to find out how a service works before you write its client. Most services a `*ctl` tool wraps have no public API, so the facts have to be observed.

This file is a guide, and a log. When a tool teaches you something that would have saved time on the one before, add it under "Lessons".

The checklist comes first, then where to find the answers, then how to keep a login, and last the lessons, which grow. The scripts it names are in [`scripts/`](scripts/) in ctlkit. Each prints what it does when run without arguments. Use them instead of writing your own: they are tested, and they keep values out of the conversation.

| Script | Does |
| --- | --- |
| `bundle-fetch <page-url> <new-dir>` | Downloads a web client's script files, chunks and all |
| `bundle-paths <dir> [filter]` | Lists what looks like request paths in them |
| `bundle-grep <dir> <text> [characters]` | Shows what stands around a string: the method, the body, the form's checks |
| `json-shape` | Prints the shape of a JSON answer on stdin, and none of what it says |
| `cookie-matrix <state.json> <cookie-host> <url>` | Sends one request for every combination of cookies, and prints status and the names of what was set |

## What to find out

Answer these before the client is written. Write the answers in `reference/README.md`.

**Login**

- The path, the body, and what a wrong password answers.
- What an account with two-factor login answers. It may be a 200 with a challenge and a phone number in it. Never print that answer.
- Whether a captcha or a client id stands in the way.

**Tokens**

- Every token the login answers with, and when each runs out. Read the expiry fields in the answer. Do not assume there is one token.
- Whether there is a refresh token. If the login answer has one, there is a way to use it.
- The renewal request: path, body, and what it answers.
- Whether renewal hands out a new refresh token. If it does, the old one works once, and the new pair must be stored before anything else happens.
- What a refused renewal looks like. That, and only that, means the user has to log in again.
- When the service's own client renews: before the token runs out, on a 401, or both.

**Requests**

- Headers the service insists on: a client id, a client version, a user agent.
- The order of a list, and how to change it. Check it together with the limit.
- Paging, the highest limit, and what happens past it.
- Rate limits, and whether a 429 carries `Retry-After`.
- The shape of an error body, so that the message can be shown.
- How times are written, in answers and in queries.
- Whether a file is fetched with the token or without it, and from which host.

**Writes**

- Observe the request. Do not build one from a guess or from somebody else's docstring.
- What running it twice does, and whether the service has an idempotency key or a draft step. If it has neither, the tool looks before and after it sends, see "Changes" in CONVENTIONS.md.
- The rules the web client's form checks before it sends: required fields, lengths, date limits, and for files the number, the size and the types. The service may not check them itself, so the tool does.
- Run it for real only with the consent of whoever it concerns.
- A write that cannot be tried, because it reaches other people, is read out of the web client and stays under "Unverified" in `reference/`. Tell the user that the first real use is the test.

## Where to look, cheapest first

Stop at the first source that answers the question. Each step down costs more setup and puts more of other people's data in front of you.

| Source | Good for | Cost |
| --- | --- | --- |
| 1. Official docs, an OpenAPI file, a developer portal | Everything, when it exists | None |
| 2. An open client somebody else wrote | Paths, parameters, field names | None. Take facts only, and check its licence before you copy a line |
| 3. The service's own web client | What the service's own code does: login, renewal, headers, retries | None. No proxy and no certificate |
| 4. Live answers from the tool itself | Shapes, lifetimes, ordering, limits | One login |
| 5. Sniff the traffic, on your own or with the user's help | A service that has no web client | A proxy, a certificate on the phone, and it fails when the app pins its certificate |

An open client is a lead, not a fact. It shows what worked for its author when it was written. It is often wrong about what is missing: see the first lesson below.

### 3. Read the web client

The web client is the service's own code, and it is public. This is the fastest way to learn how login and renewal work, because the code says it outright. It also holds every request the service has, and the rules its forms check before they send.

```bash
S=~/Developer/Private/ctlkit/scripts
$S/bundle-fetch https://example.com/app/ bundle      # every script file, chunks and all
$S/bundle-paths bundle 'api|/v[0-9]'                 # what looks like a request path
$S/bundle-grep bundle '/auth/refresh' 400            # what stands around one of them
```

A web client is often split into a hundred chunks, and the paths are rarely in the first file. `bundle-fetch` follows the names from file to file. A minified file is one long line, so plain `grep` prints all of it or nothing. `bundle-grep` prints some characters on either side of each hit.

Search for: `refresh`, `renew`, `token`, `Authorization`, `login`, `/auth`, `expiration`, `X-`, and the API host. Read the whole function around a hit. It shows the request body, which fields of the answer are kept, and what the client does on a 401.

To watch the client work, drive it in a browser and read its requests. `playwright-cli` records them without a proxy:

```bash
playwright-cli open https://example.com/app --headed --persistent --profile ./profile
# the user logs in themselves, then:
playwright-cli requests          # numbered list
playwright-cli request 14        # one request in full
playwright-cli response-body 14
```

The user logs in. A password never passes through the agent.

### 4. Ask the tool itself

Give a read command a `--raw` flag that prints the answer as it came, and look at the shape with `jq`:

```bash
tmplctl thing <id> --raw | jq 'keys'
tmplctl thing <id> --raw | jq '{attachments, responseKeys: (.responses | keys)}'
```

Or print the shape of the whole answer, with every value replaced by its type and length:

```bash
tmplctl thing <id> --raw | ~/Developer/Private/ctlkit/scripts/json-shape
```

Look at keys and types. Do not print a whole answer into the conversation or into a file in the repo: it holds real people. Use `json-shape` from the first request, so that nothing private is in the conversation to begin with.

### 5. Sniff the traffic

When the steps above do not answer the question: the service has no web client, the app does something the web client does not, or the bundle is too obscured to read.

**Try on your own first.** Much can be captured without anyone's help:

- The web client in a browser you drive yourself, with `playwright-cli requests` as shown above. The user logs in once, and the rest is yours.
- A desktop app or a command line tool, by starting it behind a proxy: `HTTPS_PROXY=http://127.0.0.1:8080 <command>`, with the proxy's certificate trusted.
- An iOS simulator or an Android emulator on this machine, when the app can be installed there.

**Then ask the user for help.** A phone app with a real account needs a person: someone has to hold the phone and tap. Do not give up, and do not guess the request instead. Ask, and make the ask small and exact:

1. Say what you need to see and why: "I need the request the app sends when you accept an invitation."
2. Say what to set up, or set up the part you can. The user has Proxyman.
3. Ask them to do the one thing in the app: log in, open a list, accept and then undo. Name the taps.
4. Ask them to say when it is done, then read what was captured.

A capture of a login is the most useful one there is. It shows every token, and a second start of the app a day later shows the renewal.

**Tools**

| Tool | Use it when | Notes |
| --- | --- | --- |
| Proxyman | A person captures from an iPhone or an Android phone | The user has it. It walks through the certificate on the phone, and it has an MCP server (`/Applications/Proxyman.app/Contents/MacOS/mcp-server`) and `proxyman-cli`, so the agent can read the flows the user captured. Keep its redaction on. The app has to be open, and bodies come as previews |
| mitmproxy | The agent captures on its own, with no window | `brew install mitmproxy`. `mitmdump -w flows` writes to a file, and a Python addon can strip values so that only shapes are written. `mitmproxy2swagger` turns a flow file or a HAR into a draft OpenAPI file. Not used in any tool here yet |
| `playwright-cli` | The service has a web client | No proxy and no certificate |

**Capturing from a phone, in short**

1. The phone and the Mac are on the same network. The proxy runs on the Mac.
2. On the phone, set the Wi-Fi proxy to the Mac's address and the proxy's port.
3. Install the proxy's certificate on the phone and trust it. On iOS that is two places: install the profile, then turn it on under Settings, General, About, Certificate Trust Settings. On Android 7 and later an app trusts a user certificate only if it was built to, so many apps show nothing.
4. Turn on SSL proxying for the service's hosts only.
5. Do the thing in the app.
6. Turn the phone's proxy off again. Say so to the user, or their phone has no network once the Mac sleeps.

When the app shows a network error as soon as the proxy is on, it pins its certificate. Then the proxy will not work without changing the app. Go back to the web client, or to an older or a different client of the same service.

**Afterwards.** A capture holds a login token and other people's data. Extract the request and the field names, write them in `reference/`, and delete the capture. Never commit one. `.gitignore` in the template already refuses `*.har` and `*.proxymansession`.

## Keep secrets and people out of the conversation

The agent must be able to work with a login without ever seeing it.

Pipe a password from the password manager straight into the tool:

```bash
op read "op://vault/item/password" | tmplctl login --email <email> --password-stdin
```

Read the shape of a login answer, and nothing else:

```bash
op read "op://vault/item/password" \
  | jq -Rc '{email: "<email>", password: .}' \
  | curl -s -X POST -H 'Content-Type: application/json' --data-binary @- https://api.example.com/login \
  | jq '{keys: keys, access: (.accessToken | {keys: keys, expiration}), refresh: (.refreshToken | {keys: keys, expiration})}'
```

The password goes in on stdin and never on a command line. `jq` lets through the key names and the expiry times only.

Tell whether a stored token changed without printing it:

```bash
security find-generic-password -s tmplctl -a token -w \
  | sed 's/^go-keyring-base64://' | base64 -d | jq -r .refreshToken | shasum | cut -c1-10
```

Use a token in a one-off request without printing it:

```bash
T=$(security find-generic-password -s tmplctl -a token -w | sed 's/^go-keyring-base64://' | base64 -d | jq -r .accessToken)
curl -s -H "Authorization: Bearer $T" "https://api.example.com/things?max=4" | jq '[.[].startTimestamp]'
```

## Stay logged in

An agent cannot answer a login prompt, so a tool has to keep itself logged in for as long as the service allows. Aim for this in every tool: the user logs in once, and after that the tool renews the login itself.

- **Find the refresh token.** Nearly every service that logs in with a password has one. Look for it in the login answer before you settle for an access token that runs out. If the answer has none, look at what the service's own client does when its token runs out: it does something, and that is the renewal.
- **Find the renewal request.** The web client's code says it outright. Failing that, a capture of the app being opened a day after login shows it.
- **Store the whole login in the system keyring, and nowhere else.** `session.Manager` over `keystore.Keyring` keeps the access token, the refresh token, and when each runs out, in the macOS Keychain or its counterpart. Never in a file, an environment file, a log or an error message.
- **Never store the password.** The refresh token is what keeps the tool logged in. A password manager with a fingerprint prompt is fine for the first login, and wrong for every day: an agent cannot press the sensor.
- **Renew at the right times.** `Manager.Token` renews when the access token is about to run out. `Manager.Refresh` renews after the service refused a token, and the client then sends the request once more.
- **Renew under the lock.** An agent runs several commands at the same time, and a service that rotates refresh tokens would log out every run but one. `session.Manager` does this when it has a `LockPath`.
- **Only a clear no ends the login.** A timeout or a 503 during renewal says nothing about it, and the stored login is left alone.
- **Give the tool `status` and `refresh`.** `status` says how long the login lasts without calling the service. `refresh` renews now. The template has both.
- **Say when the user is needed.** When the refresh token itself runs out, the user logs in again. Say so in the error, and say in `status` when that will be.

### A login that needs a browser

BankID, ID-porten, Feide and other single sign-ons cannot be done by a tool. The login the browser ends up with can still be taken over, and it usually renews itself.

1. Open the login page in a browser the user can see: `playwright-cli open <url> --headed`.
2. Ask the user to log in, and to say so the moment they are in.
3. Save the state at once: `playwright-cli state-save state.json`. Session cookies go when the browser does.
4. List the cookies by name, length and expiry, never by value: `jq '[.cookies[] | {name, domain, expires, length: (.value | length)}]' state.json`.
5. Find which cookies a request needs and which ones renew: `scripts/cookie-matrix state.json <cookie-host> <url>`. It prints names and statuses only. Run it once at once and once after the access token has run out, since a renewal shows only then.
6. Read them into the keyring: `<tool> login --state-stdin < state.json`, and delete `state.json`.

Do not wait for `playwright-cli close` to return: it has been seen to hang. Run it in the background or leave the browser to the user. Never copy a stored login from one machine to another yourself. Give the user the commands to log in there.

When the renewal is found, send only what it needs. A service that renews on any request hands out a new access token only when the one it was sent no longer works, so a renewal that carries the stored access token does nothing. `ctltest.RefreshRenewsEveryTime` checks this against the fake, and `refresh` twice in a row checks it against the real service. Run both again after every change to the login code.

In the tool, `httpx.BrowserCookies(state, host)` reads the cookies for one host, `session.JWTExpiry` reads when a cookie that is a JWT runs out, and `Session.Extra` holds a part that is neither token, such as a session id. A service that renews in `Set-Cookie` is read with `httpx.CookieCatcher` as the jar of a copy of the client, and renewed with `Manager.RenewFrom`, which gets the whole stored session. Set `Manager.Margin` below the lifetime of the access token.

`Manager` keeps `Extra` through a renewal it runs itself. A `login` that calls the client's renewal directly, to check the cookies before it stores them, gets a session without `Extra` back and has to copy it over before `Save`.

The template's `login` has both ways. Delete the one the service does not have.

If a service truly has no way to renew a login, write that in `reference/README.md` with what you checked, so that the next person does not look again.

## Lessons

**An open client can be wrong about what is missing (spondctl, 2026-10).** The Python client for Spond says in its code that a token cannot be renewed. The login answer holds three tokens: an access token for 24 hours, a refresh token for 90 days, and a password token for 10 minutes. The renewal request was in Spond's own web client, found in ten minutes: `POST /auth2/login/refresh` with `{"value": <refresh token>}`. Read the login answer yourself.

**A list has an order, and the limit cuts from the wrong end (spondctl, 2026-10).** Spond lists events latest first. The tool asked for 21 events from today, sorted them, and showed the "next" 20, which were the 20 furthest away. `order=asc` fixed it. A test with a fake service could not have caught this, because the fake answered in whatever order the test wrote. Check the order of every list against the real service, with a limit lower than the number of items.

**A login that needs a person can still renew itself (skolectl, 2026-10).** Visma Flyt Skole logs in with ID-porten, which no tool can do. The login the browser ends up with was three cookies: an access token for five minutes, a refresh token, and a session id. The tool takes them over: the user logs in once in a browser the agent opened with `playwright-cli open <url> --headed`, `playwright-cli state-save state.json` writes the cookies, and `login --state-stdin < state.json` reads them into the keyring. List the cookies before you conclude that a browser login cannot be kept: name, length and expiry, never the value.

**Renewal may not be a request of its own (skolectl, 2026-10).** The web client had no renewal code at all, and treated a 401 as logged out. The server renews instead: any request sent with the refresh token cookie is answered with a new access token in `Set-Cookie`. To find such a thing, wait until the access token has run out, send one request with a cookie jar (`curl -b jar -c jar`), and look at which cookies changed. Then send the cookies one subset at a time to learn which ones a request needs and which ones renew.

**Leave the access token out of a renewal in `Set-Cookie` (skolectl, 2026-10).** A service that renews on any request hands out a new access token only when the one it was sent no longer works. With the stored access cookie in the request, `refresh` did nothing until that token had run out. Send the refresh token and whatever else the renewal needs, and nothing more. Run `refresh` twice in a row while the token is still good, and check that the expiry moves both times.

**A token may hide in a cookie (skolectl, 2026-10).** A cookie whose value has two dots is a JWT. Decode the middle part and read `exp`, `iat`, `client_id` and `scope`, and leave `sub` and the names alone. That gave the lifetime of the access token before a single request had been timed.

**A login can have more than two parts (skolectl, 2026-10).** A third part that never changes, such as a session id, goes in `Session.Extra`, and `Manager.RenewFrom` hands it to the renewal. Before ctlkit v0.3.0 it had to be packed into the refresh token. Set `Manager.Margin` when the access token is short: the default of five minutes renews a five minute token on every run.

**An unknown id is not always a 404 (skolectl, 2026-10).** Visma Flyt Skole answers 500 for a thread that does not exist and 403 for a child that is not the user's. Try a made-up id against every kind of path before you write the hints, and say in the hint that the id may be wrong.

**The web client's bundle is split into chunks (skolectl, 2026-10).** The first page named 12 script files and they named 150 more. Follow `chunk-XXXXXXXX.js` names from file to file until no new one turns up, join them, and search the whole. The paths were in template strings next to the HTTP method: a search for `httpService.<method>({url:` listed every request the service has.

**Print shapes, not answers (skolectl, 2026-10).** A ten line script that replaces every string with its length and every time with nines shows the shape of an answer without showing a child's name. Use one from the first request, so that nothing private is in the conversation to begin with.

**The browser of a login may not stay open (skolectl, 2026-10).** The `playwright-cli` browser was gone twice during one login, and it was never found out why. Its cookies were session cookies, so they went with it. Save the state as soon as the user says they are in.

**Time how long a refresh token lasts unused (skolectl, 2026-10).** The service did not say, so the tool was left alone and tried again: the login still renewed after 11 hours without a request. Write the longest gap that worked in `reference/README.md`, and raise it when a longer one works.

**An agent may not be allowed to write a secret (skolectl, 2026-10).** `op read ... | gh secret set HOMEBREW_TAP_TOKEN` was refused when the agent ran it, although the secret never passed through the conversation. Do not look for a way around it. Give the user the one line to run themselves, with `!` in front in Claude Code.

**A write can be read out of the web client without being sent (skolectl, 2026-10).** The bundle gave the path, the multipart body, the form's rules and the file limits for a leave application, and the read requests next to it were checked live. The write itself stays under "Unverified" in `reference/` until the user has agreed to one real one.

**A login page may be another door to the same service (skolectl, 2026-10).** A new address turned out to be the school's login page for staff, with a link to the portal the tool already used. Follow the link before you conclude that the service has moved.

**A list flag splits on commas (2026-10).** A `List` flag takes `a,b` as two values and drops repeats, so a file path with a comma in it could not be given to `--attach`. From ctlkit v0.4.0 a flag for paths or free text is a `Repeat`, which takes each value whole.

**go-keyring wraps long values (2026-10).** On macOS, a value that is long comes back from `security find-generic-password -w` as `go-keyring-base64:` followed by base64. Strip the prefix and decode before you use it by hand.

**A file address is not always behind the login (spondctl, 2026-10).** Attachments on Spond are served without a token from another host than the API. Fetch a file only from the origin you expect, and never send the token along with an address that came out of an answer.

**`go install` of a module that needs a private module fails for everyone else.** A public tool needs a public ctlkit. CI fails the same way, with `could not read Username for 'https://github.com'`.

**A local Homebrew tap is a copy.** After a release, `brew install` does not find a new cask until `brew update` has run.

**Ask a GraphQL service for its own schema first (checkinctl, 2026-10).** Checkin's `/graphql` answered an introspection query without a login: `{ __schema { queryType { fields { name } } mutationType { fields { name } } } }`. That gave every query, every mutation and every input type in one request, before any page had been read. Save the full schema to a scratch file and search it, also for arguments named `hash`, `token` or `key`, which show what a link or a code gives access to.

**A service may describe itself for agents (checkinctl, 2026-10).** Checkin has no public API docs, and a web search found none. Its own `llms.txt` said outright that there is an official API for organisers, that the docs are given on request, and that there is an MCP server. Fetch `/llms.txt` on the service's sites as part of step 1, and say in `reference/` who the official API is for.

**The login may be a link (checkinctl, 2026-10).** A participant at Checkin has no account. The link in the confirmation email holds a ticket number and a short code, and the code is all the service asks for. Such a link is a secret: read it with `Call.Secret` from stdin or a prompt, keep it in the keyring under a name the user gives it, and never print it. There is no token and nothing to renew, so the tool has no `refresh` and does not use `session.Manager`.

**A wrong link may be a redirect, not an error (checkinctl, 2026-10).** A wrong code and an unknown ticket both answered `302` to the front page, which a client that follows redirects reads as a page that loaded. Turn redirects off for such a request, with `CheckRedirect` returning `http.ErrUseLastResponse` on a copy of the client, and say in the hint that the two cases cannot be told apart.

**A refused write may be a 200 (checkinctl, 2026-10).** Checkin answers a change it will not make with HTTP 200 and a warning, `full`, next to the unchanged ticket. Try a write that must fail, with the user's yes, before you build on what the web client seems to do: the test showed that the ticket keeps what it had, which the page's script only hinted at.

**ctltest cannot tell a GraphQL read from a change (checkinctl, 2026-10).** `ctltest.ChangeNeedsAYes` and `Fake.Changes` take every request that is not a GET for a change, and a GraphQL read is a POST to the same path as a mutation. Until ctlkit has a way to tell them apart by the body, the tool writes that check itself. `ctlkit.Write` also does not fit a change that is tried again each time a place opens, when a refused try is known to change nothing.

**A GraphQL API may describe itself without a login (tibberctl, 2026-10).** Tibber's API answers an introspection query with no token at all. One `curl` with `{ __schema { types { name kind fields { name args { name } type { name } } } } }` gives every type, field, argument and enum value, which is more than its documentation page shows, since that page is a script that a plain fetch reads as empty. Try it before you read an open client or a web client. Look at the enums in particular: Tibber has quarter-hourly prices but no quarter-hourly consumption.

**A GraphQL error comes with a 200 (tibberctl, 2026-10).** The reason is in `errors[0].message`, and the kind, when there is one, in `errors[0].extensions.code`, such as `UNAUTHENTICATED`. `httpx.Do` sees a 2xx and returns the body, so the client has to read `errors` itself and give the error its kind. A GraphQL service that also answers 4xx puts the message in `errors`, where `httpx.Do` does not look: the error then says only `HTTP 400`.

**A React Router or Remix web client is its own API (artifikctl, 2026-10).** Artifik has no API host: every page has a loader, and `GET <page>.data` answers the loaders' data as a turbo-stream (`text/x-script`). `?_routes=<route id>` asks for one route and leaves out the root and the translations, which were 30 kB a request. The route ids, their paths and which ones have a loader are in the build manifest, `assets/manifest-<hash>.js`. A route whose chunk is empty has its work on the server, so the bundle says nothing about it: read the loader's answer instead. The format: the first line is a flat JSON array, element 0 is the root, `{"_k": v}` is the string at index k pointing at the element at index v, array elements are indices, negative numbers are constants (-5 null, -7 undefined). A redirect is a 202 with `[["SingleFetchRedirect",1],...]`, which is what a page behind the login answers when the login is gone.

**Try every read without the login first (artifikctl, 2026-10).** The user logged in with an SMS code, and then the search, the procurement pages and the attachments all turned out to need no login at all. Only the user's own list needed it. Send each request once with no cookie before you build the login into it.

**The keyring has a size limit (artifikctl, 2026-10).** On macOS go-keyring refuses a value over about 3000 bytes with `data passed to Set was too big`. A login cookie that carries a Cognito access and refresh token is about 4000 bytes, and the session stores it twice. From ctlkit v0.6.0 `keystore.Keyring` keeps a long secret in parts, `token`, `token.1`, `token.2`, and a tool needs nothing of its own. To read one by hand, the head entry says `ctlkit-parts:<n>`, and the parts are joined in order.

**json-shape prints the keys of an object, and a key can be data (artifikctl, 2026-10).** An object used as a map, keyed by email or by name, came out with the real emails as its keys. Look at a new answer with `jq 'keys'` on each level first, or replace the map with one entry before `json-shape`. The same goes for a quick `jq` listing of the fields that are set: a short string field such as `ownerId` turned out to be the buyer's email. Print lengths, not values, for every string.

**An `Enum` is matched in lower case (artifikctl, 2026-10).** ctlkit lowers the input before it compares, so an enum written in upper case, as the service writes it, refuses every value. Write the enum in lower case and convert before sending.

**`Call.Save` is for one file (artifikctl, 2026-10).** It prints one document each time, so a command that saves many files cannot call it in a loop and still print one JSON document. artifikctl writes the files itself with `O_EXCL`, checks every path before it fetches the first file, and prints one list of `ctlkit.SavedFile`. A `SaveAll` in ctlkit would do the same.
