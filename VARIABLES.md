# Variables

Everything `.env` may hold, by container. Generated from `manifest.json` and the templates by `./bootstrap check --write`; `check` fails when this file is stale, so it always matches the checked-out commit.

An asked variable is collected by the interview when its container is switched on and the answer is not already in `.env`. An optional variable is neither asked nor written: add the line to `.env` by hand, and the next apply picks it up. `.env` is the only file you own.

## userland

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `VISIBILITY` | written by the CLI | always |  | `local` or `public`, chosen once for the whole of userland. |
| `USERLAND_ON` | written by the CLI | always |  | The selection: every container that is switched on, comma-separated. |
| `USERLAND_OFF` | written by the CLI | always |  | Every container that was asked about and is off, so a re-run does not ask again. |

## metabase

### metabase

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `MB_ENCRYPTION_SECRET_KEY` | generated | always | yes | Key metabase encrypts saved data-source credentials with |
| `METABASE_DB_PASSWORD` | generated | always |  | Password of the `metabase` user on Postgres, which owns the `metabase` database. Provisioning converges it to whatever this holds. |
| `METABASE_PORT` | optional |  |  | Loopback port to publish on while traefik is on; nothing is published unless it is set. With traefik off the container publishes on `127.0.0.1:3000` regardless. |
| `METABASE_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |
| `MB_AGGREGATED_QUERY_ROW_LIMIT` | optional |  |  | Read by the template; defaults to `10000`. |
| `MB_UNAGGREGATED_QUERY_ROW_LIMIT` | optional |  |  | Read by the template; defaults to `2000`. |

## n8n

### n8n

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `N8N_ENCRYPTION_KEY` | generated | always | yes | Key n8n encrypts saved credentials with |
| `N8N_DB_PASSWORD` | generated | always |  | Password of the `n8n` user on Postgres, which owns the `n8n` database. Provisioning converges it to whatever this holds. |
| `N8N_PORT` | optional |  |  | Loopback port to publish on while traefik is on; nothing is published unless it is set. With traefik off the container publishes on `127.0.0.1:5678` regardless. |
| `N8N_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |
| `GENERIC_TIMEZONE` | optional |  |  | Read by the template; defaults to `UTC`. |

### n8n-runners

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `N8N_RUNNERS_AUTH_TOKEN` | generated | always |  | Shared secret between n8n and its runners |
| `N8N_RUNNERS_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |
| `GENERIC_TIMEZONE` | optional |  |  | Read by the template; defaults to `UTC`. |

## postgres

### pgbouncer-session

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `PGBOUNCER_SESSION_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |

### pgbouncer-transaction

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `PGBOUNCER_TRANSACTION_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |

### postgres-18

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `POSTGRES_PASSWORD` | generated | always |  | Password of the Postgres superuser |
| `PGBOUNCER_AUTH_PASSWORD` | generated | always |  | Password of the user the doors look passwords up with |
| `POSTGRES_18_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |

## traefik

### traefik

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `DOMAIN` | hostname | public |  | Domain every hostname sits under |
| `CERT_EMAIL` | email | public |  | Email for the ACME account |
| `DNS_PROVIDER` | choice: route53, cloudflare | public |  | DNS-01 provider |
| `AWS_ACCESS_KEY_ID` | secret | DNS_PROVIDER=route53 |  | Access key that may write TXT records in the zone |
| `AWS_SECRET_ACCESS_KEY` | secret | DNS_PROVIDER=route53 |  | Its secret |
| `CF_DNS_API_TOKEN` | secret | DNS_PROVIDER=cloudflare |  | Cloudflare token that may edit DNS in the zone |
| `TRAEFIK_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |
| `AWS_REGION` | optional |  |  | Read by the template; defaults to `us-east-1`. |

