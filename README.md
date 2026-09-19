# userland

One host, one compose project, and containers you switch on and off. Clone it, run one
command, answer what it asks, and the host ends up running a reverse proxy, a set of
shared datastores, and whichever applications you switched on — each behind TLS, each
with its database already provisioned and already being backed up.

There is no application code here. userland is the ground your own projects stand on,
and it is deliberately not one of them.

> **This repo is being built in the open.** Nothing here runs yet — the templates, the
> CLI and the documentation arrive one container at a time. The design is published
> as issues on this repo as it is settled.

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

Each application is a Postgres tenant — its own role and database on the shared Postgres.
The backup discovers databases by reading the server rather than by being handed a list, so
a tenant is backed up from the day it exists.

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

## Layout

| Path | What lives there |
|---|---|
| `bootstrap` | Builds the CLI inside docker and runs it. The one command; docker is all it needs. |
| `manifest.json` | What every container depends on and what it needs asked. The CLI trusts nothing else. |
| `compose/` | One template per product. The CLI renders `compose.yml` from them; nothing is ever run from inside it. |
| `scripts/` | Shell that runs inside a container — the backups' schedules, the dump, fort's run. Nothing here runs on the host. |
| `initdb/` | First-start initialisation for a datastore. Runs once, against an empty volume, and never again. |
| `config/` | Configuration files a container mounts, checked in because they hold nothing secret. |
| `consumer/` | Files you copy into a project of your own. userland never runs them. |
| `main.go`, `internal/` | The CLI, in Go. |

`compose.yml`, `.env` and the `userland` binary are yours and untracked. Support
directories are grouped **by kind, at the root** — `scripts/`, never `metabase/scripts/`.
One container's files are spread across several of them on purpose: the thing you switch
on and off is a container, not a folder.

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

See [CONTEXT.md](CONTEXT.md) for the language this repo uses.
