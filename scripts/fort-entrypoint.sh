#!/bin/sh
set -e

clickhouse_image_user=101:101

backup() {
	worst=0
	part files
	[ -z "${PGHOST:-}" ] || part postgres
	[ -z "${CLICKHOUSE_HOST:-}" ] || part clickhouse
	return "$worst"
}

part() {
	code=0
	"$@" || code=$?
	if [ "$code" -ne 0 ] && [ "$worst" -ne 1 ]; then
		worst=$code
	fi
}

files() {
	if [ -z "$FORT_FILES" ]; then
		echo "fort: FORT_FILES is empty, so there is nothing to keep" >&2
		return 1
	fi
	old=$IFS
	IFS=:
	set -- $FORT_FILES
	IFS=$old
	for path in "$@"; do
		shift
		set -- "$@" "/files$path"
	done
	restic backup --tag files "$@"
}

postgres() {
	databases=$(psql -d postgres -tAX -v ON_ERROR_STOP=1 -c "SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate AND datname <> 'postgres' ORDER BY datname") || {
		echo "fort: could not read the databases on $PGHOST, so none was kept" >&2
		return 1
	}
	failed=0
	restic backup --tag postgres --stdin-filename /postgres/globals.sql --stdin-from-command -- pg_dumpall --globals-only </dev/null || failed=1
	while IFS= read -r database; do
		[ -n "$database" ] || continue
		restic backup --tag postgres --stdin-filename "/postgres/$database.dump" --stdin-from-command -- pg_dump --format=custom --compress=0 "$database" </dev/null || {
			echo "fort: $database on $PGHOST was not kept" >&2
			failed=1
		}
	done <<EOF
$databases
EOF
	return "$failed"
}

clickhouse() {
	databases=$(query "SELECT name FROM system.databases WHERE name NOT IN ('system', 'information_schema', 'INFORMATION_SCHEMA') ORDER BY name") || {
		echo "fort: could not read the databases on $CLICKHOUSE_HOST, so none was kept" >&2
		return 1
	}
	rm -rf /clickhouse/*
	chown "$clickhouse_image_user" /clickhouse
	failed=0
	while IFS= read -r database; do
		[ -n "$database" ] || continue
		query "BACKUP DATABASE \`$database\` TO File('$database')" >/dev/null || {
			echo "fort: $database on $CLICKHOUSE_HOST was not kept" >&2
			failed=1
		}
	done <<EOF
$databases
EOF
	restic backup --tag clickhouse /clickhouse || failed=1
	rm -rf /clickhouse/*
	return "$failed"
}

query() {
	printf 'header = "X-ClickHouse-Key: %s"\n' "$CLICKHOUSE_PASSWORD" |
		curl -sS --fail-with-body -K - -H "X-ClickHouse-User: default" --data-binary "$1" "http://$CLICKHOUSE_HOST:8123/"
}

schedule() {
	umask 077
	export -p >/run/fort.env
	printf '%s . /run/fort.env; /fort-entrypoint.sh backup\n' "${FORT_SCHEDULE:-@daily}" >/etc/crontabs/root
	echo "fort: ${FORT_SCHEDULE:-@daily}, keeping ${FORT_FILES:-no file}${PGHOST:+, every database on $PGHOST}${CLICKHOUSE_HOST:+, every database on $CLICKHOUSE_HOST}"
	exec crond -f -L /dev/stdout
}

case "${1:-}" in
"") schedule ;;
backup) backup ;;
*) exec restic "$@" ;;
esac
