# Can the Infisical Agent write the host's `.env` files?

Research for [#38](https://github.com/Abdullah0297445/userland/issues/38), "Every .env lives in
Infisical, and a new host gets them back", a child of the map
[#32](https://github.com/Abdullah0297445/userland/issues/32). It answers one of #38's questions:
how a `.env` is written from Infisical, and when. It builds on "How Infisical is self-hosted"
(`docs/research/infisical.md` on the branch `research/infisical`), and does not repeat it. It was
checked on 2026-09-25 against Infisical `v0.165.16` and the Infisical CLI `0.43.136`.

## The short answers

- **Yes, it can.** `infisical agent` is in the `infisical/cli:0.43.136` image. It logs in once,
  with universal auth, and then keeps one file per template up to date. One agent can write
  userland's `.env` and every consumer's `.env`, each from its own project, each to its own path.
- **It writes only when something changed.** Each template asks Infisical on every poll: every 5
  minutes by default, and not faster than once a minute. It writes only when the server's
  fingerprint of the secrets changes. The first poll after a start always writes.
- **It keeps the old file when things fail.** Infisical down, login refused, access lost: the
  agent writes nothing and tries again. It builds the whole file in memory first.
- **But an empty project gives an empty `.env`.** The agent writes a 0-byte file. Compose then
  gives every variable a blank value, with only a warning. A two-line guard in the template
  stops this: it makes the template fail, so the old file stays.
- **The template sets the format, and it can be exact.** A template that writes `KEY="value"`,
  and escapes `\`, `"`, `$` and newlines, gave compose every value unchanged: `'`, `$`, `#` and
  newlines too. That fixes the single-quote break of `export --format dotenv`. `${OTHER}` is
  expanded by default; one option turns that off.
- **It writes in place.** It empties the file and writes it again. It makes no temporary file and
  renames nothing. So a bind mount of a single file works. But that file must exist before the
  container starts, or docker makes a folder in its place.
- **Owner and mode.** The image runs as root. A new file is `root:root`, mode `0644`: any user on
  the host can read it. A file that already exists keeps its owner and mode. Run the agent with
  `user:`, or make each `.env` first with mode `0600`.
- **It cannot restart anything.** `execute` runs a shell command inside the agent's own
  container, as root, after a change. The image has no docker. And compose applies a new `.env`
  only when `docker compose up -d` makes the container again; `restart` keeps the old values. So
  the agent keeps the file fresh, but the containers stay as they were until someone runs `up`.
- **Login: files or environment variables.** The client ID and secret come from files, or from
  `INFISICAL_UNIVERSAL_AUTH_CLIENT_ID` and `INFISICAL_UNIVERSAL_CLIENT_SECRET`. The second is not
  the name `infisical login` uses. `remove_client_secret_on_read` deletes the secret file, the
  host's copy too. `infisical.address` must be set: without it the agent logs in to Infisical's
  cloud, whatever `--domain` says.
- **A wrong client secret locks the login for five minutes.** Three failed logins within 30
  seconds lock that client ID, for every caller. The CLI retries a refused login by itself, so
  one start with a wrong secret is enough.
- **Rate limits do not apply to a self-hosted Infisical.** The server turns its rate limiter on
  only on Infisical's cloud. Ten templates polling every 5 seconds made 120 secrets calls a
  minute, and every one was answered. This corrects "Rate limits in the free edition" in the
  earlier note.
- **Nothing about the agent is paid.** The CLI is MIT. What the agent uses on the server is free.
- **A one-shot helper can have the same template.** `infisical export --template FILE` runs the
  agent's template engine once. It exits 1 when the guard fails. So a helper gets the exact
  format and the empty-project guard, without a service that runs all the time. The agent adds
  only this: files that stay fresh with no one running anything.

## Words used here

- **Agent**: `infisical agent`, a program that keeps running. It logs in, then writes files from
  templates, again and again.
- **Template**: a text file in Go's template language. It says which secrets to fetch and how to
  print them. The agent turns it into the file's content. This is called *rendering*.
- **Poll**: one round in which the agent asks Infisical for a template's secrets.
- **ETag**: a short fingerprint that the server sends with a list of secrets. It changes when the
  secrets change.
- **Sink**: a file where the agent saves its access token. userland does not need one.
- **Bind mount**: a folder or file of the host that appears inside a container. A *single-file*
  bind mount is one file, not a folder.
- **In place**: writing into the same file, so it keeps its identity on disk (its *inode*). The
  other way is to write a new file and *rename* it over the old one.
- **Atomic**: a change that a reader sees whole or not at all, never half done.
- **umask**: the setting that removes permission bits from each new file. Here it is `022`, so a
  new file is `0644`: the owner writes, everyone reads.
- **Lockout**: Infisical refusing a login for a while after too many failed ones.

Words from the earlier note keep their meaning: *machine identity*, *universal auth*,
*environment*, *bootstrap*.

## How this was checked

1. The CLI's source at the tag `v0.43.136`
   ([Infisical/cli](https://github.com/Infisical/cli/tree/v0.43.136)). The agent is one file,
   [packages/cmd/agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go).
2. Infisical's source at the tag `v0.165.16`
   ([Infisical/infisical](https://github.com/Infisical/infisical/tree/v0.165.16)), for what the
   server does with the agent's calls.
3. Infisical's docs, above all the
   [Infisical Agent page](https://infisical.com/docs/integrations/platforms/infisical-agent),
   and Docker's [`.env` file syntax](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/#env-file-syntax).
4. **A drill**: a throwaway compose project, with its own project name, torn down afterwards
   with its volumes. It ran `infisical/infisical:v0.165.16`, `postgres:18-alpine`,
   `redis:7-alpine` and one agent from `infisical/cli:0.43.136`. Bootstrap went as the earlier
   note says. It made three projects, `userland`, `consumer-a` and `consumer-b`, each with one
   environment, `host`. One machine identity with universal auth was a `member` of all three.
   One agent ran ten templates, polling every 5 seconds. It wrote into a bind-mounted folder, a
   single-file bind mount and a named volume. Compose read the files with Docker Compose v5.5.1.
   Findings marked **(drill)** come from it.

Owners and modes were read inside the named volume, where they are real Linux ones. Where a doc
and the source disagree, this note trusts the source; the list is at the end.

## 1. The agent, its config, and its login

### It is in the CLI image

- `infisical agent` is a command of the CLI. The `infisical/cli` image's entrypoint is the CLI,
  so the compose service's command is `["agent", "--config", "/agent/agent.yaml"]`
  **(drill)**. ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L3508-L3520),
  [docker/alpine](https://github.com/Infisical/cli/blob/v0.43.136/docker/alpine))
- The config is a YAML file, `agent-config.yaml` by default, set with `--config`. It can also be
  given whole, base64-encoded, in `INFISICAL_AGENT_CONFIG_BASE64`.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L3526-L3545),
  [init](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L3975-L3987))

### The config file

This is the shape the drill ran, with paths as userland might use them. The drill used more
templates and shorter polls.

```yaml
infisical:
  address: "http://infisical:8080"
auth:
  type: "universal-auth"
  config:
    client-id: "/agent/client-id"
    client-secret: "/agent/client-secret"
templates:
  - source-path: /agent/templates/userland.tmpl
    destination-path: /userland/.env
    config:
      polling-interval: 5m
  - source-path: /agent/templates/consumer-a.tmpl
    destination-path: /consumers/consumer-a/.env
    config:
      polling-interval: 5m
```

- The fields are `infisical` (`address`, `exit-after-auth`, `revoke-credentials-on-shutdown`,
  `retry-strategy`), `auth`, `sinks`, `cache` and `templates`. Each template has one source
  (`source-path`, `template-content` or `base64-template-content`), a `destination-path`, and a
  `config` with `polling-interval` and `execute`.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L102-L206),
  [docs](https://infisical.com/docs/integrations/platforms/infisical-agent#agent-configuration-file))
- `sinks` are optional. The agent needs no token file to write templates.
- `cache` is for dynamic secrets, and works only in Kubernetes. userland needs neither.

### Pointing it at a self-hosted Infisical

- **Set `infisical.address`.** The agent reads only that. If it is empty, it becomes
  `https://app.infisical.com`, Infisical's cloud.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L51),
  [parse](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L864-L891))
- **`--domain` and `INFISICAL_DOMAIN` do not help.** The agent hides `--domain` and then
  overwrites it with the config's address. In the drill, with no address in the config and both
  set to `http://infisical:8080`, the agent logged "Infisical instance address set to
  https://app.infisical.com" and tried to log in there. (It ran with no network, so nothing left
  the machine.) **(drill)** A config that forgets the address sends the client secret to the
  cloud.
- `/api` may be left off the address; the CLI adds it.

### Logging in with universal auth

- **Files or environment variables, and the variable comes first.** For each of the two values,
  the agent first reads an environment variable. If it is empty, it reads the file named in the
  config, and trims spaces and newlines.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1250-L1283),
  [helper.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/helper.go#L502-L516))
- **The names are not the same as `infisical login`'s.** The client ID is
  `INFISICAL_UNIVERSAL_AUTH_CLIENT_ID`. The client secret is `INFISICAL_UNIVERSAL_CLIENT_SECRET`,
  with no `AUTH`. With `INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET`, the name `login` uses, the agent
  found no secret and failed: "unable to read file content from file path ''". With
  `INFISICAL_UNIVERSAL_CLIENT_SECRET` it logged in, with no files at all **(drill)**.
  ([constants.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/constants.go#L41-L42))
- **`remove_client_secret_on_read: true` deletes the secret file** after reading it. Through a
  bind mount, that is the host's file: in the drill it was gone after one run **(drill)**. The
  agent keeps the secret in memory for later logins. But after a restart it has none, and cannot
  log in. For a service that restarts with the host, leave this `false`.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1262-L1272))
- **One login serves every template.** The agent holds one access token. It renews it at two
  thirds of its life, and logs in again when the token reaches its maximum age. With universal
  auth's defaults (a 30-day token) that is one renewal every 20 days.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1718-L1810))

## 2. Templates, and a `.env` that compose reads

### Naming a project, an environment and a path

A template fetches secrets with one of these
([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1035-L1053),
[docs](https://infisical.com/docs/integrations/platforms/infisical-agent#available-secret-template-functions)):

| Function | Arguments |
|---|---|
| `listSecrets` | project ID, environment slug, path, and an optional JSON string of options |
| `listSecretsByProjectSlug` | project slug, environment slug, path, options |
| `getSecretByName` | project ID, environment slug, path, secret name |

Each secret has `.Key` and `.Value`, and also `.SecretPath`, `.Comment` and a few more. The
[Sprig](https://masterminds.github.io/sprig/) functions are there too, such as `replace` and
`fail`, but not `env` or `expandenv`.
([templates.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/templates/templates.go#L13-L31))

Keys came out sorted by name **(drill)**.

### Secret references: on by default, and can be turned off

- **`${OTHER}` is expanded by default.** The options are `recursive` (default `false`) and
  `expandSecretReferences` (default `true`). Pass `` `{"expandSecretReferences": false}` `` to
  keep values as written.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L893-L929))
- In the drill, `REF=${DB_PASSWORD}-suffix` came out as `abc123def456-suffix` by default, and as
  `${DB_PASSWORD}-suffix` with the option **(drill)**.
- Imported secrets are always included.

### What each format does to hard values

Compose's own rules: unquoted and double-quoted values are interpolated (a `$` starts a
variable); single-quoted ones are taken literally and may span lines; double quotes support
`\n`, `\\` and `\"`; an unquoted value ends at a ` #`.
([Docker docs](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/#env-file-syntax))

Three templates wrote the same secrets: unquoted, single-quoted and double-quoted. Compose then
read each file as the project `.env`, and a container printed what it got **(drill)**:

| Value in Infisical | `KEY=value` | `KEY="value"`, escaped |
|---|---|---|
| `hello` | `hello` | `hello` |
| `it's` | `it's` | `it's` |
| `pa$$word$HOME` | `pa$word` and then the host's `$HOME` | `pa$$word$HOME` |
| `before #after` | `before` | `before #after` |
| `line1` newline `line2` | `line1` | `line1` newline `line2` |
| `${DB_PASSWORD}-suffix` | `abc123def456-suffix`: compose expands it itself | `${DB_PASSWORD}-suffix` |
| `say "hi"` | `say "hi"` | `say "hi"` |
| `a\b` | `a\b` | `a\b` |

- **Unquoted `KEY=value` is safe only for plain values**, such as hex passwords. Compose expands
  `$`, cuts at ` #`, and loses the rest of a value after a newline. It gives no error.
- **Single-quoted `KEY='value'` is what `export --format dotenv` writes.** Compose refused the
  whole file: `unexpected character "'" in variable name`, because one value held `'`. So it
  gives no column above. The earlier note found `$`, `#`, `=` and spaces fine in single quotes,
  as long as no value holds `'`.
- **Double-quoted, with four escapes, was exact.** Every value reached the container unchanged.
  The same file also worked as a service's `env_file:` **(drill)**. This is the template:

```text
{{- $s := listSecrets "PROJECT_ID" "host" "/" `{"expandSecretReferences": false}` -}}
{{- if not $s }}{{ fail "the project holds no secrets" }}{{ end -}}
{{ range $s }}{{ .Key }}="{{ .Value | replace "\\" "\\\\" | replace "\"" "\\\"" | replace "$" "$$" | replace "\n" "\\n" }}"
{{ end }}
```

The backslash must be escaped first. `$$` is compose's way to write one `$`. The first two lines
are the guard of section 6.

One more drill result: compose also read `A='Let\'s go!'` as `Let's go!`, and a single-quoted value
across two lines as two lines **(drill)**. So escaping `'` as `\'` may work too. A backslash inside
single quotes was not tried, so this note keeps to double quotes.

## 3. Many `.env` files in one agent

- **Yes, with one login.** Each template is its own loop, with its own poll timer and its own
  destination. They share the one access token
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L3646-L3675)).
  In the drill, ten templates across three projects ran from one agent, after one login
  **(drill)**.
- **The identity must be a member of every project it reads.** When the drill removed the
  identity from `consumer-b`, that template failed and its file stayed as it was **(drill)**.
- **The agent needs every destination mounted.** Each consumer lives in its own folder elsewhere
  on the host, so the agent's container needs a bind mount for each one, or for a folder above
  them all. A new consumer means a new mount, a new template and a restart of the agent.
- **Keep one fetch per template.** The agent compares only the fingerprint of the last fetch in
  a template. A template that fetched two projects missed a change in the first one: it did not
  rewrite in 12 seconds of 2-second polls, while a one-project template saw the change **(drill)**.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L905-L929))

### API calls per poll

- Each poll of each template makes one `GET /api/v4/secrets` for each `listSecrets` in it.
  `listSecretsByProjectSlug` adds one `GET /api/v1/projects/slug/SLUG` first.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L905-L946),
  [api.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/api/api.go#L626-L668))
- **The agent sends no `If-None-Match`**, so the server sends the full list every time. The
  server could answer "not modified", but the agent never asks. It compares the ETag itself.
  ([secret-router.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/v4/secret-router.ts#L225-L233))
- In the drill, ten templates at 5 seconds made 60 secrets calls and 6 project lookups every 30
  seconds, exactly one per template per poll **(drill)**.
- A failed call is retried up to 3 more times, on a network error or a 429, 502, 503 or 504.
  ([retry.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/retry.go#L19-L55))
- So `N` templates at the default 5 minutes cost about `N / 5` secrets calls a minute. At the
  floor of 1 minute, `N` a minute.

### Rate limits: none on a self-hosted Infisical

- The earlier note read the free edition's limits: 40 secrets calls a minute per address. Those
  numbers are real, but **the server only turns its rate limiter on when it runs as Infisical's
  cloud**. The check is `isProductionMode && isCloud`, and `isCloud` means a cloud licence key
  is set. A self-hosted server skips it, and the per-route limits then do nothing.
  ([app.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/app.ts#L145-L148),
  [env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L718-L729))
- In the drill the agent made about 120 secrets calls a minute from one address. All 144
  requests the server logged in that minute answered 200; none answered 429 **(drill)**.

## 4. How it writes the file

- **In place, not by rename.** The agent calls Go's `os.Create` on the destination and writes
  the bytes. That empties the file and writes it again. It makes no temporary file.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L798-L807),
  [WriteTemplateToFile](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1848-L1858))
  The file kept its inode across rewrites **(drill)**.
- **So the write is not atomic.** For a moment the file is empty or half written. Compose reads
  `.env` only when it runs, so the risk is a `docker compose` command at that same moment. The
  window was not measured.
- **The template is rendered whole before the file is opened.** A failed fetch never leaves half
  a file. See section 6.
- **A single-file bind mount works**, because nothing is renamed. The drill wrote `/single/.env`,
  a single-file mount, and the host's file changed **(drill)**.
- **A rename would fail there.** In the same container, `mv` onto the single-file mount failed with
  "Resource busy". A rename inside a mounted folder worked **(drill)**. This matters for any tool
  that writes a temporary file and renames it, not for the agent.
- **The file must exist before the container starts.** When a bind mount's source is missing,
  docker makes a folder with that name on the host **(drill)**. The agent then cannot write, and
  compose cannot read a `.env` that is a folder. Mounting the folder avoids this.
- **The destination's folder must exist.** The agent does not make folders. When it was missing,
  the agent logged "no such file or directory. Will try again on next cycle". It did not: after
  the folder was made, the file came only with the next change of the secrets, or a restart
  **(drill)**.

### Owner and permissions

- **The image runs as root.** Its Dockerfile has no `USER`, and the image's user is empty. `id`
  in the container said `uid=0(root)`, and `umask` said `0022` **(drill)**.
  ([docker/alpine](https://github.com/Infisical/cli/blob/v0.43.136/docker/alpine))
- **A new file is `root:root`, mode `0644`** **(drill)**. Any user on the host can read it, and
  only root can change it.
- **A file that exists keeps its owner and mode.** `os.Create` empties it but does not change
  them. A file made first as `1000:1000`, mode `0600`, stayed so after the agent wrote it **(drill)**.
- **The agent also runs as another user.** With `--user 1000:1000`, it wrote its file as
  `1000:1000` into a folder of mode `0700` owned by that user **(drill)**. The mode was still
  `0644`, so the folder's mode is what keeps others out.

## 5. When it writes

- **Every poll fetches; a write happens only on a change.** After each fetch the agent compares
  the new ETag with the last one. It writes when they differ, or on its first render.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1932-L1958))
- **The ETag comes from the server.** It is a hash of the secrets as served, sent as the `ETag`
  header. So it changes when any secret in the list changes.
  ([secret-v2-bridge-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/secret-v2-bridge/secret-v2-bridge-service.ts#L1364-L1392),
  [api.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/api/api.go#L665))
- In the drill a file's modified time did not move over more than a minute of 5-second polls.
  After one secret changed, the changed project's files were rewritten within one poll. A file
  from an unchanged project was not touched **(drill)**.
- **After a restart, every file is written again**, even when nothing changed **(drill)**.
- **The interval.** `polling-interval` in each template's `config`. The default is 5 minutes.
  The units are `s`, `ms`, `m`, `h`, `d` and `w`. Under 60 seconds is refused: `30s` gave "polling
  interval must be at least 60 seconds", and the agent stopped, with exit code 0 **(drill)**.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1860-L1877),
  [helper.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/helper.go#L717-L769))
- The check in milliseconds only refuses under `1000ms`. So `5000ms` gives 5 seconds; the drill
  used it **(drill)**. It looks like a slip in the code, and should not be relied on.

## 6. When things go wrong

| What happened | What the agent did | The file |
|---|---|---|
| Infisical stopped while the agent ran | Every fetch failed ("no such host"). It logged an error and tried again every 5 seconds (the shorter of the interval and 30 seconds). | Kept **(drill)** |
| The agent restarted while Infisical was down | Login failed. It tried again every 30 seconds. | Kept **(drill)** |
| Infisical came back | Login worked at the next try. Every file was written again. | Rewritten **(drill)** |
| Wrong client secret | Login refused (401), then locked out. It tried again every 30 seconds, forever. | Kept **(drill)** |
| Identity removed from a project | That template's fetch failed. Others went on. | Kept **(drill)** |
| Destination folder missing | Logged an error. Wrote again only on the next change. | Not written **(drill)** |
| **Project emptied, no guard** | Fetch worked, with no secrets. | **Emptied to 0 bytes** **(drill)** |
| Project emptied, with the guard | `fail` made the template fail. It logged the error each poll. | Kept **(drill)** |

([agent.go, the template loop](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1860-L1979),
[the login loop](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1718-L1810))

- **The rule is simple.** Any error while rendering means no write. A render that works is
  written, even when it is empty.
- **An empty `.env` does not stop compose.** Read as the project `.env`, it made compose warn
  "The "DB_PASSWORD" variable is not set. Defaulting to a blank string." for each variable, and
  go on **(drill)**. A container made then would get blank passwords.
- **The guard** is two lines at the top of a template, shown in section 2:
  `{{- if not $s }}{{ fail "…" }}{{ end -}}`.
- **The lockout.** Universal auth locks a client ID after 3 failed logins within 30 seconds, for
  5 minutes. These are the defaults, and each identity can change them. The lock is on the client
  ID, not on the caller's address. The CLI retries a refused login by itself, so one start with a
  wrong secret locked it. In the drill, a second container with the right secret was then refused
  with "This identity auth method is temporarily locked" until the five minutes were over. An
  agent that is already logged in keeps working, because it holds a token **(drill)**.
  ([identity-ua-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/identity-ua/identity-ua-service.ts#L141-L222),
  [identity-universal-auth-router.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/v1/identity-universal-auth-router.ts#L208-L221),
  [Universal auth docs](https://infisical.com/docs/documentation/platform/identities/universal-auth))
- **Exit codes are not a health signal.** A missing config file and a bad interval both logged
  an error and exited 0 **(drill)**. A config that does not parse does the same, by the source.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L3528-L3560))

### `exit-after-auth`: the agent run once

- With `infisical.exit-after-auth: true`, the agent renders each template once and exits 0. If a
  template fails, such as the guard, it exits 1 and writes nothing **(drill)**.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1913-L1917),
  [exit](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L3654-L3668))
- **But a failed login never exits.** It loops every 30 seconds, `exit-after-auth` or not. A
  one-shot run with a wrong secret was still running after half a minute **(drill)**. A helper that
  uses this mode needs its own time limit.

## 7. Running a command on change

- `templates[].config.execute.command` runs after the file is written, **only when the content
  changed**, never on the first render. `timeout` is in seconds.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1943-L1950))
- It runs with `$SHELL -c`, or `sh -c` when `SHELL` is unset, as it is in the image. It runs
  **inside the agent's container**, as root, with the container's files and network.
  ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L753-L787))
- A failing command is logged ("unable to execute command because exit status 1") and the agent
  goes on **(drill)**.
- **The image has no docker.** It is Alpine with busybox: `sh`, `wget` and `nc`, but no `docker`,
  no `curl`, no `bash`, and no docker socket **(drill)**. So it cannot restart containers.
- **Mounting the docker socket and adding the docker CLI** would need an image of userland's own,
  and gives the agent root over the host. That was not tried.
- **Even a restart would not be enough.** Compose reads `.env` when it makes a container. In the
  drill, after the `.env` changed, `docker compose restart` kept the old value, and
  `docker compose up -d` made the container again with the new one **(drill)**. So a new value
  needs `up`, which needs compose and the project's files: the host, not the agent.

## 8. What is paid

- **Nothing the agent needs.** The CLI repository has no `ee/` folder, so all of it, the agent
  included, is under MIT. ([LICENSE](https://github.com/Infisical/cli/blob/v0.43.136/LICENSE))
- On the server, the agent uses universal auth, projects, environments and reading secrets. The
  earlier note found all of these free.
- **Out of reach, and not needed:** the `dynamicSecret` template function needs dynamic secrets,
  a paid feature. The persistent cache works only in Kubernetes.
  ([license-fns.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/ee/services/license/license-fns.ts#L53-L64),
  [docs, Caching](https://infisical.com/docs/integrations/platforms/infisical-agent#caching))

## 9. The agent, or a one-shot helper

**`export` can use a template too.** `infisical export --template FILE` renders the same kind of
template, with the same functions, and prints it. On a template error it exits 1. In the drill it
printed exactly the bytes the agent wrote for the double-quoted template. With the guard on an
empty project it printed nothing and exited 1. Plain `export --format dotenv` on the same empty
project exited 0 with empty output **(drill)**.
([export.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/export.go#L112-L133),
[flag](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/export.go#L276))

So the choice is not about format. Both get the exact format and the guard. It is about when the
file is written.

| | The agent, as a service | A helper: `login`, then `export --template`, once per `.env` |
|---|---|---|
| Runs | All the time, as one more container | Only when someone runs it, such as just before `docker compose up -d` |
| Stays fresh | Within one poll of a change | Only when run |
| Applies the change to containers | No; someone still runs `up` | The helper can run `up` straight after |
| Format and empty-project guard | Same template | Same template |
| If Infisical is down | Keeps the old file, tries again | Exits non-zero; the helper must keep the old file |
| Writing the file | In place, not atomic, root-owned `0644` unless set | The helper chooses: write a temporary file with `0600`, rename it on exit 0 |
| Mounts | Every `.env`'s folder, into one container | Only the one `.env` it writes, or none: it can print to stdout |
| A new consumer | New mount, new template, agent restart | One more run of the helper |
| Login | Once, then renewals every 20 days | One login per run |
| Failure signals | Log lines only; exit 0 on bad config | Exit codes, which a script can check |
| Costs on the server | 1 call per template per poll, forever | 1 login and 1 call per `.env` per run |
| Fits "nothing runs on its own" | No | Yes, like the helpers in `bin/` |

Two limits hold for both. Neither can write the `.env` that Infisical itself needs in order to
start, because both read from Infisical. And both need the client ID and secret on the host.

## Where the docs and the source disagree

The source wins in each case.

1. **Where the login comes from.** The docs say `client-id` and `client-secret` are file paths.
   The source reads an environment variable first, and the secret's name,
   `INFISICAL_UNIVERSAL_CLIENT_SECRET`, is not the one `infisical login` reads.
   ([docs](https://infisical.com/docs/integrations/platforms/infisical-agent#agent-configuration-file),
   [agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1257-L1262))
2. **Login retries.** The docs say the agent keeps trying "increasing the time between each
   attempt". The source waits a fixed 30 seconds between rounds, after up to 3 quick retries.
   ([agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go#L1750-L1790))
3. **`exit-after-auth`.** The docs say it exits "after authentication and first secret render".
   It exits after the first render, but never while login fails.
4. **The polling floor.** The docs give only the default. The source refuses under 60 seconds,
   except in milliseconds.
5. **"Will try again on next cycle."** The agent logs this when a write fails. It tries again
   only when the secrets change.

## Not confirmed

- **Linux-host bind mounts.** Owners were read in a named volume. A bind mount on a Linux host
  should behave the same, since docker passes the owner through, but the drill did not run on one.
- **A read during a write.** How long the file is empty while the agent writes, and whether
  compose ever caught it, was not measured.
- **Carriage returns and tabs in values.** The double-quoted template was tested with `\`, `"`,
  `$`, `'`, `#` and newlines, not with `\r` or tabs.
- **Backslashes inside single quotes.** Compose read `\'` as `'`, but `\\` inside single quotes
  was not tried.
- **Token renewal.** The 20-day renewal and the re-login at the token's maximum age are from the
  source. The drill ran for minutes, not weeks.
- **The docker socket.** Restarting containers from `execute`, with a custom image and the socket
  mounted, was not tried.
- **`revoke-credentials-on-shutdown`** and **sinks** were not tried; userland needs neither.
- **The `5000ms` loophole** worked, but may be closed in a later release.

## Sources

- Infisical CLI source, tag `v0.43.136`: <https://github.com/Infisical/cli/tree/v0.43.136>
  - [packages/cmd/agent.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/agent.go)
  - [packages/cmd/export.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/export.go)
  - [packages/util/helper.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/helper.go)
  - [packages/util/retry.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/retry.go)
  - [packages/api/api.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/api/api.go)
  - [packages/templates/templates.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/templates/templates.go)
  - [docker/alpine](https://github.com/Infisical/cli/blob/v0.43.136/docker/alpine)
  - [LICENSE](https://github.com/Infisical/cli/blob/v0.43.136/LICENSE)
- Infisical source, tag `v0.165.16`: <https://github.com/Infisical/infisical/tree/v0.165.16>
  - [backend/src/server/app.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/app.ts)
  - [backend/src/lib/config/env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts)
  - [backend/src/server/routes/v4/secret-router.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/v4/secret-router.ts)
  - [backend/src/services/secret-v2-bridge/secret-v2-bridge-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/secret-v2-bridge/secret-v2-bridge-service.ts)
  - [backend/src/services/identity-ua/identity-ua-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/identity-ua/identity-ua-service.ts)
  - [backend/src/ee/services/license/license-fns.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/ee/services/license/license-fns.ts)
- Docs:
  - [Infisical Agent](https://infisical.com/docs/integrations/platforms/infisical-agent)
  - [Universal Auth](https://infisical.com/docs/documentation/platform/identities/universal-auth)
  - [Docker Compose: `.env` file syntax](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/#env-file-syntax)
- The images: <https://hub.docker.com/r/infisical/cli/tags>,
  <https://hub.docker.com/r/infisical/infisical/tags>
- The earlier note: `docs/research/infisical.md` on the branch `research/infisical`.
- The drill, described under [How this was checked](#how-this-was-checked).
