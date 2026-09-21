# userland

One host, one compose project, and containers you switch on and off. Clone it, run one
command, answer what it asks, and the host ends up running a reverse proxy, a set of
shared datastores, and whichever applications you switched on — each behind TLS, each
with its database already provisioned and already being backed up.

There is no application code here. userland is the ground your own projects stand on,
and it is deliberately not one of them.

> **This repo is being built in the open.** Today the interview writes `.env`, every verb
> exists, and userland renders and runs traefik, Postgres with its doors, ClickHouse, Metabase
> and n8n. The other containers arrive one at a time. The design is published as issues on this repo as
> it is settled.

`userland` is the part of a running system that is not the kernel: everything the machine
runs *for you*. This repo is that layer, for one host.

## What you can switch on

| | |
|---|---|
| **The proxy** | traefik, terminating TLS for everything else. |
| **The datastores** | One Postgres and one ClickHouse, shared: one of each for the whole host, never one per application. Redis is the exception: a product that needs it runs its own, inside the product, and nothing else is pointed at it. |
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
| `./bootstrap on NAME…` | Switch containers on, then apply. A product's name opens its gate instead, with what is on already ticked. Asks whatever variable the new selection needs before writing anything. |
| `./bootstrap off CONTAINER…` | Switch containers off, then apply. Refuses while a container is blocking another, naming the dependents. Prints what it left behind and offers to reclaim it; answering nothing keeps it. |
| `./bootstrap set VAR` | Ask one variable again, then apply. `set VISIBILITY` switches visibility and then asks whatever the new one needs. Only asked variables: an optional one is a line you add by hand. |
| `./bootstrap reclaim CONTAINER…` | Drop the volume and the database a switched-off container left, after naming them and asking. |
| `./bootstrap contract` | Print the Contract: what a consumer needs to use userland. |
| `./bootstrap postgres database add NAME` | Make a consumer's database and user, and print the DSN once. `--session` names the session door; `--api` adds the PostgREST recipe. |
| `./bootstrap postgres database remove NAME` | Drop a consumer's database and its users, after asking. |
| `./bootstrap apply` | Render, bring up, provision, print. |
| `./bootstrap render` | Write `compose.yml` and stop. |
| `./bootstrap provision` | Converge the door's auth user and every switched-on database, on Postgres and ClickHouse, and nothing else. |
| `./bootstrap new PRODUCT CONTAINER…` | For contributors: append a product to `manifest.json` and write its template, with placeholders. |
| `./bootstrap check [--write]` | Assert the manifest and templates hold. `--write` regenerates `VARIABLES.md`. |

`./bootstrap --help` lists the same verbs, grouped the same way; a product's verbs sit under
the product's name, so `postgres` has `database`, and a later backup verb would join it there.
Every verb that changes `.env` ends with an apply, and every verb that drops something asks
first and defaults to no.

### What apply does

1. Validates the selection: nothing on is refused, and so is a container whose required
   dependency is off. A missing optional dependency is a warning.
2. Renders `compose.yml`. Only switched-on containers are in it, every value is a `${VAR}`
   reference, and there are no profiles.
3. If Postgres or ClickHouse is on, brings them up first and waits for them to be healthy,
   then provisions.
4. Brings up everything else with `--remove-orphans`. A container absent from the file is
   an orphan, so switching it off is enough to remove it; its volume and its database stay.
5. Prints the URL of everything that answers HTTP and the `.env` lines you must copy off
   the machine because they cannot be regenerated.

Switching traefik on or off recreates every HTTP container, because their labels and
published ports change. That is expected and loses nothing.

### Switching off, and reclaiming

`off` removes the container and keeps its named volumes and its database, then names them
and offers to reclaim them. Decline, and `reclaim CONTAINER` drops them later, after naming
them again and asking. A database shared with a container that is still on is not offered.
Reclaiming `postgres-18` drops its volume, and every database on Postgres lives in it, which
the question says; `clickhouse` and its volume the same. The product's network stays until `docker compose down`; it costs
nothing.

There is no verb that switches everything off: an empty selection is a refusal. To stop
the host, `docker compose down` stops every container and keeps every volume; the next
apply brings them back.

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
- Each setting the manifest names on a database's user, such as n8n's `statement_timeout`,
  with `ALTER ROLE … SET` when the stored value differs. A setting the manifest stops naming
  is never reset.

