# userland

One host, one compose project, and containers you switch on and off. Clone it, run one
command, answer what it asks, and the host ends up running a reverse proxy, a set of
shared datastores, and whichever applications you switched on — each behind TLS, each
with its database already provisioned and already being backed up.

There is no application code here. userland is the ground your own projects stand on,
and it is deliberately not one of them.

> **This repo is being built in the open.** Today the interview writes `.env`, every verb
> exists, and userland renders and runs traefik, the whole of Postgres — its doors and pgadmin —
> ClickHouse, Metabase, n8n, and fort, which backs all of it up. The other containers arrive one at a time. The design is published as issues on this repo as
> it is settled.

`userland` is the part of a running system that is not the kernel: everything the machine
runs *for you*. This repo is that layer, for one host.

## What you can switch on

| | |
|---|---|
| **The proxy** | traefik, terminating TLS for everything else. |
| **The datastores** | One Postgres and one ClickHouse, shared: one of each for the whole host, never one per application. Postgres brings two doors and pgadmin. Redis is the exception: a product that needs it runs its own, inside the product, and nothing else is pointed at it. |
| **The applications** | n8n, Metabase, Langfuse, Twenty, neo4j. |
| **fort** | Keeps the files you name, `.env` first, and every database on Postgres and ClickHouse, in a bucket of its own as [restic](https://restic.net) snapshots, under a master key that never touches the host. |

Two things are pointed at rather than run: an S3-compatible object store you bring, which
fort and Langfuse each need a bucket of, and a secret store you own, which holds fort's
master key.

The interview offers to make each of those for you on AWS, with admin credentials it uses
once and never writes, and it adopts what already exists in your account rather than making
a second one. Decline, and it prints a checklist with your names filled in, for any
S3-compatible provider. Each bucket is reached by an access key of its own. fort's may delete
under `locks/` and nowhere else, so a compromised host cannot erase its own archives;
Langfuse's may delete, because its Data Retention feature does. Nothing may ever expire in
fort's, and *Object store* says why.

Each application gets its own database on the shared Postgres, owned by a user of the same
name. fort discovers databases by reading each server rather than by being handed a list,
so a database is backed up from the day it exists.

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
the product's name, so `postgres` has `database` and `fort` has its four.
Every verb that changes `.env` ends with an apply, and every verb that drops something asks
first and defaults to no.

### What apply does

1. Validates the selection: nothing on is refused, and so is a container whose required
   dependency is off. A missing optional dependency is a warning.
2. Renders `compose.yml`. Only switched-on containers are in it, every value is a `${VAR}`
   reference, and there are no profiles.
3. If Postgres or ClickHouse is on, brings them up first and waits for them to be healthy,
   then provisions.
4. Brings up everything else with `--remove-orphans` and `--build`. A container absent from
   the file is an orphan, so switching it off is enough to remove it; its volume and its
   database stay. `--build` is for fort, the one image this repo builds.
5. Prints the URL of everything that answers HTTP and the `.env` lines you must copy off
   the machine because they cannot be regenerated.
6. If fort is on, backs up. Every apply ends with a backup, so a host is kept from the day
   fort is switched on, and a listed file that has gone missing or a database that will not
   dump fails the apply rather than being noticed a month later.

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
   A container's external dependencies come after its own variables: the name and the
   region, then the offer to make it on AWS, then the checklist and the rest, under *Object
   store* and *Secret store* below.
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

## Object store

userland never runs an object store. A container that needs a bucket says so in the manifest,
`"external": {"FORT_S3": {"kind": "bucket", "versioned": true}}`, and the kind supplies five
variables under that prefix: `_BUCKET`, `_REGION`, `_ENDPOINT`, `_ACCESS_KEY_ID` and
`_SECRET_ACCESS_KEY`. The interview asks the name (enter to generate
`userland-<dependency>-<8 hex>`, or type one you made) and the region, then offers to make the
rest on AWS. Decline, the default, and it prints a checklist with your names in it and asks for
the endpoint and the access key. A dependency whose five variables are in `.env` is never asked
again, and two containers naming one prefix share the bucket. The region is text with no
default, because `auto` is a real answer on Cloudflare R2.

**The offer.** *Create it on AWS now?* Yes asks for an admin access key id, its secret and, if
it has one, a session token, once per run. They go into the environment of
`docker run --rm amazon/aws-cli` and are written nowhere, and the CLI says so when it finishes.
The steps: `sts get-caller-identity`; the bucket, `head-bucket` then `create-bucket` in the
region; versioning on when the kind says `versioned`; an IAM user named as the bucket; an inline
policy named `userland` on that user, written on every run; an access key. Then the five
variables land in `.env`, with the endpoint `https://s3.<region>.amazonaws.com`. What already
exists is **adopted**, never overwritten: a bucket you own is reused and its versioning
re-applied, a user that exists gets the policy re-applied and you are asked to paste one of its
keys or to mint one, and a bucket name another account owns is refused and asked again. A user
already holding two access keys stops the run, since AWS allows no third. The offer reads
`AWS_ENDPOINT_URL` from your environment, as the aws CLI does, and nothing else about it is
configurable. Outside the offer nothing in the interview reaches the network.

**The keys, and what each may do.** Every key reaches its one bucket and nothing else. It lists
the bucket, gets and puts objects, and aborts a multipart upload, since a killed upload leaves
parts behind and abort can never remove a finished object. `delete` says what else it may remove.
Left out, nothing, and that is the default, so a compromised host cannot erase its own archives.
`"*"` means any object in the bucket; langfuse's is the one, because its Data Retention feature
deletes. A prefix such as `"locks/*"` means objects under that prefix and nowhere else; fort's is
the one, so a backup can clear the lock file it just wrote and cannot touch the archive. Nothing
in userland reads whether versioning is on, so no key may. This is the document the offer writes
for a bucket with no `delete`; `"*"` adds `s3:DeleteObject` to the second statement, and a prefix
adds a third:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {"Effect": "Allow", "Action": ["s3:ListBucket"], "Resource": "arn:aws:s3:::BUCKET"},
    {"Effect": "Allow", "Action": ["s3:GetObject", "s3:PutObject", "s3:AbortMultipartUpload"], "Resource": "arn:aws:s3:::BUCKET/*"},
    {"Effect": "Allow", "Action": ["s3:DeleteObject"], "Resource": "arn:aws:s3:::BUCKET/locks/*"}
  ]
}
```

**Retention is yours, except where nothing may expire.** Nothing in userland deletes from a
bucket whose key cannot, and the CLI never writes a lifecycle rule: any rule is set by you, at
your provider. The first time a bucket's container is switched on, the CLI says so; without a
rule the bucket grows. A bucket the manifest marks `"never_expire": true` is the
exception, and it is not a preference. What it holds is one archive whose parts point at each
other, so an object removed by age takes with it every later part that pointed at it. There the
CLI tells you to set no rule at all, and S3 performs an expiration itself, so no bucket policy
can stop one you set by mistake. fort's bucket is the one.

**By hand, at any provider.** The checklist the interview prints is the short form of this.

- **AWS.** Bucket, then user, then the inline policy above, then an access key. New buckets
  block public access, disable ACLs and encrypt at rest by default, so nothing else is set.
  Retention is a lifecycle rule, where a bucket allows one. **No rule of any kind on fort's
  bucket**, not even a
  noncurrent-version expiration: the only versions that ever appear there are the ones
  something else left, which are both the evidence and the way back. AWS also recommends a
  rule that aborts incomplete multipart uploads after a few days; that one is yours too,
  and it never fires for fort, whose objects are far below one part.
- **Backblaze B2.** An application key restricted to the one bucket with `listFiles`,
  `readFiles` and `writeFiles`, adding `deleteFiles` only for a `delete` bucket. `writeFiles`
  without `deleteFiles` is the no-delete key. A B2 key carries one capability list for the
  whole key, so **`delete` cannot be scoped to a prefix here**: fort's key either deletes
  everywhere or nowhere, and *fort* says what each costs. Every B2 bucket keeps versions, so
  the overwrite guard is there by default — keep it that way and ignore restic's own advice
  to add a "keep only the last version" rule, which is for repositories that prune and would
  throw the guard away. Retention is B2's lifecycle rules; through the S3 API an expiration
  rule is paired with a delete-marker rule, and neither belongs on fort's bucket. Endpoint
  `https://s3.<region>.backblazeb2.com`, region as in the endpoint. **This is the provider to
  pick without an AWS account.**
