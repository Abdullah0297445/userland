# userland

One host, one compose project, and containers you switch on and off. Clone it, run one
command, answer what it asks, and the host ends up running a reverse proxy, a set of
shared datastores, and whichever applications you switched on — each behind TLS, each
with its database already provisioned and already being backed up.

There is no application code here. userland is the ground your own projects stand on,
and it is deliberately not one of them.

> **This repo is being built in the open.** Today the interview writes `.env`, and userland
> renders and runs traefik, Postgres with its doors, and Metabase. The remaining verbs and
> the other containers arrive one at a time. The design is published as issues on this repo
> as it is settled.

`userland` is the part of a running system that is not the kernel: everything the machine
runs *for you*. This repo is that layer, for one host.

## What you can switch on

| | |
|---|---|
| **The proxy** | traefik, terminating TLS for everything else. |
| **The datastores** | One Postgres, one Redis, one ClickHouse. Shared — one of each for the whole host, never one per application. |
| **The applications** | n8n, Metabase, Langfuse, Twenty, neo4j. |
| **fort** | Keeps the files you name, `.env` first, encrypted in a bucket of their own under a master key that never touches the host. |

Two things are pointed at rather than run: an S3-compatible object store you bring, which
the Postgres backup, Langfuse and fort each need a bucket of, and a secret store you own,
which holds fort's master key.

The interview offers to make each of those for you on AWS, with admin credentials it uses
once and never writes, and it adopts what already exists in your account rather than making
a second one. Decline, and it prints a checklist with your names filled in, for any
S3-compatible provider. Each bucket is reached by an access key of its own. The Postgres
dumps' and fort's may write and never delete, so a compromised host cannot erase its own
archives; Langfuse's may delete, because its Data Retention feature does. Retention is
yours: nothing in userland deletes from the dumps' bucket or fort's, so set a rule at your
provider, or accept that they grow.

Each application gets its own database on the shared Postgres, owned by a user of the same
name. The backup discovers databases by reading the server rather than by being handed a
list, so a database is backed up from the day it exists.

## The shape

- **One compose project.** The CLI renders one `compose.yml` at the root from a template
  per product and what you switched on. It holds only what is on, it holds no secret, and
  you never edit it.
- **One `.env`.** Every choice you make lands in it, and it is the only file you own. The
  CLI asks only what has no safe default; everything else has a documented name you may
  add by hand, and the CLI never touches a line it did not write.
- **You switch on containers, not bundles.** Postgres without pgadmin is a valid choice.
  Switching one off removes it and keeps its volume and its database until you ask to
  reclaim them.
- **Upstream, not a copy you edit.** You never edit a tracked file — so `git pull` keeps
  working, and it is how the next container reaches you.

## Two visibilities

**local** — plain HTTP on `*.localhost`. No DNS record, no certificate, no domain to buy.
This is how you find out whether you want it.

**public** — real hostnames, with TLS issued over a DNS-01 challenge.

They are one variable apart, and both go through traefik. Without traefik, each container
answers on this machine only, at a loopback port of its own.

## Running it

```sh
git clone https://github.com/Abdullah0297445/userland
cd userland
./bootstrap
```

`bootstrap` builds the CLI inside a `golang` container, drops the binary at the root and
hands off to it. Docker is all the host needs, and the binary always matches the commit
you have checked out. The first build pulls the image and takes a minute; later builds
reuse a cache in a docker volume named `userland-go`.

With no verb, it runs the interview, which writes `.env` and applies. `.env` is yours
afterwards: every line it holds is listed in [`VARIABLES.md`](VARIABLES.md), and the
interview never asks again for what is already there.

| Verb | What it does |
|---|---|
| `./bootstrap` | The interview: ask what is new, write `.env`, apply. |
| `./bootstrap apply` | Render, bring up, provision, print. |
| `./bootstrap on CONTAINER…` | Switch containers on, then apply. |
| `./bootstrap off CONTAINER…` | Switch containers off, then apply. Refuses while a container is blocking another, naming the dependents. Prints what it left behind. |
| `./bootstrap render` | Write `compose.yml` and stop. |
| `./bootstrap provision` | Converge the door's auth user and every switched-on database, and nothing else. |
| `./bootstrap check [--write]` | Assert the manifest and templates hold. `--write` regenerates `VARIABLES.md`. |

### What apply does

1. Validates the selection: nothing on is refused, and so is a container whose required
   dependency is off. A missing optional dependency is a warning.
2. Renders `compose.yml`. Only switched-on containers are in it, every value is a `${VAR}`
   reference, and there are no profiles.
