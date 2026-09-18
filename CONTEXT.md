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

**Fragment**:
One compose file that the root includes. Which containers share a fragment carries no
meaning, and nothing is ever run from one.
_Avoid_: compose file, stack file, override

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
A project of your own that uses userland and is not part of it. It may be a Postgres
tenant and it may sit behind traefik; userland never runs it.
_Avoid_: tenant (for the outside thing), client, app, application

**Contract**:
Everything a consumer needs to use userland, and nothing about how userland runs.
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

**Refusal**:
The interview stopping on a selection that cannot run, naming what is missing.
_Avoid_: error, validation failure, abort

**Warning**:
The interview naming a selection that runs but is degraded, and proceeding.
_Avoid_: notice, hint, caution

**Provisioning**:
Making what a selection depends on exist before it runs, whether roles, databases, buckets
or access keys, and changing nothing that already exists. A consumer is provisioned by the
same path.
_Avoid_: seeding, bootstrapping, init, setup, migration

**Reclaim**:
Dropping what a switched-off container left behind, such as its volume or its database.
Only on request, and only after naming it.
_Avoid_: cleanup, purge, prune, delete

**Access key**:
A key pair that reaches one bucket and nothing else. Two buckets means two access keys,
never shared.
_Avoid_: identity, IAM user, credentials, service account, token

## Postgres

**Postgres tenant**:
One role and one database on Postgres, shared by every container that connects as it. A
tenant that serves customers of its own is still one tenant, and they are never tenants.
Nothing on Redis or ClickHouse is a tenant; "tenant" alone means this, and only here.
_Avoid_: user, account, owner, client, app

**Tenant database**:
The database Postgres holds for one tenant. The tenant's own role owns it.

**Tenant role**:
The one login role a tenant connects as. It owns its tenant database and reaches no other.
_Avoid_: user, account

**Superuser**:
The single Postgres superuser that provisions tenants and runs the backup. No tenant
holds it.
_Avoid_: engine superuser, admin, root, postgres user

**Door**:
The container a tenant connects through to reach Postgres. There are two, they differ only
in how long a tenant may hold a connection, and the DSN names one. A tenant's containers are
blocked by the door their DSN names. pgadmin is not a door.
_Avoid_: pooler, pgbouncer (as the concept), endpoint, entrypoint

**Transaction door**:
The door for a tenant that keeps no state on the connection between transactions. It is
the default, and it lets many tenant connections share few Postgres connections.

**Session door**:
The door for a tenant that keeps state on the connection. It holds one Postgres connection
for as long as the tenant holds its own, so a tenant on this door must release promptly.

**DSN**:
The connection string userland hands a tenant. It is part of the contract, and it names a
door.
_Avoid_: connection URL, database URL, credentials

**Archive**:
The backup of one tenant database. One archive restores one tenant alone.
_Avoid_: dump, backup file, snapshot

**Globals**:
The Postgres objects outside every tenant database: the tenant roles and their passwords.
One backup holds them for the whole of Postgres. It is as sensitive as the data, and it
goes only into a Postgres being rebuilt.
_Avoid_: users, roles file, cluster objects

**Drill**:
A rehearsal of a recovery, against a throwaway, to prove that the recovery works. A backup
that no drill has restored is not a backup.
_Avoid_: test, dry run

### Roles inside a tenant database, for PostgREST

These exist only in a tenant database that wants a REST API. PostgREST is not a userland
container; it runs in the consumer's own repo.

**Authenticator**:
The only role a PostgREST process logs in as. It holds no table rights and inherits none,
so it can do nothing until it takes another role.
_Avoid_: service account, api user

**Anon role**:
The role a PostgREST request takes when it carries no valid token. It cannot log in
directly.
_Avoid_: public role, guest