- **Cloudflare R2.** A token of *Object Read & Write* scoped to the bucket. There is no level
  that writes without deleting, so on R2 fort's key can delete anywhere in its bucket, and a
  compromised host could erase its own archives there. R2 has no versioning either, so fort's
  overwrite guard is absent as well; fort's own history is unaffected, because it never lived
  in versions. Both of those are R2's floor, not a setting: R2 is the weakest of the three for
  fort. Lifecycle rules exist and are prefix-scoped, and none belongs on fort's bucket.
  Endpoint `https://<account id>.r2.cloudflarestorage.com`, region `auto`. Virtual-hosted
  requests are accepted, so no path-style setting is needed.

## Secret store

fort's master key lives in a secret store you own, never on the host. The kind is
`"external": {"FORT_KEY": {"kind": "secret-store"}}`; its five variables are `_PROVIDER`
(`ssm`, AWS Parameter Store, the one there is), `_NAME`, `_REGION`, `_ACCESS_KEY_ID` and
`_SECRET_ACCESS_KEY`, and the interview asks them the same way: the name (enter to generate
`/userland/<dependency>-<8 hex>`, or type one you made), the region, then the offer or the
checklist.

The offer writes a `SecureString` parameter with a 32-byte random value it never shows and
never overwrites: a parameter that exists is adopted, because a replaced master key would make
every archive fort ever wrote unreadable. Then a user named from the parameter with `/` made `-`,
this policy, and an access key. `NAME` is the parameter's name without its leading slash, and
`KEY-ID` is the account's `aws/ssm` key, which `kms describe-key --key-id alias/aws/ssm`
returns; a `SecureString` written without a key of your own is encrypted under it.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {"Effect": "Allow", "Action": ["ssm:GetParameter"], "Resource": "arn:aws:ssm:REGION:ACCOUNT:parameter/NAME"},
    {"Effect": "Allow", "Action": ["kms:Decrypt"], "Resource": "arn:aws:kms:REGION:ACCOUNT:key/KEY-ID"}
  ]
}
```

By hand: the parameter, the user, the policy, the access key, in that order. The key may only
read that one parameter, and nothing it holds writes.

## traefik

traefik is the one container that publishes on every interface, on ports 80 and 443. Every
HTTP container is reached through it by the labels the generator writes from the manifest,
and none holds a certificate or a redirect of its own: in public visibility the `web`
entrypoint sends every plain-HTTP request to `websecure` before any router is matched.

**Certificates come over a DNS-01 challenge**, from Let's Encrypt, through the provider
`DNS_PROVIDER` names. A certificate can therefore be issued before the hostname has a public
DNS record, but nothing answers on the hostname until it does. The propagation check asks the
public resolvers `1.1.1.1` and `8.8.8.8` rather than the host's own, and on Route 53 the
template passes no `AWS_HOSTED_ZONE_ID`, so the zone of each hostname is found for it; both
are what let one host hold certificates in more than one DNS zone.

**It logs at `INFO`**, not traefik's default of `ERROR`, so every certificate issued or renewed
is a line in `docker logs traefik`. At `ERROR` a renewal that succeeded logs nothing, and you
cannot tell it apart from one that never ran.

## Postgres

Four containers, each switched on by itself. `postgres-18` is the server: one Postgres for
the whole host, with a database per product and per consumer, each owned by a user of the
same name. `pgbouncer-transaction` and `pgbouncer-session` are the two doors, under *For a
consumer*. `pgadmin` is the browser UI. Every database is archived by fort, under *fort*.

**pgadmin and fort bypass both doors**: each names `postgres-18:5432` directly. `pg_dump`
through a transaction pooler fails, and pgadmin keeps session state on its connections.

### pgadmin

It registers exactly one server, `postgres-18`, from
[`config/pgadmin-servers.json`](config/pgadmin-servers.json), loaded into an empty
`pgadmin_data` volume at the first start and never again.
`PGADMIN_REPLACE_SERVERS_ON_STARTUP` is deliberately not set: it deletes every server row
and re-imports at each start, and a password saved in the browser is part of the row it
deletes. The trade is that editing that file does not reach a pgadmin that has already
run — change the server in the browser too, or switch pgadmin off, reclaim its volume and
switch it back on to re-seed.

That server connects as the superuser, and its password is not in the file: paste
`POSTGRES_PASSWORD` from `.env` the first time, and pgadmin keeps an encrypted copy in its
volume if you tick *Save password*.

**Its own login is the only lock**, and it stands in front of the superuser of every
database on this host. The email and password the interview asks for are what you sign in
with; nothing else is in the way, in either visibility. The container is created with them
only when its volume is empty, so changing either line in `.env` afterwards does not change
the login of a pgadmin that has already started — switch it off, reclaim the volume, and
switch it on again, which costs one server row and a saved password. The image floats on
`latest` on purpose, so security fixes arrive without review, and its volume needs no
backup for the same reason.

Its session cookie is marked secure only in public visibility, where there is TLS for it to
ride; in local, a secure cookie is a login that never completes. It is told there is exactly
one proxy in front of it while traefik is on, and none while traefik is off.

## ClickHouse

userland runs ClickHouse as one container. langfuse calls that development-only, because one
box has no redundancy. Every event langfuse ingests is written to your bucket first, and
Postgres holds everything you configure; ClickHouse holds what you see in the UI. fort
archives every database on it, under *fort*.

The image is `clickhouse/clickhouse-server:26.8`, the long-term-support line after the 26.4
that langfuse recommends, and it moves within that line. The container runs at ClickHouse's
own defaults, in UTC, which langfuse requires, with the one setting the image documents,
`nofile 262144`. `CLICKHOUSE_PASSWORD` is the admin user `default`, which provisioning and
fort use and no product does; the image turns on access management for it, so it may create
users.

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

Its backup directory, `/var/lib/clickhouse/backups`, is a volume of its own,
`clickhouse_backups`. fort mounts it too while both are on, and it holds nothing between
runs.

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
execution, so fort's archive of Postgres covers them. `n8n_data` holds only what n8n rebuilds: the
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

## Metabase

Metabase is one container and one JVM: the web UI, the query engine, the scheduler, and the
MCP server at `/api/metabase-mcp`. Its database, `metabase` on Postgres, holds every
dashboard, question, user and setting, and the credentials of every data source you connect.
Without traefik it listens on `127.0.0.1:3000`.

**Its database is reached through the transaction door.** Nothing Metabase does against its
own database needs the session door: it takes no advisory lock there, and an upgrade from
v0.50 to v0.63 ran 833 migrations through the transaction door without a warning from the
door.

**A data source is a credential of your own, and so is its door.** A database you connect in
Metabase's Admin is reached by a connection Metabase makes with the host and port you type
there, and nothing in `.env` reaches it. Metabase's Postgres driver sends `SET SESSION
TIMEZONE` before a query when a report timezone is set, and `SET ROLE` when impersonation is
on, and the transaction door discards both. Point a data source that uses either at
`pgbouncer-session:5432`, or at `postgres-18:5432` directly; one that uses neither may take
the transaction door like anything else.

**`MB_ENCRYPTION_SECRET_KEY` is a one-way door.** It encrypts the secret columns of Metabase's
database, the data-source credentials above all; without it they sit in clear in the database
and in every archive of it. The interview generates one before the first start, or takes
yours, so a database userland made is encrypted from its first boot. Metabase's five cases, as
observed on v0.63.15:

| Its database | The key | What happens |
|---|---|---|
| unencrypted | none | starts, and logs that encryption is disabled |
| unencrypted | a new one | starts, and encrypts the database in place on that start |
| encrypted | the right one | starts |
| encrypted | a wrong one | exits 1 before any migration runs |
| encrypted | none | exits 1 before any migration runs |

The last two restart forever, and behind traefik they read as a 404 rather than an error,
because traefik routes no container whose health is still `starting`. `remove-encryption` and
`rotate-encryption-key` both need the key you lost, so the only way back is a dump taken
before encryption was on, and for a database userland made there is none: every dashboard and
question is rebuilt by hand. The CLI names the line when it finishes. Copy it off the machine,
or switch on fort, which keeps `.env` in a bucket apart from the Postgres archives; an archive
and the key that opens it in one place are one loss, not two.

**Set the Site URL at install.** Admin → Settings → General → Site URL, to the address you
reach it on: `https://metabase.DOMAIN` in public, `http://metabase.localhost` in local,
`http://127.0.0.1:3000` without traefik. It is a row in Metabase's database, and the template
does not set it. Metabase builds more than its email links from it: the OAuth discovery of its
MCP server, every endpoint that server advertises and its `WWW-Authenticate` challenge all
derive from it, so a client registered against one address stops matching when it changes,
and registering again is the only fix. Set it before any MCP client registers, and again after
`set VISIBILITY`. Behind traefik, Metabase sees traefik's plain-HTTP hop, so an address it
guesses for itself can read `http://` in public visibility.

