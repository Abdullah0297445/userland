# userland

One host, one compose project, and containers you switch on and off. This file is the
glossary. It holds no implementation detail.

## Language

**userland**:
This repo, and everything it runs on one host as one compose project. The word covers both
what you clone and what ends up running.
_Avoid_: platform, stack, engine, system, installation

**Container**:
The unit you switch on. One container is one compose service, and nothing larger or
smaller is ever switched; Postgres without pgadmin is a valid choice.
_Avoid_: service, unit, component, module, profile

**Template**:
A tracked file the CLI renders a container's compose definition from, one per product.
Which containers share a template carries no meaning, and nothing is ever run from one.
_Avoid_: fragment, compose file, stack file, override

**Datastore**:
A container that stores structured data, such as Postgres, Redis or ClickHouse.
_Avoid_: store, database (for the whole thing), engine

**Object store**:
An S3-compatible store for objects. userland points at one and never runs one, so it is
always an external dependency.
_Avoid_: file store, blob store, bucket (for the store), S3 (as the category), datastore

**Dependency**:
Something a container needs in order to run. Every dependency has a severity, required or
optional, and a type, internal or external.
_Avoid_: prerequisite, link, requirement

**Required**:
A dependency without which the container cannot run. A selection missing one is refused.
_Avoid_: must-have, hard, mandatory

**Optional**:
A dependency without which the container runs, but degraded. A selection missing one gets
a warning and proceeds.
_Avoid_: soft, nice-to-have, recommended

**Internal**:
The type of a dependency on another container.
_Avoid_: local, built-in, bundled

**External**:
The type of a dependency on something userland never runs, such as an object store. The
interview collects its address and access key rather than switching anything on.
_Avoid_: bring-your-own, third-party, remote, cloud

**Blocked by**:
A required dependency, read from the container that has it. langfuse-web is blocked by
Postgres.
_Avoid_: depends on, downstream of

**Blocking**:
A required dependency, read from the container that is depended on. Postgres is blocking
langfuse-web, and cannot be switched off while it is.
_Avoid_: dependents, upstream of, parent

**Visibility**:
Whether userland can be reached from the machine that runs it alone, or from the internet.
It is **local** (no domain, no certificate, this machine only) or **public** (real
hostnames, TLS), chosen once for the whole of userland and never per container.
_Avoid_: mode, environment, stage, dev/prod, exposure

**Consumer**:
A project of your own that uses userland and is not part of it. It may have a database on
Postgres and it may sit behind traefik; userland never runs it.
_Avoid_: tenant, client, app, application

**Contract**:
A short summary of what is on and how to reach each part of it, to glance at while starting
a consumer. Nothing depends on it.
_Avoid_: hand-off, handshake, connection details, exports, integration

## The interview

**Interview**:
One run of the CLI's questions. It ends in a selection or a refusal, and a re-run asks
only about what is new.
_Avoid_: wizard, setup, install, onboarding

**Selection**:
The set of containers switched on, as the interview last recorded it.
_Avoid_: enabled set, profile set, configuration

**Manifest**:
The one record the interview trusts for what every container depends on and what it needs
asked.
_Avoid_: registry, catalog, dependency file

**Render**:
Writing the one compose file the host runs, from the manifest, the templates and the
selection. It holds only what is switched on, it holds no secret, and nobody edits it.
_Avoid_: generate, build, compile

**Apply**:
Making the host match the selection and `.env`: render, bring up, provision, print. Every
change to the selection or to `.env` ends with one.
_Avoid_: deploy, sync, reconcile, up

**Refusal**:
The interview stopping on a selection that cannot run, naming what is missing.
_Avoid_: error, validation failure, abort

**Warning**:
The interview naming a selection that runs but is degraded, and proceeding.
_Avoid_: notice, hint, caution

**Provisioning**:
Making what a selection depends on exist before it runs, whether users, databases, buckets
or access keys. What userland's own containers need converges on `.env`; what a consumer
was given is never changed once it exists. A consumer is provisioned by the same path.
_Avoid_: seeding, bootstrapping, init, setup, migration

**Reclaim**:
Dropping what a switched-off container left behind, such as its volume or its database.
Only on request, and only after naming it.
_Avoid_: cleanup, purge, prune, delete

**Access key**:
A key pair that reaches one bucket, or one secret store, and nothing else. Two buckets
means two access keys, never shared, and fort's bucket and fort's master key are reached
by two.
_Avoid_: identity, IAM user, credentials, service account, token

