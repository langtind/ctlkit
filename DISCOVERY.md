# Discovery

How to find out how a service works before you write its client. Most services a `*ctl` tool wraps have no public API, so the facts have to be observed.

This file is a guide, and a log. When a tool teaches you something that would have saved time on the one before, add it under "Lessons".

## Where to look, cheapest first

Stop at the first source that answers the question. Each step down costs more setup and puts more of other people's data in front of you.

| Source | Good for | Cost |
| --- | --- | --- |
| 1. Official docs, an OpenAPI file, a developer portal | Everything, when it exists | None |
| 2. An open client somebody else wrote | Paths, parameters, field names | None. Take facts only, and check its licence before you copy a line |
| 3. The service's own web client | What the service's own code does: login, renewal, headers, retries | None. No proxy and no certificate |
| 4. Live answers from the tool itself | Shapes, lifetimes, ordering, limits | One login |
| 5. A proxy in front of the mobile app | A service that has no web client | A certificate on the phone, and it fails when the app pins its certificate |

An open client is a lead, not a fact. It shows what worked for its author when it was written. It is often wrong about what is missing: see the first lesson below.

### 3. Read the web client

The web client is the service's own code, and it is public. Download the page, find the script bundle, and search it. This is the fastest way to learn how login and renewal work, because the code says it outright.

```bash
curl -sL https://example.com/app -o index.html
grep -oE 'src="[^"]+\.js[^"]*"' index.html       # find the bundle
curl -sL https://example.com/app/main-abc123.js -o main.js
```

A bundle is tens of megabytes on few lines, and `grep` with a long pattern gives up on it. Search it with a few lines of Python:

```python
import re
s = open("main.js", encoding="utf-8", errors="replace").read()
print(sorted(set(m.group(0) for m in re.finditer(r"auth2?/[A-Za-z0-9/_-]*", s))))
i = s.find("refreshToken")
print(s[max(0, i - 300):i + 900])
```

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

Look at keys and types. Do not print a whole answer into the conversation or into a file in the repo: it holds real people.

### 5. A proxy

Only when the service has no web client, or the app does something the web client does not.

- **mitmproxy** (`brew install mitmproxy`) is the one that suits an agent: `mitmdump` runs without a window, writes flows to a file, and takes a Python addon that can strip values so only shapes are written. `mitmproxy2swagger` turns a flow file or a HAR into a draft OpenAPI file. Not used in any tool here yet.
- **Proxyman** has an MCP server with its own redaction, and makes the certificate setup on an iPhone easy. The app has to be open, bodies come as previews, and it exports cURL but not HAR. Good when a person is driving.

A capture holds a login token and other people's data. Extract the request and the field names, write them in `reference/`, and delete the capture. Never commit one. `.gitignore` in the template already refuses `*.har` and `*.proxymansession`.

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
- What running it twice does.
- Run it for real only with the consent of whoever it concerns.

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

An agent cannot answer a login prompt, so a tool has to keep itself logged in for as long as the service allows.

- Store the whole login with `session.Manager`: the access token, the refresh token, and when each runs out.
- `Manager.Token` renews when the access token is about to run out. `Manager.Refresh` renews after the service refused a token, and the client then sends the request once more.
- Renewal runs under a lock file. An agent runs several commands at the same time, and a service that rotates refresh tokens would log out every run but one.
- Only a clear no from the service ends the login. A timeout or a 503 during renewal says nothing about it, and the stored login is left alone.
- Give the tool `status`, which says how long the login lasts without calling the service, and `refresh`, which renews now. The template has both.
- When the refresh token itself runs out, the user logs in again. Say so in the error, and say in `status` when that will be.

## Lessons

**An open client can be wrong about what is missing (spondctl, 2026-10).** The Python client for Spond says in its code that a token cannot be renewed. The login answer holds three tokens: an access token for 24 hours, a refresh token for 90 days, and a password token for 10 minutes. The renewal request was in Spond's own web client, found in ten minutes: `POST /auth2/login/refresh` with `{"value": <refresh token>}`. Read the login answer yourself.

**A list has an order, and the limit cuts from the wrong end (spondctl, 2026-10).** Spond lists events latest first. The tool asked for 21 events from today, sorted them, and showed the "next" 20, which were the 20 furthest away. `order=asc` fixed it. A test with a fake service could not have caught this, because the fake answered in whatever order the test wrote. Check the order of every list against the real service, with a limit lower than the number of items.

**go-keyring wraps long values (2026-10).** On macOS, a value that is long comes back from `security find-generic-password -w` as `go-keyring-base64:` followed by base64. Strip the prefix and decode before you use it by hand.

**A file address is not always behind the login (spondctl, 2026-10).** Attachments on Spond are served without a token from another host than the API. Fetch a file only from the origin you expect, and never send the token along with an address that came out of an answer.

**`go install` of a module that needs a private module fails for everyone else.** A public tool needs a public ctlkit. CI fails the same way, with `could not read Username for 'https://github.com'`.

**A local Homebrew tap is a copy.** After a release, `brew install` does not find a new cask until `brew update` has run.