**The MCP server** is part of the application, on every edition. A client signs in over OAuth
2.0 against a server Metabase embeds, and its token carries the permissions of the account
that authorised it, so a connection is per person rather than a shared key. It is governed in
Admin, not here.

**Leave Metabase's own Redirect to HTTPS off.** In public visibility traefik's entrypoint
already redirects, so the setting adds nothing, and it turns into a redirect loop if
`X-Forwarded-Proto` ever stops arriving.

**Upgrading.** The tag is exact and moves only when this repo moves it, so a `git pull` that
moves it in `compose/metabase.yml` is an upgrade, and it runs at the next apply. Metabase runs
the new version's migrations at start, a failure is fatal, and a downgrade is not the way
back: `migrate down` moves one major per run, from the newer binary, and cannot undo what
happens at start rather than in a migration, so Metabase's own advice is to restore a dump.
Before that apply:

1. Read every release note between the two versions. What bites is rarely in the migrations:
   a major can move the sample database's engine, break the driver plugin API so a
   third-party driver needs rebuilding, or move the bundled JVM.
2. Take a fresh archive with `./bootstrap fort backup`. Last night's is not one minute ago,
   and this one is the rollback.
3. Rehearse on another machine: restore that archive into a throwaway Postgres, start the new
   tag against it, and compare the counts of dashboards, questions and users, `/api/health`,
   and the schema version in the log. The rehearsal needs the key, since an encrypted
   database does not start without it, so that machine holds production data and the key to
   its credentials: give it no route to your data sources, and destroy it afterwards.

