# userland

One host, one compose project, and products you switch on and off. This file is the
glossary. It holds no implementation detail.

## Language

**userland**:
This repo, and everything it runs on one host as one compose project. The word covers both
what you clone and what ends up running.
_Avoid_: platform, stack, engine, system, installation

**Product**:
What you switch on: a set of containers that are always on together, such as langfuse's web,
worker and Redis. One product is one compose file. pgadmin is a product of its own, so
Postgres without pgadmin is a valid choice.
_Avoid_: stack, bundle, service, module, app

**Container**:
One compose service. Every container belongs to exactly one product, and is switched on and
off with it, never alone.
_Avoid_: service, unit, component, module, profile

**Selection**:
The set of products switched on.
_Avoid_: enabled set, profile set, configuration

**Datastore**:
A container that stores structured data, such as Postgres, Redis or ClickHouse.
_Avoid_: store, database (for the whole thing), engine

**Object store**:
An S3-compatible store for objects. userland points at one and never runs one, so it is
always an external dependency.
_Avoid_: file store, blob store, bucket (for the store), S3 (as the category), datastore

**Dependency**:
Something a product needs in order to run. Every dependency has a severity, required or
optional, and a type, internal or external.
_Avoid_: prerequisite, link, requirement

**Required**:
A dependency without which the product cannot run. A selection missing one is refused.
_Avoid_: must-have, hard, mandatory

**Optional**:
A dependency without which the product runs, but degraded.
_Avoid_: soft, nice-to-have, recommended

**Internal**:
The type of a dependency on another product.
_Avoid_: local, built-in, bundled

**External**:
The type of a dependency on something userland never runs, such as an object store. You
make it; userland is only told where it is and given the access key that reaches it.
_Avoid_: bring-your-own, third-party, remote, cloud

**Blocked by**:
A required dependency, read from the product that has it. langfuse is blocked by postgres.
_Avoid_: depends on, downstream of

**Blocking**:
A required dependency, read from the product that is depended on. postgres is blocking
langfuse, and cannot be switched off while it is.
_Avoid_: dependents, upstream of, parent

**Visibility**:
Whether userland answers to names only this machine resolves, or to real hostnames on the
internet. It is **local** (`*.localhost`, no domain, no certificate; reachable by anyone on a
network it shares who sends the name) or **public** (real hostnames, TLS), chosen once for the
whole of userland and never per product.
_Avoid_: mode, environment, stage, dev/prod, exposure

**Consumer**:
A project of your own that uses userland and is not part of it. It may have a database on
Postgres and it may sit behind traefik; userland never runs it.
_Avoid_: tenant, client, app, application

**Provisioning**:
Making the users and databases a product needs exist before it runs. What userland's own
products need converges on their settings; what a consumer was given is never changed once it
exists. A consumer is provisioned by the same path.
_Avoid_: seeding, bootstrapping, init, setup, migration

**Access key**:
A key pair that reaches one bucket, or one secret store, and nothing else. Two buckets
means two access keys, never shared, and the archivist's bucket and its master key are
reached by two.
_Avoid_: identity, IAM user, credentials, service account, token

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
The single Postgres superuser that provisions databases and that the archivist archives them
as. No other user holds it. On ClickHouse the same seat is the user `default`, which
provisioning and the archivist use.
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
The backup of one database. One archive restores one database alone.
_Avoid_: dump, backup file, snapshot, copy

**Retention**:
How long objects stay in their bucket. A rule at your provider sets it, never userland,
which deletes nothing it has written. The archivist's bucket may carry no rule at all, so its
archives stay until someone removes them with restic, from a machine whose key may delete.
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

## The archivist

**Archivist**:
The product that keeps every database off this host, in a bucket of its own, as archives
only the master key opens.
_Avoid_: fort, backup service, backup container

**Master key**:
The one secret that encrypts every archive the archivist keeps. It lives in a secret store,
never on the host, and the archivist reads it at each run. Lose it and every archive is waste.
_Avoid_: passphrase, backup key, encryption key, secret

**Secret store**:
A service you own, outside the host, that holds the master key. userland reads it and
never writes it. An external dependency.
_Avoid_: vault, parameter store (as the category), key store, KMS

**Restore**:
Writing an archive back into the datastore it came from. Deliberate, by hand, never on a
schedule.
_Avoid_: recover, pull, sync