3. If Postgres is on, brings it up alone and waits for it to be healthy, then provisions.
4. Brings up everything else with `--remove-orphans`. A container absent from the file is
   an orphan, so switching it off is enough to remove it; its volume and its database stay.
5. Prints the URL of everything that answers HTTP and the `.env` lines you must copy off
   the machine because they cannot be regenerated.

Switching traefik on or off recreates every HTTP container, because their labels and
published ports change. That is expected and loses nothing.

### The interview

1. **Visibility**, once, on the first run.
2. **One gate per product**, in dependency order: a product whose containers another
   product's require is asked after it, so applications come first and traefik last. A
   gate lists the product's containers with nothing ticked, and a container something
   else requires or wants says so beside its name. Nothing is on until you tick it;
   whatever you leave unticked is recorded as off and not asked again.
3. **The selection is checked before any variable is asked.** Nothing on is a refusal, and
   so is a container whose required dependency you left off, named. A refusal writes
   nothing, so a wrong selection costs no answers. A missing optional dependency is a
   warning, said out loud and waved past.
4. **Variables**, product by product, for the switched-on containers, skipping any already
   in `.env`. A variable with a `when` is asked only when it applies: in one visibility,
   or when another variable holds a given value. Each answer is checked against its
   type as you type it, and every one must fit on one line, without `$`, `#`, quotes or
   a backtick and without a space at either end, because compose reads `.env` unquoted.
5. **Write `.env`**, then apply.

| Type | The interview asks for |
|---|---|
| `text` | Anything that fits the line. |
| `hostname` | Labels of letters, digits and hyphens, joined by dots. |
| `email` | One address, like `name@example.com`. |
| `url` | A scheme and a host, like `https://example.com`. |
| `port` | A number from 1 to 65535. |
| `secret` | Pasted, hidden as you type. Never generated. |
| `generated` | Enter for 26 URL-safe characters, or paste your own, hidden as you type. Every database password is one. |
| `choice` | One of the manifest's options. |
| `paths` | Absolute paths on this machine, joined by `:`, each of which must exist. |

A re-run asks only about what is new: a container `.env` records as neither on nor off,
and a variable the selection needs that `.env` lacks. Answering nothing new, it applies
and stops. Switching `VISIBILITY` to `public` in `.env` makes every `when: public`
variable new, so the next run asks them. Ctrl-C anywhere writes nothing.

When a container's variable is renamed or dropped upstream, `manifest.json` says so, and
the next run of any verb moves the value to its new name or drops the line, saying so.
Those are the only two ways a line the CLI wrote is ever moved or deleted.

In a terminal that cannot draw, or from a script, set `TERM=dumb`: the interview then asks
with plain numbered prompts and reads lines. A hidden prompt still needs a terminal.

## Provisioning

Provisioning converges on `.env`. Every run, through `docker exec postgres-18 psql`:

- The user the doors look passwords up with, `pgbouncer_auth`, and its `SECURITY DEFINER`
  lookup function in the `postgres` database.
- Each switched-on container's database: its user, the database with `CONNECT` revoked
  from everyone else and `CREATE` on `public` revoked, and the `vector` extension.

A password is compared with the user's stored SCRAM verifier and changed only when they
differ, so a re-run changes nothing, a hand-edited or restored `.env` heals itself, and
rotating a password is one edit plus an apply. Passwords travel on stdin, never on a
command line. Nothing is ever dropped.

## check

`check` renders with every container on, in both visibilities, runs
`docker compose config`, and asserts:

- every container has a template and every template is a container;
- no template writes a key the generator owns, and every template's `environment` opens
  with the merge line that carries `TZ=UTC`;
- every variable a template reads without a default is asked by the manifest or is a
  database password;
- `VARIABLES.md` matches the manifest and templates;
- compose accepts the rendered file;
- `TZ=UTC` reaches every service;
- every container something requires has a healthcheck, since `depends_on` waits on it;
- every named volume is declared on the container that mounts it, and vice versa.

It runs in CI on every pull request and on every push to `main`
([`.github/workflows/check.yml`](.github/workflows/check.yml)), followed by `go test`.
The manifest itself is refused on load for a container in two products, a volume declared
by two containers, two containers on one loopback port, or an ask whose type or `when` the
interview does not know.

## Layout