**Health.** The healthcheck asks `/api/health`, which answers 200 once the application has
started and its database is reachable; `start_period` is 120 seconds, the JVM's start plus a
first boot's migrations. `/dev/urandom` is mounted over `/dev/random`, because the JVM blocks
on a starved entropy pool, and that shows as a start that hangs rather than one that fails.

**The volume needs no backup.** Metabase downloads its own driver JARs into
`metabase_plugins` at start. A third-party driver you put there by hand is the one thing that
would not come back: keep your own copy, and expect to rebuild it after a major upgrade. The
database is on Postgres, so fort archives it with everything else.

The two row limits, both connection pools and every other number run at Metabase's defaults.

## fort

fort keeps off this host what you cannot lose with it: the files you name, and every database
on Postgres and ClickHouse while each is on. It is one container and it requires nothing; it
is the only thing here that backs up something outside userland, and the only thing that backs
up anything at all.

`FORT_FILES` is the list: absolute paths, separated by colons. Switching fort on puts this
clone's `.env` at the head of it, because that one file holds every secret of every container
you switched on. `./bootstrap fort add PATH…` adds more and refuses a path that is not on this
host; `remove PATH…` stops keeping one, and refuses this clone's `.env` while fort is on. Each
listed path has its **directory** mounted read-only at the same place under `/files`, so fort
can read the file and can write nothing.

