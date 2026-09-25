#!/bin/sh
set -eu

if [ -z "${PGBOUNCER_AUTH_USER:-}" ] || [ -z "${PGBOUNCER_AUTH_PASSWORD:-}" ]; then
	echo "door-auth: PGBOUNCER_AUTH_USER and PGBOUNCER_AUTH_PASSWORD must both be set" >&2
	exit 1
fi

psql -v ON_ERROR_STOP=1 --no-psqlrc --username "$POSTGRES_USER" --dbname postgres \
	-v authuser="$PGBOUNCER_AUTH_USER" -v authpw="$PGBOUNCER_AUTH_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN', :'authuser')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'authuser')
\gexec

ALTER ROLE :"authuser"
  WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION
  PASSWORD :'authpw';

CREATE OR REPLACE FUNCTION public.pgbouncer_get_auth(p_usename TEXT)
RETURNS TABLE(usename TEXT, passwd TEXT)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
  SELECT usename::TEXT, passwd::TEXT
  FROM pg_catalog.pg_shadow
  WHERE usename = p_usename;
$$;

REVOKE EXECUTE ON FUNCTION public.pgbouncer_get_auth(TEXT) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.pgbouncer_get_auth(TEXT) TO :"authuser";
SQL

echo "door-auth: the doors' auth user and its lookup function are installed"