| Path | What lives there |
|---|---|
| `bootstrap` | Builds the CLI inside docker and runs it. The one command; docker is all it needs. |
| `manifest.json` | What every container depends on and what it needs asked. The CLI trusts nothing else. |
| `VARIABLES.md` | Every variable `.env` may hold, by container. Generated; `check` fails when it is stale. |
| `compose/` | One template per product. The CLI renders `compose.yml` from them; nothing is ever run from inside it. |
| `scripts/` | Shell that runs inside a container — the backups' schedules, the dump, fort's run. Nothing here runs on the host. |
| `initdb/` | First-start initialisation for a datastore. Runs once, against an empty volume, and never again. Empty today: what used to live here is provisioned instead. |
| `config/` | Configuration files a container mounts, checked in because they hold nothing secret. |
| `consumer/` | Files you copy into a project of your own. userland never runs them. |
| `main.go`, `internal/` | The CLI, in Go, on the standard library plus [huh](https://github.com/charmbracelet/huh) for the interview's prompts. |

`compose.yml`, `.env` and the `userland` binary are yours and untracked. Support
directories are grouped **by kind, at the root** — `scripts/`, never `metabase/scripts/`.
One container's files are spread across several of them on purpose: the thing you switch
on and off is a container, not a folder.

## Adding a container

A container is a block in `manifest.json` and a `{{ define "<container>" }}` in its
product's `compose/<product>.yml`. `check` tells you what is missing.

### The manifest

| Field | Meaning |
|---|---|
| `requires` / `optional` | Containers this one depends on. A missing required one is a refusal; a missing optional one is a warning. `depends_on` is emitted from `requires`, with `condition: service_healthy`. |
| `http` | `container` port, `host` loopback port, and `subdomain` (defaults to the name). Drives the traefik labels and the `ports` block. |
| `postgres` | The `database` this container gets on Postgres, which is also its user's name, and the `.env` variable holding its `password`. Two containers naming one database share it. It implies `postgres-18` is on. |
| `ports` | Ports published on every interface. traefik alone. |
| `volumes` | Named volumes this container mounts. The top-level `volumes` block, "left behind" and `reclaim` all read this. |
| `asks` | `var`, `type`, `prompt`, optional `when` (`public`, `local` or `VAR=value`) and `keep`. Types: `text hostname email url port secret generated choice paths`, described under *The interview*. |
| `renamed` | `{"OLD_NAME": "NEW_NAME"}`. The next run moves the `.env` value under its new name and drops the old line. |
| `removed` | Variables this container no longer reads. The next run drops their lines. |

### The template

- The generator owns `container_name`, `restart`, `depends_on`, `networks`, `labels`,
  `ports` and `mem_limit`. The template holds every other key compose knows: `image`,
  `environment`, `command`, `volumes`, `healthcheck`, `shm_size` and the rest.
- `environment` opens with `<<: *userland-environment`. That merge line is how `TZ=UTC`
  reaches every container from one anchor the generator emits.
- The template reads the manifest rather than repeating it: `.Name`, `.Product`,
  `.Visibility`, `.Postgres` and `.HTTP` are in scope, and `{{ ref "VAR" }}` renders
  `${VAR}`. Never write a value where a reference will do.
- A container anything requires needs a healthcheck. Postgres's must probe over TCP:
  over the socket it is green while the image's temporary first-start server is up.
- A named volume is both a line under `volumes` in the manifest and a mount in the
  template.
- A variable read without a default is asked in the manifest or is a database password;
  one with a default, `${VAR:-value}`, is optional and lands in `VARIABLES.md` by itself.

### Names the CLI knows

Three names are kinds the CLI defines rather than manifest data: `traefik`, whose
presence decides labels and loopback ports; `postgres-18`, which provisioning execs into
and which every container with a database requires; and `pgbouncer_auth`, the user both
doors look passwords up with.

## Notes

- **Every concrete value stays out of git.** Your domain, your cloud account, your bucket
  name and your host address all live in `.env`, which is gitignored, so this repo can be
  published without redaction. Never write a real domain, bucket name, host address, email
  or account identifier into a tracked file.
- **That one `.env` holds every secret of every container you switched on.** Confirm
  `.gitignore` excludes it before your first commit, and keep a copy somewhere off this
  machine. Some of what it holds — encryption keys a container writes data with — cannot be
  regenerated, and losing them loses the data. The CLI names those lines when it finishes;
  copy them somewhere before anything runs, or switch on fort and it keeps `.env` for
  you, under a master key that lives in a secret store you own.
- **Postgres and the doors run at their images' defaults.** No pool size, connection
  ceiling or memory setting is written anywhere in this repo, beyond the two doors'
  client ceiling. Measure first; a number guessed in advance is worse than none.

See [CONTEXT.md](CONTEXT.md) for the language this repo uses.