A password is compared with the user's stored SCRAM verifier and changed only when they
differ, so a re-run changes nothing, a hand-edited or restored `.env` heals itself, and
rotating a password is one edit plus an apply. Passwords travel on stdin, never on a
command line. Nothing is ever dropped.

On ClickHouse the same, through `docker exec clickhouse clickhouse-client` as the admin user
`default`, whose password the image itself rewrites from `.env` on every start: each
switched-on container's database and user, and the grants under *ClickHouse* below, added
only when `SHOW GRANTS` would change. ClickHouse keeps no hash a program can recompute, so a
password is checked by logging in as the user, which fails for real there, and set only when
the login fails. The client reads its password from its environment, so none is on a command
line.

## ClickHouse

userland runs ClickHouse as one container. langfuse calls that development-only, because one
box has no redundancy. Every event langfuse ingests is written to your bucket first, and
Postgres holds everything you configure; ClickHouse holds what you see in the UI. A backup of
its volume is not here yet.

The image is `clickhouse/clickhouse-server:26.8`, the long-term-support line after the 26.4
that langfuse recommends, and it moves within that line. The container runs at ClickHouse's
own defaults, in UTC, which langfuse requires, with the one setting the image documents,
`nofile 262144`. `CLICKHOUSE_PASSWORD` is the admin user `default`, which provisioning uses
and no product does; the image turns on access management for it, so it may create users.

**Every product gets its own database and user on ClickHouse, made by provisioning, exactly
as on Postgres.** The template holds nothing product-specific, and the image's `CLICKHOUSE_DB`
is not used: it acts only on a first start with an empty volume, and would put a product's
name in the central template. The user is named as its database and holds, on that database
alone, what langfuse documents its user needs: `SELECT`, `INSERT`, `ALTER UPDATE`,
`ALTER DELETE`, `CREATE`, `DROP TABLE`, `DROP VIEW`, the column, index and view `ALTER`s,
`SYSTEM SYNC REPLICA`, `SYSTEM MERGES` and `ALTER SETTINGS`; and `SELECT` on the columns of
`system.parts`, `system.mutations` and `system.tables` it reads, on `system.processes` and on
`system.query_log*`. It cannot read another database, make one, or make a user.

**ClickHouse is the heaviest container here.** `CLICKHOUSE_MEM_LIMIT` in `.env` is where a cap
goes: ClickHouse reads the cgroup limit and keeps its own ceiling at nine tenths of it, so a
compose limit is one it respects rather than one it dies against. No number is written here.

ClickHouse logs at trace level to files inside the container, in `/var/log/clickhouse-server`,
rotated by the image; `docker logs clickhouse` shows only the entrypoint. Nothing is published
on the host: products reach it on `userland_clickhouse`, ports 8123 for HTTP and 9000 for the
native protocol, and you reach it with `docker exec clickhouse clickhouse-client`.

## n8n

n8n is two containers. `n8n` is the editor, the webhooks and the schedules, and it runs every
workflow itself. `n8n-runners` runs every Code node, in a container of its own with its own
user, and reaches nothing but n8n's task broker on port 5679, which nothing routes. The two
images come from two registries, and that is not a mistake: `docker.n8n.io` mirrors
`n8nio/n8n` alone and answers `NAME_UNKNOWN` for the runners image, so that one comes from
Docker Hub. **The two tags are one version**, and every upgrade moves both.

Leave `n8n-runners` off and n8n falls back to its own default, running Code nodes inside its
own container, which n8n calls internal mode and does not recommend for an instance that holds
credentials. The interview warns, and the template follows the selection.

**Its database is reached through the transaction door**, and two facts follow from that
door alone. n8n applies its query time limit by sending `SET statement_timeout` on every
connection it opens, and the transaction door discards a `SET`, so the template tells n8n to
send none (`DB_POSTGRESDB_STATEMENT_TIMEOUT: 0`) and the manifest puts the same limit, n8n's
own five minutes, on the `n8n` user instead, where Postgres applies it as each connection
starts and the door cannot touch it. For the same reason **the schema stays `public`**: any
other name is set by a `SET search_path` the door discards just the same, and n8n would read
and write `public` regardless. `public` is n8n's default, so the manifest never names it.