**Offer**:
The interview proposing to make an external dependency for you on AWS, with admin
credentials it uses once and never writes. Declining it prints the checklist.
_Avoid_: auto-provisioning, create-it-for-me, wizard, setup

**Checklist**:
The steps to make an external dependency by hand at any provider, printed with your names
filled in when you decline the offer. The README holds the long form.
_Avoid_: manual path, instructions, guide, runbook

**Adopt**:
Reusing what already exists in your account, whether a bucket, a parameter or the AWS
user an access key belongs to, instead of making a second one. The offer adopts and never
overwrites.
_Avoid_: reuse, import, attach, take over

## Postgres and ClickHouse

**Database**:
One database on Postgres or ClickHouse, made by provisioning for one product, or on
Postgres for one consumer, and owned by a user of the same name. Every container that
connects as that user shares it. A database that serves customers of its own is still one
database. Nothing on Redis is a database in this sense: a product that needs Redis runs its
own, inside the product, and nothing else is ever pointed at it.
_Avoid_: tenant, schema (for the whole thing), account, workspace

**User**:
The one login a database is reached through. It carries the database's name, owns it, and
reaches no other database. Postgres calls a user that can log in a role; here it is a user.
_Avoid_: tenant, role (for this), account, owner (as its name)

**Superuser**:
The single Postgres superuser that provisions databases and that fort archives them as. No
other user holds it. On ClickHouse the same seat is the user `default`, which provisioning and
fort use.
_Avoid_: engine superuser, admin, root, postgres user

**Door**:
The container a connection to Postgres goes through. There are two, they differ only in how
long a connection may be held, and the DSN names one. A container is blocked by the door
its DSN names. pgadmin is not a door.
_Avoid_: pooler, pgbouncer (as the concept), endpoint, entrypoint

**Transaction door**:
The door for a container that keeps no state on the connection between transactions. It is
the default, and it lets many client connections share few Postgres connections.

**Session door**:
The door for a container that keeps state on the connection. It holds one Postgres
connection for as long as the container holds its own, so a container on this door must
release promptly.

**DSN**:
The connection string userland hands a consumer, or writes for a container. It names a
door.
_Avoid_: connection URL, database URL, credentials

**Archive**:
The backup of one database, or of one listed file. One archive restores one database or
one file alone.
_Avoid_: dump, backup file, snapshot, copy

**Retention**:
How long objects stay in their bucket. A rule at your provider sets it, never userland,
which deletes nothing it has written. fort's bucket may carry no rule at all, so its archives
stay until someone removes them with restic, from a machine whose key may delete.
_Avoid_: expiry, lifecycle (for the concept), cleanup, pruning

**Globals**:
The Postgres objects outside every database: the users and their passwords.
One backup holds them for the whole of Postgres. It is as sensitive as the data, and it
goes only into a Postgres being rebuilt.
_Avoid_: users, roles file, cluster objects

**Drill**:
A rehearsal of a recovery, against a throwaway, to prove that the recovery works. A backup
that no drill has restored is not a backup.
_Avoid_: test, dry run

### Roles inside a database, for PostgREST

These exist only in a database that wants a REST API. PostgREST is not a userland
container; it runs in the consumer's own repo.

**Authenticator**:
The only role a PostgREST process logs in as. It holds no table rights and inherits none,
so it can do nothing until it takes another role.
_Avoid_: service account, api user

**Anon role**:
The role a PostgREST request takes when it carries no valid token. It cannot log in
directly.
_Avoid_: public role, guest

## fort

**Listed file**:
A file on the host, named by its absolute path, that fort keeps. userland's own `.env` is
always one.
_Avoid_: env file, secret file, watched file, tracked file

**Master key**:
The one secret that encrypts every file fort keeps. It lives in a secret store, never on
the host, and fort reads it at each run. Lose it and every archive is waste.
_Avoid_: passphrase, backup key, encryption key, secret

**Secret store**:
A service you own, outside the host, that holds the master key. userland reads it and
never writes it. An external dependency.
_Avoid_: vault, parameter store (as the category), key store, KMS

**Restore**:
Writing every archive fort holds back onto the host at the path it came from, over
whatever is there. Deliberate, by hand, never on a schedule.
_Avoid_: recover, pull, sync