**Postgres.** While `postgres-18` is on, every run archives the globals and every database but
`postgres`, read from `pg_database`, so **no database is ever named**: one is archived from the
day it exists, and one that is dropped stops appearing. Each is `pg_dump` in custom format
streamed straight into restic, so nothing is staged on disk, and a dump that fails saves no
snapshot. The dump is left uncompressed, because restic compresses what it stores and finds
far more to deduplicate in a dump that is not already compressed. fort connects as the
superuser, to `postgres-18:5432` directly, with the `pg_dump` 18 its image carries; the client
has to match the server's major. The globals file is there because a user is a **cluster**
object: it lives outside every database, so `pg_dump` does not carry it, and a database restored
into a Postgres that holds no users fails on the first `ALTER TABLE … OWNER TO`. That one file
carries the stored password verifier of every user on the server, so it is as sensitive as the
data.

**ClickHouse.** While `clickhouse` is on, every run archives every database but ClickHouse's
own three, `system`, `information_schema` and `INFORMATION_SCHEMA`; `default` is included.
fort asks ClickHouse over HTTP, as `default`, to `BACKUP DATABASE … TO File(…)` into its backup
directory, which is the volume `clickhouse_backups` that both containers mount. restic reads
what ClickHouse wrote there, and fort empties it again. **The host needs free disk for one full
copy of ClickHouse's databases while a run is going.** fort hands that directory to uid 101,
the ClickHouse image's own user, before each run, because a volume Docker creates belongs to
root. The password reaches `curl` on its standard input, never on a command line. No file of
users is kept, because provisioning makes every ClickHouse user from `.env`. ClickHouse can
write a backup to S3 by itself, and fort does not use that: a backup to S3 deletes its own lock
file when it finishes, so a key that may not delete fails every one, and ClickHouse cannot
encrypt an archive it writes to S3 at all.