**`N8N_ENCRYPTION_KEY` is a one-way door.** Every saved credential is encrypted with it; it is
not in the database and cannot be derived, so losing it loses every credential for good. The
interview generates it before the first start, because n8n otherwise writes one of its own
into the volume where you would have to go and find it, and the CLI names the line when it
finishes: copy it off the machine, or switch on fort.

**The volume needs no backup.** Postgres holds the workflows, the credentials and every
execution, so the Postgres backup covers them. `n8n_data` holds only what n8n rebuilds: the
binary data of an execution, which n8n prunes together with the execution that owns it; the
settings file, which comes back from `.env` because the key is pinned there; the node cache;
and any community node, which `N8N_REINSTALL_MISSING_PACKAGES` reinstalls from n8n's own
database record at start. A file a workflow must keep is the workflow's job: write it to
durable storage from the workflow itself, because n8n deletes from that volume on its own
schedule. Binary data stays on the filesystem, n8n's default in this mode, and that is a
one-way door too: a later change of mode does not move the old files.

**Behind traefik**, n8n is told there is exactly one proxy (`N8N_PROXY_HOPS: 1`), so it trusts
one forwarded address and no more; a larger number would let a client forge its own. In local
visibility the template also turns off the secure flag on n8n's cookie, because n8n refuses to
serve its editor over plain HTTP from any hostname but `localhost` or `127.0.0.1`, and
`n8n.localhost` is not exempt; Safari refuses regardless of hostname. Without traefik, n8n
listens on `127.0.0.1:5678` and advertises its own default URLs, which are exactly that.

**Time.** `TZ=UTC` sets the clock, as everywhere. `GENERIC_TIMEZONE` sets what a schedule
means by 03:00, and defaults to `UTC` here rather than n8n's `America/New_York`; add the line
to `.env` to change it for the instance, and any workflow may set its own.

**Health.** The healthcheck asks `/healthz/readiness`, which answers 200 only once the
database is connected, the migrations are done and the start has finished; `/healthz` answers
ok at all times and says nothing about the database. It runs `node`, the one binary the image
is certain to carry, rather than `curl`. `n8n-runners` waits for it.

**Upgrading.** An upgrade runs the new version's migrations at start; a failure is fatal, and
many migrations have no way back, so an upgrade is an irreversible change to the database and
never runs by itself: both tags are exact. Before moving them, read every breaking-changes
entry between the two versions and take a fresh dump of the `n8n` database. A downgrade is a
restore from that dump.

**Two things only you can enforce.** A Postgres Trigger node holds its own credential and uses
`LISTEN`, which the transaction door drops silently: point that credential at
`pgbouncer-session:5432`, or at `postgres-18:5432` directly, never at the door n8n itself uses.
And `N8N_PORT` in `.env` is userland's loopback-port variable, as for every HTTP container;
n8n never sees it and always listens on 5678 inside its container.

n8n runs in n8n's regular mode: no queue, no worker, no Redis. Pruning, the pool and every
other number run at n8n's defaults.

## For a consumer

A **consumer** is a project of your own that uses userland and is not part of it. The
Contract is everything it needs, and `./bootstrap contract` prints it for the visibility
you are in:

- **Two networks**, `userland_postgres` and `userland_traefik`, which the consumer's compose
  file declares as `external: true` and joins.
- **Two doors to Postgres**, both on port 5432, and the DSN names one. `pgbouncer-transaction`
  is the default, for a consumer that keeps no state on a connection between transactions.
  `pgbouncer-session` is for one that does, whether a `SET`, a `LISTEN`, a session-scoped
  advisory lock or a prepared statement it reuses; it pins one Postgres connection for as
  long as the consumer holds its own, so the consumer must release connections promptly.
  `pg_dump`, pgadmin and PostgREST bypass the doors and name `postgres-18:5432` directly.
- **traefik's labels**: the rule (`NAME.localhost`, or `NAME.DOMAIN` in public), the
  entrypoint (`web`, or `websecure` in public with the certificate resolver `letsencrypt`),
  the port, and `traefik.docker.network=userland_traefik`.

