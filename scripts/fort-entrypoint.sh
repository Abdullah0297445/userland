#!/bin/sh
set -e

backup() {
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
	restic backup "$@"
}

schedule() {
	umask 077
	export -p >/run/fort.env
	printf '%s . /run/fort.env; /fort-entrypoint.sh backup\n' "${FORT_SCHEDULE:-@daily}" >/etc/crontabs/root
	echo "fort: ${FORT_SCHEDULE:-@daily}, keeping ${FORT_FILES:-nothing}"
	exec crond -f -L /dev/stdout
}

case "${1:-}" in
"") schedule ;;
backup) backup ;;
*) exec restic "$@" ;;
esac