**What is in the bucket.** restic snapshots, and nothing you can read without the master key.
Each object is named after the hash of its own contents, so nothing is ever overwritten and
nothing is ever a file path; every backup adds snapshots, and the list of snapshots is the
history. One run makes one snapshot tagged `files`, one tagged `postgres` for the globals and
for each database, and one tagged `clickhouse` holding every ClickHouse database;
`docker exec fort restic snapshots` lists them. There is no `.gpg` next to a familiar name to
grab, and equally no way to get anything back except through restic with the key.

**The master key is the repository password**, read out of your secret store at the start of
every run by `scripts/fort-key`, held in memory, and written nowhere — not in `.env`, not on
disk, not in the bucket, which holds it only as ciphertext that the password unlocks. So
**replacing the parameter's value does not re-key anything; it locks fort out of its own
archive.** That is why the offer adopts a parameter that exists and never overwrites it.

**Retention: never prune, nothing expires.** fort only ever adds. Every run is a full backup,
so each snapshot restores alone, but restic stores only the chunks it has not seen before, so
a run adds roughly what changed since the last one. A large database that did not change adds
almost nothing. A small one is stored whole again whenever it changes at all, because it is
only a chunk or two, and the globals file changes on every run. **The archive keeps everything,
including what you delete**: a row dropped from a database, a trace langfuse's own Data
Retention removes, a file you stop keeping — every earlier snapshot still holds it. `forget` and
`prune`, which are how restic reclaims space, need delete rights the key does not have, and so
do `unlock --remove-all`, `rewrite` and `tag`: if you ever want them, restic's own guidance is a
separate, well-secured machine with a delete-capable key, never this host. And **set no
lifecycle rule on this bucket**: see *Object store*.

