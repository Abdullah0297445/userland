# Variables

Everything `.env` may hold, by container. Generated from `manifest.json` and the templates by `./bootstrap check --write`; `check` fails when this file is stale, so it always matches the checked-out commit.

An asked variable is collected by the interview when its container is switched on and the answer is not already in `.env`. An optional variable is neither asked nor written: add the line to `.env` by hand, and the next apply picks it up. `.env` is the only file you own.

## userland

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `VISIBILITY` | written by the CLI | always |  | `local` or `public`, chosen once for the whole of userland. |
| `USERLAND_ON` | written by the CLI | always |  | The selection: every container that is switched on, comma-separated. |
| `USERLAND_OFF` | written by the CLI | always |  | Every container that was asked about and is off, so a re-run does not ask again. |

## clickhouse

### clickhouse

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `CLICKHOUSE_PASSWORD` | generated | always |  | Password of the ClickHouse admin user, default |
| `CLICKHOUSE_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |

## fort

### fort

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `FORT_KEY_PROVIDER` | secret-store | always |  | Secret store the master key lives in; `ssm` is AWS Parameter Store, the one there is. |
| `FORT_KEY_NAME` | secret-store | always |  | Name of the SecureString parameter that holds the master key. Enter to generate `/userland/fort-key-<8 hex>`, or type one you made. |
| `FORT_KEY_REGION` | secret-store | always |  | Region of the parameter. |
| `FORT_KEY_ACCESS_KEY_ID` | secret-store | always |  | Access key that may read this one parameter and nothing else. |
| `FORT_KEY_SECRET_ACCESS_KEY` | secret-store | always |  | Its secret. |
| `FORT_S3_BUCKET` | bucket, versioned, delete under locks/, never expire | always |  | Name of the bucket. Enter to generate `userland-fort-s3-<8 hex>`, or type one you made. |
| `FORT_S3_REGION` | bucket, versioned, delete under locks/, never expire | always |  | Region of the bucket, as the provider names it; `auto` on Cloudflare R2. |
| `FORT_S3_ENDPOINT` | bucket, versioned, delete under locks/, never expire | always |  | URL the bucket is reached at. The offer writes `https://s3.<region>.amazonaws.com`. |
| `FORT_S3_ACCESS_KEY_ID` | bucket, versioned, delete under locks/, never expire | always |  | Access key that reaches this bucket and nothing else. It may list the bucket and get and put objects, and delete under `locks/` and nowhere else. |
| `FORT_S3_SECRET_ACCESS_KEY` | bucket, versioned, delete under locks/, never expire | always |  | Its secret. |
| `FORT_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |
| `FORT_FILES` | optional |  |  | Absolute paths this container keeps, separated by colons, each one's directory mounted read-only under `/files`. Written by the CLI: `./bootstrap fort add PATH` and `remove PATH` change it. |
| `FORT_SCHEDULE` | optional |  |  | Read by the template; defaults to `@daily`. |

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

### pgadmin

| Variable | Kind | When | Keep | Meaning |
|---|---|---|---|---|
| `PGADMIN_DEFAULT_EMAIL` | email | always |  | Email address you sign in to pgadmin with |
| `PGADMIN_DEFAULT_PASSWORD` | generated | always |  | Password you sign in to pgadmin with |
| `PGADMIN_PORT` | optional |  |  | Loopback port to publish on while traefik is on; nothing is published unless it is set. With traefik off the container publishes on `127.0.0.1:5050` regardless. |
| `PGADMIN_MEM_LIMIT` | optional |  |  | Memory limit in compose's units, such as `2g`. Unbounded unless set. |

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

