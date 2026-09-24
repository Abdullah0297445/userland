# How Infisical is self-hosted

Research for [#34](https://github.com/Abdullah0297445/userland/issues/34), a child of the map
[#32](https://github.com/Abdullah0297445/userland/issues/32). It was checked on 2026-09-24 against
Infisical `v0.165.16` and the Infisical CLI `0.43.136`, the newest releases that day.

## The short answers

- **Door: the transaction door works.** Infisical takes no session-level advisory lock, sends no `LISTEN`
  and uses no named prepared statement. All 549 migrations and a full read, write and restore ran
  through pgbouncer in transaction pooling, on Postgres 18. The session door is not needed.
- **Only `ENCRYPTION_KEY` loses data if lost.** It unlocks every secret. Everything else can be
  made fresh on a new host: `AUTH_SECRET`, the database password, the Redis password and all of
  Redis. A fresh `AUTH_SECRET` costs only logins: every token issued before it stops working.
- **The first admin needs no browser.** One call, `POST /api/v1/admin/bootstrap`, or
  `infisical bootstrap` from the CLI image, makes the admin user, the organization and an admin
  machine identity. It closes sign-up by itself. It works once only.
- **A script needs only docker.** The `infisical/cli` image logs in as a machine identity with a
  client ID and a client secret, then runs `export` to write a `.env` and `secrets set` to push
  one. It talks to `http://infisical:8080` on a compose network. No port is published.
- **Postgres holds all of Infisical's lasting state.** Redis holds queues, locks and caches,
  which Infisical rebuilds. A restore needs the database dump, the `ENCRYPTION_KEY` and an image at
  the same version. It was restored that way into a fresh stack, and it worked.

## Words used here

- **Machine identity**: an account for a program, not a person. It belongs to an organization and
  is added to projects with a role.
- **Universal auth**: a way for a machine identity to log in with a *client ID* and a *client
  secret*. The login returns an *access token*.
- **Token auth**: a machine identity that is handed an access token directly, with no login step.
  The admin identity that bootstrap makes uses it.
- **JWT**: a signed token. Infisical signs its access tokens and login sessions with
  `AUTH_SECRET`.
- **Bootstrap**: Infisical's name for the first start: making the first admin and organization.
- **Migration**: a change to the database's tables that Infisical applies at start.
- **Advisory lock**: a lock a program takes in Postgres by number. A *session-level* lock lasts
  as long as the connection. A *transaction-level* lock ends with the transaction.
- **Prepared statement**: a query sent once and run again later by name. A *named* one lives on
  the connection. An *unnamed* one lasts for one query only.
- **`LISTEN`**: a Postgres command that makes a connection wait for messages. It needs the same
  connection to stay put.
- **Environment**: Infisical's word for one set of secrets inside a project, such as `dev` or
  `prod`. It is not userland's *visibility*.

## How this was checked

Four kinds of primary source, cited at each finding:

1. Infisical's source at the tag `v0.165.16`
   ([Infisical/infisical](https://github.com/Infisical/infisical/tree/v0.165.16)), and the CLI's
   source at `v0.43.136` ([Infisical/cli](https://github.com/Infisical/cli/tree/v0.43.136)). Each
   file cited below is the same at the tag as on `main` that day, but for one line in
   `keystore.ts`.
2. Infisical's documentation. It is built from the `docs/` folder of the same repository.
3. The images on Docker Hub and the release pages on GitHub.
4. **A drill**: a throwaway compose project on one machine, then torn down. It ran these images:
   `infisical/infisical:v0.165.16`, `infisical/cli:0.43.136`, `postgres:18-alpine` (18.6),
   `edoburu/pgbouncer:v1.25.2-p0` with `POOL_MODE: transaction`, and `redis:7-alpine` started with
   `--requirepass`, `--maxmemory-policy noeviction` and `--appendonly yes`. Infisical reached
   Postgres only through pgbouncer. Postgres ran with `log_statement=all`, so every statement
   Infisical sent was logged. Findings marked **(drill)** come from it.

Where a doc and the source disagree, this note trusts the source. The list is in
[Where the docs and the source disagree](#where-the-docs-and-the-source-disagree).

## 1. The images, their pins, and the containers they need

### The Infisical image

- One image, [`infisical/infisical`](https://hub.docker.com/r/infisical/infisical/tags), holds
  both the API and the web UI. It listens on port 8080, on every interface. It runs as uid 1001.
  ([Dockerfile](https://github.com/Infisical/infisical/blob/v0.165.16/Dockerfile.standalone-infisical))
- **Tags.** Each release is pushed three ways, with one digest: `v0.165.16`, the short commit
  `0b52a60`, and `latest`. Each tag holds `amd64` and `arm64`.
  ([Docker Hub](https://hub.docker.com/r/infisical/infisical/tags),
  [release v0.165.16](https://github.com/Infisical/infisical/releases/tag/v0.165.16))
- **Pin by the `v` tag**, or by digest. Infisical's own compose file uses `latest`, with the note
  "PIN THIS TO A SPECIFIC TAG".
  ([docker-compose.prod.yml](https://github.com/Infisical/infisical/blob/v0.165.16/docker-compose.prod.yml))
- **Releases are frequent.** The docs say about once a week. Docker Hub shows twelve releases
  between 2026-09-04 and 2026-09-23. A release that needs care carries a note on its release
  page. ([Upgrade guide](https://infisical.com/docs/self-hosting/guides/upgrading-infisical))
- The image is large: 2.8 GB unpacked on `arm64` **(drill)**. It carries `curl`, `wget` and a copy
  of the Infisical CLI. ([Dockerfile](https://github.com/Infisical/infisical/blob/v0.165.16/Dockerfile.standalone-infisical))
- It needs **no volume**. The docs say all lasting data is in the database. They also say logs and
  metrics are kept on disk. In the drill the logs went to stdout, and nothing asked for a volume.
  ([Hardware requirements](https://infisical.com/docs/self-hosting/configuration/requirements#storage))
- A FIPS build, `infisical/infisical-fips`, also exists. userland does not need it.

### The CLI image

- [`infisical/cli`](https://hub.docker.com/r/infisical/cli/tags) is built from a separate
  repository, [Infisical/cli](https://github.com/Infisical/cli). Its tags have **no `v`**:
  `0.43.136`, `latest`, and one per architecture such as `0.43.136-arm64`. The GitHub release is
  `v0.43.136`. ([.goreleaser.yaml](https://github.com/Infisical/cli/blob/v0.43.136/.goreleaser.yaml#L327-L357))
- It is Alpine, `tini` and the `infisical` binary. The entrypoint is `infisical`, so
  `docker run infisical/cli:0.43.136 export …` runs `infisical export …`. It is about 233 MB.
  ([docker/alpine](https://github.com/Infisical/cli/blob/v0.43.136/docker/alpine))

### Postgres

- Postgres is the only database Infisical supports. The docs say it is tested with Postgres 16
  and recommend 14 or newer. ([Hardware requirements](https://infisical.com/docs/self-hosting/configuration/requirements))
- **Postgres 18 worked (drill).** Migrations, reads, writes, a dump and a restore all ran on 18.6.
  Infisical does not claim support for 18, so this rests on the drill alone.
- The user must own its database. The docs ask for "all privileges on the Infisical database".
  ([Environment variables](https://infisical.com/docs/self-hosting/configuration/envars#postgresql))
- It needs **no extension and no extra schema**. No migration runs `CREATE EXTENSION` or
  `CREATE SCHEMA`. The one optional schema, the "sanitized schema", is off by default
  (`GENERATE_SANITIZED_SCHEMA=false`). In the drill, a database owned by a plain user was enough.
  ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L261-L263),
  [migrations folder](https://github.com/Infisical/infisical/tree/v0.165.16/backend/src/db/migrations))
- Each Infisical container keeps its own pool of up to 10 connections (`DB_POOL_MAX`).
  ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L279))

### Redis

- **Redis is required.** Infisical refuses to start without `REDIS_URL` (or a Sentinel or Cluster
  setting). ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L687-L690))
- The docs ask for Redis 6.x or 7.x, with the eviction policy `noeviction`. That is required,
  because queued jobs live in Redis and an evicted key is a lost job. They also recommend
  persistence (AOF or RDB). ([Hardware requirements](https://infisical.com/docs/self-hosting/configuration/requirements#redis))
- That is the same shape as `langfuse-redis` today: `redis:7-alpine` with `--requirepass`,
  `--maxmemory-policy noeviction` and `--appendonly yes`. It worked in the drill.
- ClickHouse is optional, for audit logs only. userland does not need it.
  ([Environment variables](https://infisical.com/docs/self-hosting/configuration/envars#clickhouse-optional))

## 2. Which door: transaction or session

**The transaction door works. Use it.** Here is why, point by point.

- **Migrations.** Migrations run by themselves at every start, before the server listens. They use
  knex. The lock that stops two containers migrating at once is **a row in a table**, not a
  session-level advisory lock. There are two tables: `infisical_migrations_lock` (knex's own) and
  `infisical_migrations_startup_lock`, which carries a heartbeat. The source says it moved off
  advisory locks on purpose, because `CREATE INDEX CONCURRENTLY` cannot run inside a transaction.
  The only advisory lock left there is `pg_advisory_xact_lock`, which is transaction-level.
  ([auto-start-migrations.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/auto-start-migrations.ts#L216-L397),
  [main.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/main.ts#L85-L90))
- **Advisory locks at run time.** Every one is `pg_advisory_xact_lock`, taken inside a
  transaction. A transaction-level lock ends with the transaction, so the transaction door keeps it
  whole. There is no `pg_advisory_lock` or `pg_try_advisory_lock` anywhere in the backend.
  ([keystore.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/keystore/keystore.ts),
  for example [kms-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/kms/kms-service.ts#L385-L391))
- **`LISTEN` and `NOTIFY`.** None. Background jobs go through Redis, with BullMQ, not through
  Postgres. An older Postgres job queue (pg-boss) was removed; a migration moves its leftover jobs.
  ([queue-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/queue/queue-service.ts),
  [20260203141935_durable-queue.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/db/migrations/20260203141935_durable-queue.ts))
- **Prepared statements.** Infisical uses knex over node-postgres. It never names a statement, so
  every query is an unnamed prepared statement, which the transaction door handles.
  ([instance.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/db/instance.ts#L76-L93))
- **`SET`.** At run time Infisical uses only `SET LOCAL`, which ends with its transaction. But
  some migrations build an index with `CREATE INDEX CONCURRENTLY` outside a transaction. They send
  `SET statement_timeout` and `SET lock_timeout` first, and set them back after. Through the
  transaction door, each of those statements may go over a different server connection. So the
  timeout may not reach the index build, and a changed timeout may stay behind on one of
  Infisical's own pooled connections. The cost is small. At worst, a 30-second lock timeout or a
  statement timeout of up to an hour lingers on one connection of Infisical's own pool, until the
  next such `SET` lands on it. The index builds still succeed.
  ([an example migration](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/db/migrations/20260602100200_add-projects-soft-deleted-index.ts))

**What Postgres saw in the drill**, counted from its log of every statement:

| What | Count |
|---|---|
| `LISTEN` | 0 |
| `pg_advisory_lock` or `pg_try_advisory_lock` (session-level) | 0 |
| `pg_advisory_xact_lock` (transaction-level) | 5 |
| `PREPARE`, or a named prepared statement | 0 |
| Unnamed prepared statements | 1,905 |
| `SET statement_timeout` / `SET lock_timeout` (migrations) | 75 / 52 |
| `SET LOCAL` | 4 |
| Errors | 0 |

All 549 migrations ran through the transaction door in about six seconds. Postgres logged no
error, and Infisical logged none about its database. Reads, writes, logins and a restore then
worked through the same door.

Two more notes:

- The docs call a connection pooler optional. They warn that a pooler in transaction pooling
  "doesn't reclaim connections held open inside a transaction". That is about sizing, not about
  whether it works. ([Environment variables](https://infisical.com/docs/self-hosting/configuration/envars#connection-pool-sizing-optional))
- The image also carries a Go "sidecar" that uses pgx. It is off by default
  (`GO_SIDECAR_SPAWN_ENABLED=false`) and was not examined. If it is ever turned on, check the door
  again. ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L677-L682))

## 3. Every setting it needs

"Made fresh" means a new host can generate a new value and nothing is lost. The source of truth is
the settings schema in [env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts).

| Setting | Format | Made fresh on a new host? | If it is lost |
|---|---|---|---|
| `ENCRYPTION_KEY` | Exactly 32 characters. `openssl rand -hex 16` makes one. Infisical uses the characters themselves as the 32-byte key. ([kms-fns.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/kms/kms-fns.ts#L31-L53)) | **No.** | **Every secret is lost.** It encrypts the root key that encrypts all the rest. A wrong key stops the container at start (see section 8). |
| `AUTH_SECRET` | Any string. The docs ask for `openssl rand -base64 32`. ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L388)) | **Yes.** | No data is lost. It signs tokens, and nothing stored is encrypted with it. Every token signed with the old one stops working: browser sessions, machine-identity access tokens, and the bootstrap admin token. A machine identity logs in again with its client secret and carries on **(drill)**. |
| `DB_CONNECTION_URI` | `postgres://infisical:PASSWORD@pgbouncer-transaction:5432/infisical`. A password with URL-special characters must be percent-encoded; a hex password needs nothing. | **Yes.** It is whatever password provisioning gives the user. | Nothing. |
| `REDIS_URL` | `redis://:PASSWORD@infisical-redis:6379`, where the container name is an example. The password goes in the URL: `REDIS_PASSWORD` is ignored when `REDIS_URL` is set. ([redis.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/redis.ts#L90-L105)) | **Yes**, with all of Redis. | Nothing userland uses (see section 8). |
| `SITE_URL` | An absolute URL with its scheme, such as `http://infisical.localhost` or `https://infisical.example.com`. A trailing slash is stripped. ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L365-L366)) | **Yes.** It follows from visibility. | Nothing. |
| `HTTPS_ENABLED` | `true`, or unset. Unset means `false`. ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L315)) | **Yes.** `true` in public, unset in local. | Nothing. |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM_ADDRESS`, `SMTP_FROM_NAME` | Optional. Port defaults to 587. ([Environment variables](https://infisical.com/docs/self-hosting/configuration/envars#email-service)) | From your mail provider. | Nothing stored. Without SMTP there are no email invites, no email MFA and no password reset by email. |
| `TELEMETRY_ENABLED`, `DISABLE_UPDATE_CHECK`, `ANNOUNCEMENTS_ENABLED` | `false`, `true`, `false` stop Infisical's calls home. The image sets `TELEMETRY_ENABLED true`. ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L174-L377), [Dockerfile](https://github.com/Infisical/infisical/blob/v0.165.16/Dockerfile.standalone-infisical)) | Yes. | Nothing. |
| `TRUSTED_PROXY_CIDRS` | Optional. A comma-separated list of the address ranges the proxy connects from. ([Environment variables](https://infisical.com/docs/self-hosting/configuration/envars#param-trusted-proxy-cidrs)) | Yes. | Nothing. |
| `MAX_MACHINE_IDENTITY_TOKEN_AGE` | Optional. A duration such as `90d`, the default. It caps every machine-identity access token. It is in the source but not in the docs. ([env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L395-L400)) | Yes. | Nothing. |
| `COOKIE_SECRET_SIGN_KEY` | Leave it unset. Infisical then derives it from the root key. It signs only single sign-on logins. ([Environment variables](https://infisical.com/docs/self-hosting/configuration/envars#param-cookie-secret-sign-key)) | Yes. | Nothing. |
| `LICENSE_KEY` | Not needed for the free edition. | — | — |

Two more things behave like settings, although Infisical does not read them from its environment:

- **The admin email and password**, given once to bootstrap. The password is stored only as a
  hash. It is needed for the web UI and for nothing a script does. If it is lost, the docs offer
  the "Forgot Password" email, which needs SMTP.
  ([Docker Compose guide, "Reset admin password"](https://infisical.com/docs/self-hosting/deployment-options/docker-compose))
- **A machine identity's client ID and client secret.** They are made after bootstrap. The client
  secret is shown once and stored only as a hash, in Postgres. So it survives a restore, and it
  survives a fresh `AUTH_SECRET` **(drill)**. By default it never expires.
  ([identity-ua-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/identity-ua/identity-ua-service.ts#L981-L992),
  [identity-universal-auth-router.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/v1/identity-universal-auth-router.ts#L560-L567))

**What this means for a new host.** Only `ENCRYPTION_KEY` must be kept off the host. The rest of
Infisical's own settings can be generated again. But a script on the new host also needs some way
to log in once Infisical is back, such as a machine identity's client ID and secret. Where that
lives is for "Every .env lives in Infisical, and a new host gets them back" to decide.

## 4. Behind traefik, on a subdomain

- **One port, at the root path.** The container serves the UI and the API on 8080. A subdomain
  such as `infisical.localhost` or `infisical.example.com` needs no path prefix.
- **`SITE_URL`** is the address people use. Infisical uses it for links in emails, for the allowed
  origin of browser requests, and for one cookie's domain.
  ([app.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/app.ts#L116-L128),
  [cookie.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/lib/cookie.ts))
- **A script need not use `SITE_URL`.** In the drill `SITE_URL` was `http://infisical.localhost`,
  and the CLI talked to `http://infisical:8080` over the compose network. Nothing checked the host
  name. So a helper can reach Infisical without going through traefik.
- **Local visibility, plain HTTP.** Leave `HTTPS_ENABLED` unset. Login cookies are then sent
  without the `Secure` flag, so the browser keeps them over HTTP.
- **Public visibility, HTTPS.** Set `HTTPS_ENABLED=true`, so login cookies are marked `Secure`.
  The reverse is a known trap: with `HTTPS_ENABLED=true` over plain HTTP, the browser drops the
  cookies, and every page refresh logs you out.
  ([FAQ](https://infisical.com/docs/self-hosting/faq), and the cookie flags in
  [auth-router.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/v1/auth-router.ts#L36-L60))
- **Forwarded headers.** By default Infisical trusts `X-Forwarded-For` from anyone. Its rate
  limits and IP checks use that address. `TRUSTED_PROXY_CIDRS` limits the trust to the proxy's
  addresses. The docs advise setting it when Infisical sits behind a proxy.
  ([app.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/app.ts#L84-L86),
  [Environment variables](https://infisical.com/docs/self-hosting/configuration/envars#param-trusted-proxy-cidrs))
- **Not tested with traefik itself.** The drill used no proxy. Nothing found in the source needs
  more than a plain HTTP router, and websockets pass through traefik by default.

### The health endpoint

- `GET /api/status` needs no login and is exempt from rate limits. It answers 200 with
  `{"message":"Ok", …}`. It reads the server's settings on each call. Infisical's own Helm chart
  uses it as the readiness check.
  ([routes/index.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/index.ts#L4485-L4535),
  [Helm chart](https://github.com/Infisical/infisical/blob/v0.165.16/helm-charts/infisical-standalone-postgres/templates/infisical.yaml#L63-L68))
- The image has `wget`, so this healthcheck works **(drill)**:
  `["CMD", "wget", "-q", "-O", "/dev/null", "http://localhost:8080/api/status"]`.
- **Log noise.** Each call writes two INFO lines of "event loop stats" to the log. A check every
  10 seconds adds about 17,000 lines a day **(drill)**.
- **First start.** On an empty database the container was healthy in well under a minute. Most of
  that was the 549 migrations, which took about six seconds **(drill)**.
- Without SMTP, the log shows one harmless error at start: a failed connection to
  `127.0.0.1:587` **(drill)**.

## 5. The first start, without a browser

Infisical calls this **bootstrap**.
([Programmatic provisioning](https://infisical.com/docs/self-hosting/guides/automated-bootstrapping))

**By the API:**

```sh
curl -X POST -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"…","organization":"userland"}' \
  http://infisical:8080/api/v1/admin/bootstrap
```

**By the CLI image**, which takes the same three values as flags or as the variables
`INFISICAL_ADMIN_EMAIL`, `INFISICAL_ADMIN_PASSWORD` and `INFISICAL_ADMIN_ORGANIZATION`:

```sh
docker run --rm --network NETWORK \
  -e INFISICAL_ADMIN_EMAIL -e INFISICAL_ADMIN_PASSWORD -e INFISICAL_ADMIN_ORGANIZATION \
  infisical/cli:0.43.136 bootstrap --domain http://infisical:8080 --silent
```

([bootstrap.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/bootstrap.go))

**What it makes**, in one call
([super-admin-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/super-admin/super-admin-service.ts#L594-L720)):

- a server admin user, who logs in by email and password;
- an organization;
- a machine identity called "Instance Admin Identity", an admin of the organization and of the
  whole instance, using token auth;
- one access token for that identity, returned in the answer as `identity.credentials.token`.

**What to know about it:**

- **It closes sign-up.** On a self-hosted instance it sets `allowSignUp` to false. In the drill,
  `GET /api/v1/admin/config` showed `"allowSignUp": false` right after
  ([super-admin-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/super-admin/super-admin-service.ts#L703-L711)).
- **It works once.** A second call answers HTTP 400 with "Instance has already been set up". The
  same happens on a restored database **(drill)**.
- **The CLI exits 0 even when bootstrap fails.** It logs the error and returns, with or without
  `--ignore-if-bootstrapped` **(drill)**. A script must check the output, which is JSON on stdout
  on success, or call the API with `curl` and read the status code.
  ([bootstrap.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/bootstrap.go#L240-L247))
- **The admin token expires after 90 days**, the `MAX_MACHINE_IDENTITY_TOKEN_AGE` ceiling. In the
  drill it carried `exp` exactly 90 days after `iat`. A fresh `AUTH_SECRET` also kills it
  **(drill)**. Bootstrap cannot issue another.
  ([identity-access-token-fns.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/identity-access-token/identity-access-token-fns.ts#L120-L145))
- So **use the admin token once, straight away**, to make a lasting login: a machine identity with
  universal auth. Its client secret does not expire unless you say so.

**The calls after bootstrap**, all tested with the admin token as `Authorization: Bearer …`
**(drill)**:

| Step | Call |
|---|---|
| Make a project | `POST /api/v1/projects` with `{"projectName":"userland","slug":"userland","type":"secret-manager","shouldCreateDefaultEnvs":false}` |
| Make an environment | `POST /api/v1/projects/PROJECT_ID/environments` with `{"name":"Host","slug":"host"}` |
| Make a machine identity | `POST /api/v1/identities` with `{"name":"…","organizationId":"ORG_ID","role":"no-access"}` |
| Give it universal auth | `POST /api/v1/auth/universal-auth/identities/IDENTITY_ID`. The answer holds `identityUniversalAuth.clientId`. |
| Make its client secret | `POST /api/v1/auth/universal-auth/identities/IDENTITY_ID/client-secrets`. The answer holds `clientSecret`, shown once. |
| Add it to the project | `POST /api/v1/projects/PROJECT_ID/memberships/identities/IDENTITY_ID` with `{"role":"member"}` |

Universal auth's defaults: an access token lives 30 days, and the client secret never expires.
Both can be set per identity.
([identity-universal-auth-router.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/v1/identity-universal-auth-router.ts#L150-L200))

## 6. A script, as a machine identity, with only docker

All of this ran in the drill, from `infisical/cli:0.43.136` on the compose network.

### Log in

```sh
TOKEN=$(docker run --rm --network NETWORK \
  -e INFISICAL_UNIVERSAL_AUTH_CLIENT_ID -e INFISICAL_UNIVERSAL_AUTH_CLIENT_SECRET \
  infisical/cli:0.43.136 login --method=universal-auth \
  --domain http://infisical:8080 --silent --plain)
```

- `--plain` prints the access token and nothing else. A machine login keeps no session: it
  prints the token and stops.
  ([login.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/login.go#L456-L460))
- `--domain` may leave off `/api`; the CLI adds it. `INFISICAL_DOMAIN` works in place of the flag.
  ([root.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/root.go#L98-L123),
  [helper.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/helper.go#L483-L489))
- Later commands take the token as `INFISICAL_TOKEN` or `--token`, and the project as
  `--projectId` or `INFISICAL_PROJECT_ID`.
  ([helper.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/helper.go#L287-L329))

### Read: `infisical export`

```sh
docker run --rm --network NETWORK -e INFISICAL_TOKEN="$TOKEN" infisical/cli:0.43.136 \
  export --projectId PROJECT_ID --env host --format dotenv --expand=false \
  --domain http://infisical:8080 --silent > .env
```

- **Formats** ([export.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/export.go#L280-L400)):
  - `dotenv`: `KEY='value'`, one per line.
  - `dotenv-export`: `export KEY='value'`.
  - `dotenv-eval`: `export KEY='value'`, with single quotes escaped, for `eval` in a shell.
  - `json`: an array of objects, each with `key` and `value`.
  - `csv` and `yaml`.
- Keys come out sorted.
- **Use `--expand=false`.** By default Infisical replaces `${OTHER_KEY}` inside a value with that
  secret's value. In the drill, `${POSTGRES_PASSWORD}-suffix` came out as `abc123def456-suffix`
  by default, and unchanged with `--expand=false`.
- **Compose reads the `dotenv` output literally.** Single quotes stop compose's own `${…}`
  interpolation. `$`, `#`, `=` and spaces all reached the container unchanged **(drill)**.
- **A single quote in a value breaks the file.** `dotenv` does not escape it. Compose then refuses
  the whole `.env`: `unexpected character "'" in variable name` **(drill)**. Hex passwords are
  safe; a hand-written value with `'` is not.
- `--output-file` writes the file with permissions `0644`, readable by anyone on the host.
  Redirect stdout into a file you made `0600` instead.
  ([export.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/export.go#L150-L160))

### Write: `infisical secrets set`

```sh
docker run --rm --network NETWORK -e INFISICAL_TOKEN="$TOKEN" -v "$PWD/.env:/work/.env:ro" \
  infisical/cli:0.43.136 secrets set --file /work/.env \
  --projectId PROJECT_ID --env host --domain http://infisical:8080 --silent
```

- It reads the current secrets first. Then it creates the new keys, updates the changed ones and
  reports the rest as "SECRET VALUE UNCHANGED". Pushing the same file twice changes nothing
  **(drill)**. ([secrets.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/secrets.go#L595-L760))
- **It never deletes.** A key missing from the file stays in Infisical. `secrets delete KEY` removes
  one.
- **An empty value is refused**, both as `KEY=` and in a file. The command exits 1 **(drill)**. So a
  `.env` line with an empty value cannot be stored through the CLI. The API itself accepts an empty
  string.
- **The server trims** spaces at both ends of a value **(drill)**.
  ([v4 secret-router.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/routes/v4/secret-router.ts#L1056-L1060))
- The `--file` parser is simple: one `KEY=VALUE` per line, lines starting `#` or `//` skipped, one
  pair of outer quotes removed. There are no multi-line values. The `dotenv` export reads back in
  unchanged.
  ([secrets.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/secrets.go#L532-L580))
- **`KEY=@/path/to/file` does not work with a token.** The help text offers it, but the CLI reads
  the file only for a logged-in person. With a machine token, the literal text `@/path/to/file` is
  stored **(drill)**. ([secrets.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/secrets.go#L266-L297))
- Key names: the server refuses `:` and `/` and anything empty. The CLI also refuses a key that
  starts with a digit or holds a space.
  ([schemas.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/lib/schemas.ts#L45-L50),
  [secrets.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/util/secrets.go#L582-L593))

### `infisical run`

- `infisical run --projectId P --env host -- COMMAND` starts `COMMAND` with every secret in its
  environment. It worked inside the CLI image **(drill)**.
- It needs the CLI in the same container as the command. The CLI image has no docker in it, so
  `run` cannot wrap `docker compose` on the host. For compose, `export` to `.env` is the fit.
  `run` suits a consumer whose own image carries the CLI.
  ([run.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/run.go#L30-L224))

### Rate limits in the free edition

Per client address, per minute: 60 reads, 200 writes, and 40 calls to the secrets endpoints.
`export` makes one secrets call. `secrets set` makes a read and one or two writes. A helper that
pulls many `.env` files at once could meet the limit of 40. This was not tested.
([license-fns.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/ee/services/license/license-fns.ts#L91-L95),
[inject-rate-limits.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/server/plugins/inject-rate-limits.ts))

## 7. Projects and environments for userland's `.env` files

These are the facts that bound the choice. The choice itself belongs to "Every .env lives in
Infisical, and a new host gets them back".

- **The layers:** organization, then project, then environment, then a folder path, then the
  secret. One `export` reads one environment at one path of one project.
- **The free edition has no limit** on projects, environments or machine identities.
  ([license-fns.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/ee/services/license/license-fns.ts#L53-L64))
- **The only free fence is the project.** Custom roles are paid (`rbac`), and so is access by
  folder (`secretsFolderRbac`). A machine identity in a project sees every folder in it.
  ([role-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/role/role-service.ts#L90-L97),
  [folder-permission-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/folder-permission/folder-permission-service.ts#L86-L93))
- **The free project roles** are `admin`, `member` (read and write), `viewer` (read only) and
  `no-access`. ([models.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/db/schemas/models.ts#L390-L399))
- So, to keep one consumer from reading another's `.env`, each consumer needs **a project of its
  own**, and **a machine identity that is a member of that project only**. userland's own `.env`
  fits the same way, in a project of its own. An identity that writes every project, such as a
  helper's, must be a member of each one, or an organization admin.
- A new project gets `dev`, `staging` and `prod` unless `shouldCreateDefaultEnvs` is false. Other
  environments are made by the API. The drill made one called `host`.
  ([project-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/project/project-service.ts#L129-L133))
- One host has one set of values per `.env`, so one environment per project is enough. Infisical's
  environment is not userland's visibility, and nothing forces the two to line up.

## 8. Where the state lives, and what a restore needs

**Postgres holds everything that lasts.** That is: users, organizations, projects and secrets
(encrypted), machine identities and the hashes of their client secrets, the root key (encrypted
by `ENCRYPTION_KEY`), the server's settings, any setting changed in the web "Server Console"
(encrypted), and audit logs by default.
([Hardware requirements](https://infisical.com/docs/self-hosting/configuration/requirements#storage),
[super-admin-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/super-admin/super-admin-service.ts#L470-L480))

**Redis holds what Infisical rebuilds or can lose.** That is: the background job queues, locks,
caches, rate-limit counts, and short-lived login state such as a pending MFA step.
([keystore.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/keystore/keystore.ts))
The docs call Redis persistent and ask for it in backups. The reason given is pending jobs:
secret rotations, syncs and webhook deliveries.
([Hardware requirements](https://infisical.com/docs/self-hosting/configuration/requirements#redis))
userland uses none of those. In the drill, Redis was deleted, volume and all. After a restart,
every secret was still read back.

**No files.** The container needs no volume. The only files the source writes are temporary ones.

**A restore needs three things:**

1. A dump of Infisical's database.
2. **The `ENCRYPTION_KEY` in use when the dump was taken.**
3. An image at the same version, or a newer one. A newer image applies its pending migrations at
   start. An older image that finds migrations it does not know logs a warning and starts only if
   it has none of its own pending. Otherwise it refuses.
   ([auto-start-migrations.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/auto-start-migrations.ts#L399-L519))

**The restore drill.** The database was dumped with `pg_dump -Fc` from the first stack. It was
restored with `pg_restore --no-owner --role=infisical` into a new stack. The new stack had the
same `ENCRYPTION_KEY`, but a fresh `AUTH_SECRET`, a fresh database password and an empty Redis.
Infisical came up healthy and logged "No migrations pending". The machine identity logged in with
its old client ID and secret, and every secret was read back. Bootstrap refused, because the
instance was already set up.

**A wrong key stops the container.** Started with another `ENCRYPTION_KEY`, Infisical exited with
code 1 and this message **(drill)**:

> The configured encryption key (label 5fdad2b4…) does not decrypt this database's root key. This
> database was last written with encryption key label(s): 6d086d20…. Set the matching key and
> restart.

A label is a fingerprint of a key, not the key. Started again with the right key, it came back
healthy.

**Rotating `ENCRYPTION_KEY`.** Infisical can replace the key, from the Server Console. The new key
takes effect at the first start with it. The old key is kept for `KMS_ROOT_KEY_RETENTION_DAYS`,
7 by default, and then removed. Only one old key is ever kept. **Dumps taken before a rotation
need the old key.** ([Rotating the encryption key](https://infisical.com/docs/self-hosting/guides/rotating-the-encryption-key))

## 9. The licence, and what the free edition lacks

- **Licence.** Everything outside the `ee/` folders is MIT. The `ee/` folders are under the
  Infisical Enterprise licence, whose production use needs a subscription. The image carries both.
  Without a `LICENSE_KEY`, the paid features stay locked, and the free set is served.
  ([LICENSE](https://github.com/Infisical/infisical/blob/v0.165.16/LICENSE),
  [ee/LICENSE.md](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/ee/LICENSE.md),
  [Enterprise licensing](https://infisical.com/docs/self-hosting/ee))
- The CLI repository has the same split. ([LICENSE](https://github.com/Infisical/cli/blob/v0.43.136/LICENSE))
- **Free, and needed here:** projects, environments, folders, secrets, secret versions, machine
  identities, universal auth, token auth, bootstrap, the CLI.
- **Paid, and what its absence means here**
  ([license-fns.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/ee/services/license/license-fns.ts#L53-L140)):
  - Custom roles and access by folder: consumers are kept apart by project, as in section 7.
  - Audit logs: there is no record of who read which secret.
  - IP allow-lists: a machine identity's trusted addresses must stay `0.0.0.0/0` and `::/0`.
    ([identity-ua-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/identity-ua/identity-ua-service.ts#L517-L545))
  - Point-in-time recovery, secret rotation, secret approvals, single sign-on, SCIM, groups,
    dynamic secrets and HSM. userland needs none of them.
  - Higher rate limits (section 6).

## Where the docs and the source disagree

The source wins in each case.

1. **`HTTPS_ENABLED`'s default.** The FAQ says to set it to `"false"` to run without TLS. That reads
   as if it were on by default. In the source it is off unless set to `"true"`, and the image sets
   it to `false`. ([FAQ](https://infisical.com/docs/self-hosting/faq),
   [env.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/lib/config/env.ts#L315),
   [Dockerfile](https://github.com/Infisical/infisical/blob/v0.165.16/Dockerfile.standalone-infisical))
2. **The bootstrap token's life.** The docs show a sample token with no expiry. The source gives it
   one, 90 days by default, and the drill confirmed it.
   ([Programmatic provisioning](https://infisical.com/docs/self-hosting/guides/automated-bootstrapping),
   [super-admin-service.ts](https://github.com/Infisical/infisical/blob/v0.165.16/backend/src/services/super-admin/super-admin-service.ts#L684-L699))
3. **`secrets set KEY=@file`.** The CLI's help offers it. The source reads the file only for a
   logged-in person, not with a machine token, and the drill confirmed it.
4. **`SITE_URL`.** The docs mark it required. The source makes it optional. Set it anyway: links in
   emails and the browser's allowed origin come from it.
5. **Redis.** One page calls Infisical stateless, with all data in the database. The same page
   calls Redis a persistent store to back up. Both hold. Redis keeps only pending work and
   short-lived state, and the drill read every secret back after Redis was wiped.
   ([Hardware requirements](https://infisical.com/docs/self-hosting/configuration/requirements))

## Not confirmed

- **traefik.** The drill used no proxy. The UI was not tried through one, in either visibility.
- **Postgres 18.** It worked in the drill. Infisical says it tests with 16.
- **The rate limits.** They are read from the source, not tested.
- **The CLI's telemetry.** The CLI sends usage events by default. It reads its `--telemetry` flag
  before it parses the command line, so `--telemetry=false` may have no effect. Not tested.
  ([root.go](https://github.com/Infisical/cli/blob/v0.43.136/packages/cmd/root.go#L152-L216))
- **`secrets set --file /dev/stdin`**, which would push a `.env` without mounting it. Not tried.
- **A lost admin password with no SMTP.** Whether the admin machine identity can reset it is not
  known.
- **The leftover `SET` after a migration** (section 2). The effect was reasoned from the source. The
  drill could not show it, because only one client was active.
- **The Go sidecar**, off by default, was not examined.

## Sources

- Infisical source, tag `v0.165.16`: <https://github.com/Infisical/infisical/tree/v0.165.16>
- Infisical CLI source, tag `v0.43.136`: <https://github.com/Infisical/cli/tree/v0.43.136>
- Releases: <https://github.com/Infisical/infisical/releases/tag/v0.165.16>,
  <https://github.com/Infisical/cli/releases/tag/v0.43.136>
- Images: <https://hub.docker.com/r/infisical/infisical/tags>,
  <https://hub.docker.com/r/infisical/cli/tags>
- Docs:
  - [Docker Compose](https://infisical.com/docs/self-hosting/deployment-options/docker-compose)
  - [Standalone container](https://infisical.com/docs/self-hosting/deployment-options/standalone-infisical)
  - [Hardware requirements](https://infisical.com/docs/self-hosting/configuration/requirements)
  - [Environment variables](https://infisical.com/docs/self-hosting/configuration/envars)
  - [Programmatic provisioning](https://infisical.com/docs/self-hosting/guides/automated-bootstrapping)
  - [Rotating the encryption key](https://infisical.com/docs/self-hosting/guides/rotating-the-encryption-key)
  - [Upgrade guide](https://infisical.com/docs/self-hosting/guides/upgrading-infisical)
  - [FAQ](https://infisical.com/docs/self-hosting/faq)
  - [Enterprise licensing](https://infisical.com/docs/self-hosting/ee)
- The drill, described under [How this was checked](#how-this-was-checked).