**Versioning guards against overwrite, not loss.** fort's key can put an object but not delete
one — and a put overwrites. A host that has been broken into can therefore write garbage over
any object under its own name, using fort's key, and restic sends nothing that would stop it.
With versioning on, the original is still there as an older version and you put it back by hand
with an identity of your own. Because restic never overwrites anything, **an older version in
this bucket means something other than restic wrote there.** `restic check` will tell you the
archive is damaged, because an object's contents no longer match its name, but it cannot repair
what it does not have.

**The image is this repo's own**, the only one it builds, so `apply` passes `--build`. It is
`restic/restic:0.19.1` plus `ssmget`, a small Go program built in a stage of its own that reads
the one parameter through Amazon's own library, and Alpine's `curl` and `postgresql18-client`.
The pin matters: a listed file that has gone missing exits 3 only since restic 0.19.0, and before
that it was a silent success. Process 1 is busybox `crond`, which the image already carries,
reading `FORT_SCHEDULE` (`@daily` unless you set it). crond hands a job almost no environment, so
the entrypoint saves its own with `export -p` into `/run/fort.env`, mode 600, and the scheduled
line sources it. `./bootstrap fort backup` runs the same backup inside the running container,
now, and every apply ends with one.

**A part that fails fails the run, and the rest is still kept.** A database whose dump fails
saves no snapshot, and a ClickHouse that cannot be reached saves none of its databases; the
files and every other database are kept regardless, and the run exits non-zero naming what was
not. A missing listed file is restic's own case: restic backs up what it can find, warns, and
exits 3, so the partial snapshot is real and it is now the latest, and the file that was missing
is not in it. A restore lists every path it is about to write before it writes anything, which
is where you see the gap; an earlier snapshot still holds the file. **Nobody is told when a
scheduled run fails**: until userland runs something that watches, `docker logs fort` and the
snapshot list are the evidence.

**Files come back from the bucket and the key alone.** `./bootstrap fort restore` works in a
clone with no `.env` at all — which is the case a restore is for. It asks where the bucket and
the master key are, builds the image, then runs it twice with no host mount: once to list the
latest `files` snapshot, so you see every path with the time that file was last changed, and
once to stream a tar which the CLI unpacks onto `/`. Every file lands back at its own absolute
path with its own mode and time, so `.env` comes back at 600. It assumes the host is laid out as
the old one was; run it as a user that may write those paths. Then `./bootstrap` brings the
stack up against what came back, and provisioning converges each database user to the password
the restored `.env` holds.

**Databases come back by hand, through the running fort, and a restore nobody has rehearsed is
not a backup.** Into a throwaway database on the running Postgres, which is the drill:

```sh
docker exec postgres-18 createdb -U postgres drill
docker exec fort restic dump --path /postgres/DATABASE.dump latest /postgres/DATABASE.dump \
  | docker exec -i postgres-18 pg_restore -U postgres --no-owner --no-acl -d drill
```

Back into the running server the user already exists, so drop `--no-owner --no-acl` and name
the real database. Into a Postgres that holds nothing, restore the globals first, streamed from
`/postgres/globals.sql` the same way into `psql -U postgres`, and the databases after it; the
only error it prints is that the image's own `postgres` user already exists. On ClickHouse, into
a database of another name:

```sh
docker exec fort restic dump --tag clickhouse latest:/clickhouse /DATABASE --archive tar \
  | docker exec -i -u clickhouse clickhouse tar -x -C /var/lib/clickhouse/backups
docker exec clickhouse clickhouse-client -q "RESTORE DATABASE DATABASE AS drill FROM File('DATABASE')"
```

fort's next run empties the backup directory again. An earlier snapshot takes its ID in place
of `latest`, and `docker exec fort restic snapshots --path /postgres/DATABASE.dump` lists one
database's.

