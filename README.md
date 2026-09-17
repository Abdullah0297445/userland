# userland

A self-hosted platform you switch services on and off in. Clone it, run one command,
answer what it asks, and a host ends up running a reverse proxy, a set of shared
datastores, and whichever applications you enabled — each behind TLS, each with its
database already provisioned and already being backed up.

There is no application code here. userland is the ground your own applications stand on,
and it is deliberately not one of them.

> **This repo is being built in the open.** Nothing here runs yet — the compose files, the
> CLI and the per-service documentation arrive service by service. The design is published
> as issues on this repo as it is settled.

`userland` is the part of a running system that is not the kernel: everything the machine
runs *for you*. This repo is that layer, for one host.

## What you can switch on

| | |
|---|---|
| **The proxy** | traefik, terminating TLS for everything else. |
| **The datastores** | One Postgres, one Redis, one ClickHouse, one S3-compatible store. Shared — one of each for the whole host, never one per application. |
| **The applications** | n8n, Metabase, Langfuse, Twenty, neo4j. |

Each application holds its own database on the shared Postgres. The backup discovers
databases by reading the server rather than by being handed a list, so a service is backed
up from the day you switch it on.

## The shape

- **One compose project.** A root `compose.yml` that does nothing but `include:` one
  fragment per service, out of `compose/`.
- **One `.env`.** Every choice you make lands in it, and it is the only file you own.
- **`profiles:` decide what runs.** The CLI writes `COMPOSE_PROFILES`; you never hand-edit
  it.
- **Upstream, not a template.** You never edit a tracked file — so `git pull` keeps
  working, and it is how the next service reaches you.

## Two modes

**local** — plain HTTP on `*.localhost`. No DNS record, no certificate, no domain to buy.
This is how you find out whether you want it.

**public** — real hostnames, with TLS issued over a DNS-01 challenge.

They are one variable apart.

## Layout

| Path | What lives there |
|---|---|
| `compose/` | One compose fragment per service. Nothing is ever run from inside it. |
| `scripts/` | Bash that a container or an operator runs — provisioning, backups. |
| `initdb/` | First-start initialisation for a datastore. Runs once, against an empty volume, and never again. |
| `config/` | Configuration files a container mounts, checked in because they hold nothing secret. |
| `templates/` | Files you copy into a project of your own. userland never runs them. |

Support directories are grouped **by kind, at the root** — `scripts/`, never
`metabase/scripts/`. One service's files are spread across several of them on purpose: the
thing you switch on and off is a profile, not a folder.

## Notes

- **Every concrete value stays out of git.** Your domain, your cloud account, your bucket
  name and your host address all live in `.env`, which is gitignored, so this repo can be
  published without redaction. Never write a real domain, bucket name, host address, email
  or account identifier into a tracked file.
- **That one `.env` holds every secret of every service you enabled.** Confirm
  `.gitignore` excludes it before your first commit, and keep a copy somewhere off this
  machine. Some of what it holds — encryption keys a service writes data with — cannot be
  regenerated, and losing them loses the data.

See [CONTEXT.md](CONTEXT.md) for the language this repo uses.