`./bootstrap postgres database add NAME` makes the consumer's database and a user of the same
name that owns it, with `CONNECT` revoked from everyone else, `CREATE` on `public` revoked,
and the `vector` extension installed, then prints the DSN once, followed by the Contract.
userland keeps no copy of that password you can read back: paste it into the consumer's own
gitignored `.env`, and keep the record where you keep such things. A name that exists stops
the verb rather than overwriting, and a product's database is refused, since provisioning
makes those. `--session` prints a DSN that names the session door instead. `--api` adds the
PostgREST recipe inside the database, an `api` schema owned by the consumer, an
authenticator user that holds no table rights and inherits none, an anonymous user that
cannot log in, and an event trigger that tells PostgREST to reload its schema cache after a
migration, and prints a second DSN for PostgREST that names `postgres-18` directly.

`./bootstrap postgres database remove NAME` drops the database and every user the recipe
made, after naming them and asking. ClickHouse stays out of the Contract until a consumer
needs it. Redis never enters it: a product that needs Redis runs its own, and so does a
consumer.

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
| `main.go`, `internal/` | The CLI, in Go, on the standard library plus [huh](https://github.com/charmbracelet/huh) for the prompts and [cobra](https://github.com/spf13/cobra) for the verbs. |

`compose.yml`, `.env` and the `userland` binary are yours and untracked. Support
directories are grouped **by kind, at the root** — `scripts/`, never `metabase/scripts/`.
One container's files are spread across several of them on purpose: the thing you switch
on and off is a container, not a folder.

## Adding a container

A container is a block in `manifest.json` and a `{{ define "<container>" }}` in its
product's `compose/<product>.yml`. `./bootstrap new PRODUCT CONTAINER…` writes both with
placeholders and regenerates `VARIABLES.md`, so you start green; `check` then tells you what
is missing as you fill them in.

### The manifest

| Field | Meaning |
|---|---|
| `requires` / `optional` | Containers this one depends on. A missing required one is a refusal; a missing optional one is a warning. `depends_on` is emitted from `requires`, with `condition: service_healthy`. |
| `http` | `container` port, `host` loopback port, and `subdomain` (defaults to the name). Drives the traefik labels and the `ports` block. |
| `postgres` | The `database` this container gets on Postgres, which is also its user's name, and the `.env` variable holding its `password`. Two containers naming one database share it, and must name the same `settings`. It implies `postgres-18` is on. Optional `settings`, as `{"statement_timeout": "5min"}`, are Postgres settings provisioning puts on the user with `ALTER ROLE … SET`, so they reach every connection regardless of the door. |
| `clickhouse` | The `database` this container gets on ClickHouse, also its user's name, and the `.env` variable holding its `password`; a different variable from the Postgres one. Two containers naming one database share it. It implies `clickhouse` is on. No `settings`. |
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
  `.Visibility`, `.Postgres`, `.ClickHouse` and `.HTTP` are in scope, and `{{ ref "VAR" }}`
  renders `${VAR}`. Never write a value where a reference will do.
- `.On "traefik"` says whether another container is in the selection, so a template can
  follow it: n8n points at its runners only while they are on, and sets its URLs only while
  traefik is.
- A container anything requires needs a healthcheck. Postgres's must probe over TCP:
  over the socket it is green while the image's temporary first-start server is up.
- A named volume is both a line under `volumes` in the manifest and a mount in the
  template.
- A variable read without a default is asked in the manifest or is a database password;
  one with a default, `${VAR:-value}`, is optional and lands in `VARIABLES.md` by itself.

### Names the CLI knows

Six names are kinds the CLI defines rather than manifest data: `traefik`, whose
presence decides labels and loopback ports; `postgres-18`, which provisioning execs into
and which every container with a Postgres database requires; `clickhouse`, the same for a
ClickHouse database; `pgbouncer-transaction` and `pgbouncer-session`, the two doors the
Contract names; and `pgbouncer_auth`, the user both doors look passwords up with.

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
  client ceiling. Measure first; a number guessed in advance is worse than none. n8n's
  five-minute query limit is n8n's own default, moved onto its Postgres user because the
  door discards it where n8n sets it; it is not a number of ours.

See [CONTEXT.md](CONTEXT.md) for the language this repo uses.