**If you ran `pg-backup`.** Until 2026-09-23 Postgres had a fifth container, `pg-backup`, which
wrote gpg-encrypted dumps into a bucket of its own. The next run drops it from the selection and
says so. Its archives stay in that bucket, and `BACKUP_PASSPHRASE`, which is the only way to read
them, stays in `.env` with the `PG_BACKUP_` lines, because nothing here removes a secret that is
the last key to an archive. Delete those lines when you no longer need those archives.

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
- every variable a template reads without a default is asked by the manifest, is a
  database password, or is one an external kind supplies;
- `VARIABLES.md` matches the manifest and templates;
- compose accepts the rendered file;
- `TZ=UTC` reaches every service;
- every container something requires has a healthcheck, since `depends_on` waits on it;
- every named volume is declared on the container that mounts it, and vice versa.

It runs in CI on every pull request and on every push to `main`
([`.github/workflows/check.yml`](.github/workflows/check.yml)), followed by `go test`.
The manifest itself is refused on load for a container in two products, a volume declared
by two containers, two containers on one loopback port, an ask whose type or `when` the
interview does not know, or an external dependency of a kind it does not know or with a
property its kind has no use for.

## Layout

| Path | What lives there |
|---|---|
| `bootstrap` | Builds the CLI inside docker and runs it. The one command; docker is all it needs. |
| `manifest.json` | What every container depends on and what it needs asked. The CLI trusts nothing else. |
| `VARIABLES.md` | Every variable `.env` may hold, by container. Generated; `check` fails when it is stale. |
| `compose/` | One template per product. The CLI renders `compose.yml` from them; nothing is ever run from inside it. |
| `scripts/` | Shell that runs inside a container — fort's entrypoint, which is its schedule and its backup, and its password command. Nothing here runs on the host. |
| `Dockerfile` | fort's image, the only one this repo builds: restic, a reader for the secret store, and the Postgres and HTTP clients its backup needs. |
| `ssmget/` | That reader, a Go module of its own so the CLI never takes a cloud SDK as a dependency. |
| `initdb/` | First-start initialisation for a datastore. Runs once, against an empty volume, and never again. Empty today: what used to live here is provisioned instead. |
| `config/` | Configuration files a container mounts, checked in because they hold nothing secret. pgadmin's one server is the first. |
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
| `volumes` | Named volumes this container mounts. The top-level `volumes` block, "left behind" and `reclaim` all read this. A container may also mount a volume declared on one it depends on, as fort mounts ClickHouse's `clickhouse_backups`; the volume stays the declaring container's. |
| `asks` | `var`, `type`, `prompt`, optional `when` (`public`, `local` or `VAR=value`) and `keep`. Types: `text hostname email url port secret generated choice paths`, described under *The interview*. |
| `external` | `{"PREFIX": {"kind": "bucket"}}`, or `{"kind": "secret-store"}`. A bucket takes `"versioned": true`, `"never_expire": true`, and `"delete"` as `"*"` for any object or a prefix like `"locks/*"` for objects under it; left out, the key may never delete. The kind supplies five variables under the prefix, and the interview asks them with the offer and the checklist, under *Object store* and *Secret store*. Two containers naming one prefix share it and must describe it alike. |
| `files` | The variable holding absolute paths this container keeps, separated by colons. Each path's directory is mounted read-only at the same place under `/files`. fort is the one, under *fort*. |
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

Seven names are kinds the CLI defines rather than manifest data: `traefik`, whose
presence decides labels and loopback ports; `postgres-18`, which provisioning execs into
and which every container with a Postgres database requires; `clickhouse`, the same for a
ClickHouse database; `pgbouncer-transaction` and `pgbouncer-session`, the two doors the
Contract names; `pgbouncer_auth`, the user both doors look passwords up with; and `fort`,
whose `backup` execs into it, whose `restore` runs its image directly, and whose presence
makes an apply end with a backup. Two volume names are known too, `postgres_data` and
`clickhouse_data`, so that switching a datastore off says which volume every database lives
in.

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
