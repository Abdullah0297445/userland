bats_require_minimum_version 1.5.0

setup_file() {
	export project=userland-test
	export env_file="$BATS_FILE_TMPDIR/env"
	cat >"$env_file" <<'EOF'
POSTGRES_PASSWORD=postgres-password
PGBOUNCER_AUTH_PASSWORD=pgbouncer-auth-password
EOF
	compose down --volumes --remove-orphans
	compose up --detach --wait postgres-18 pgbouncer-transaction pgbouncer-session
}

teardown_file() {
	compose down --volumes --remove-orphans
}

compose() {
	COMPOSE_FILE=compose.yml:compose/postgres.yml docker compose --project-name "$project" --env-file "$env_file" "$@"
}

connect() {
	docker run --rm --network "${project}_postgres" pgvector/pgvector:pg18-trixie \
		psql "$1" -v ON_ERROR_STOP=1 -X -q -tA -c "$2"
}

superuser() {
	docker exec postgres-18 psql -v ON_ERROR_STOP=1 -X -q -tA -U postgres -d postgres -c "$1"
}

printed() {
	sed -n "s/^  $1=//p" <<<"$output"
}

@test "the doors look passwords up as pgbouncer_auth, which is no superuser and inherits nothing" {
	run superuser "SELECT rolsuper, rolinherit FROM pg_roles WHERE rolname = 'pgbouncer_auth'"
	[ "$output" = "f|f" ]
	run superuser "SELECT prosecdef FROM pg_proc WHERE proname = 'pgbouncer_get_auth'"
	[ "$output" = "t" ]
}

@test "a database added is reached through the transaction door, as the user that owns it" {
	run --separate-stderr bin/add-database shop
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	[[ "$url" == postgresql://shop:*@pgbouncer-transaction:5432/shop ]]
	[ -z "$(printed PGRST_DB_URI)" ]
	run connect "$url" "SELECT current_user || ' ' || current_database()"
	[ "$status" -eq 0 ]
	[ "$output" = "shop shop" ]
	run connect "$url" "SELECT extname FROM pg_extension WHERE extname = 'vector'"
	[ "$output" = "vector" ]
}

@test "a product's database is added the same way, and the printed password line logs in through the door" {
	run --separate-stderr bin/add-database metabase
	[ "$status" -eq 0 ]
	password=$(printed METABASE_DB_PASSWORD)
	[ "${#password}" -eq 32 ]
	run connect "postgresql://metabase:$password@pgbouncer-transaction:5432/metabase" "SELECT current_user || ' ' || current_database()"
	[ "$status" -eq 0 ]
	[ "$output" = "metabase metabase" ]
}

@test "with --session, the DSN names the session door" {
	run --separate-stderr bin/add-database --session diary
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	[[ "$url" == postgresql://diary:*@pgbouncer-session:5432/diary ]]
	run connect "$url" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "diary" ]
}

@test "a database's user reaches no other database" {
	run --separate-stderr bin/add-database left
	[ "$status" -eq 0 ]
	left=$(printed DATABASE_URL)
	run --separate-stderr bin/add-database right
	[ "$status" -eq 0 ]
	run connect "${left%/left}/right" "SELECT 1"
	[ "$status" -ne 0 ]
	[[ "$output" == *"permission denied for database"* ]]
}

@test "with --api, the recipe is installed, and PostgREST's DSN names the session door" {
	run --separate-stderr bin/add-database --api notes
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	api=$(printed PGRST_DB_URI)
	[[ "$url" == postgresql://notes:*@pgbouncer-transaction:5432/notes ]]
	[[ "$api" == postgresql://notes_authenticator:*@pgbouncer-session:5432/notes ]]
	[ "$(printed PGRST_DB_SCHEMAS)" = api ]
	[ "$(printed PGRST_DB_ANON_ROLE)" = notes_anon ]
	run connect "$url" "CREATE TABLE api.items (id int); INSERT INTO api.items VALUES (1); GRANT SELECT ON api.items TO notes_anon"
	[ "$status" -eq 0 ]
	run connect "$api" "SELECT id FROM api.items"
	[ "$status" -ne 0 ]
	[[ "$output" == *"permission denied"* ]]
	run connect "$api" "SET ROLE notes_anon; SELECT id FROM api.items"
	[ "$status" -eq 0 ]
	[ "$output" = "1" ]
	run connect "$url" "SELECT evtname FROM pg_event_trigger"
	[ "$output" = "pgrst_watch" ]
}

@test "with --session and --api, both DSNs name the session door, and both connect" {
	run --separate-stderr bin/add-database --session --api board
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	api=$(printed PGRST_DB_URI)
	[[ "$url" == postgresql://board:*@pgbouncer-session:5432/board ]]
	[[ "$api" == postgresql://board_authenticator:*@pgbouncer-session:5432/board ]]
	run connect "$url" "SELECT current_user"
	[ "$output" = "board" ]
	run connect "$api" "SELECT current_user"
	[ "$output" = "board_authenticator" ]
}

@test "adding a name twice is refused, and changes nothing" {
	run --separate-stderr bin/add-database twice
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	run --separate-stderr bin/add-database twice
	[ "$status" -eq 1 ]
	[[ "$stderr" == *"the database twice already exists. Nothing was changed."* ]]
	run connect "$url" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "twice" ]
}

@test "a role left behind without its database stops an add, and remove clears it" {
	superuser "CREATE ROLE half_anon NOLOGIN"
	run --separate-stderr bin/add-database half
	[ "$status" -eq 1 ]
	[[ "$stderr" == *"there is no database half, but these remain from an earlier one: half_anon."* ]]
	run superuser "SELECT count(*) FROM pg_roles WHERE rolname LIKE 'half%'"
	[ "$output" = "1" ]
	run --separate-stderr bin/remove-database half <<<"half"
	[ "$status" -eq 0 ]
	run --separate-stderr bin/add-database half
	[ "$status" -eq 0 ]
}

@test "a bad name is refused by both helpers, before anything is made" {
	local long name
	long=$(printf 'a%.0s' {1..64})
	for name in '' my-app MyApp 1app "$long" "my'app" 'my app' postgres template1 pgbouncer_auth pg_app; do
		run --separate-stderr bin/add-database "$name"
		[ "$status" -eq 1 ]
		run --separate-stderr bin/remove-database "$name"
		[ "$status" -eq 1 ]
	done
	run --separate-stderr bin/add-database --api "$(printf 'a%.0s' {1..50})"
	[ "$status" -eq 1 ]
	[[ "$stderr" == *"too long for --api"* ]]
	run superuser "SELECT count(*) FROM pg_database WHERE datname LIKE 'aaaaaaaa%'"
	[ "$output" = "0" ]
}

@test "a helper run with no name, or with two, prints its usage" {
	run --separate-stderr bin/add-database
	[ "$status" -eq 1 ]
	[[ "$stderr" == *"usage: bin/add-database [--session] [--api] NAME"* ]]
	run --separate-stderr bin/add-database one two
	[ "$status" -eq 1 ]
	run --separate-stderr bin/add-database --pool one
	[ "$status" -eq 1 ]
	run --separate-stderr bin/remove-database
	[ "$status" -eq 1 ]
	[[ "$stderr" == *"usage: bin/remove-database NAME"* ]]
}

@test "remove asks first, and drops nothing unless the name is typed" {
	run --separate-stderr bin/add-database keep
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	run --separate-stderr bin/remove-database keep <<<"y"
	[ "$status" -eq 1 ]
	[[ "$output" == *"This drops the database keep."* ]]
	[[ "$output" == *"Nothing was dropped."* ]]
	run --separate-stderr bin/remove-database keep </dev/null
	[ "$status" -eq 1 ]
	run connect "$url" "SELECT current_user"
	[ "$output" = "keep" ]
}

@test "remove drops the database while both doors hold it, its user and both roles, and the name can be added again" {
	run --separate-stderr bin/add-database --api again
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	api=$(printed PGRST_DB_URI)
	run connect "$url" "SELECT 1"
	[ "$status" -eq 0 ]
	run connect "$api" "SELECT 1"
	[ "$status" -eq 0 ]
	run superuser "SELECT count(*) > 0 FROM pg_stat_activity WHERE datname = 'again'"
	[ "$output" = "t" ]

	run --separate-stderr bin/remove-database again <<<"again"
	[ "$status" -eq 0 ]
	[[ "$output" == *"This drops the database again."* ]]
	[[ "$output" == *"This drops the user again."* ]]
	[[ "$output" == *"This drops PostgREST's roles: again_anon, again_authenticator."* ]]
	[[ "$output" == *"Dropped the role again_authenticator."* ]]
	run superuser "SELECT count(*) FROM pg_database WHERE datname = 'again'"
	[ "$output" = "0" ]
	run superuser "SELECT count(*) FROM pg_roles WHERE rolname LIKE 'again%'"
	[ "$output" = "0" ]

	run --separate-stderr bin/add-database --session again
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	run connect "$url" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "again" ]
	run connect "${url/pgbouncer-session/pgbouncer-transaction}" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "again" ]
}

@test "a new password logs in through the door, and the old one no longer does" {
	run --separate-stderr bin/add-database renew
	[ "$status" -eq 0 ]
	old=$(printed DATABASE_URL)
	run connect "$old" "SELECT 1"
	[ "$status" -eq 0 ]
	run --separate-stderr bin/new-password renew
	[ "$status" -eq 0 ]
	new=$(printed DATABASE_URL)
	password=$(printed RENEW_DB_PASSWORD)
	[ "${#password}" -eq 32 ]
	[ "$new" = "postgresql://renew:$password@pgbouncer-transaction:5432/renew" ]
	[ "$new" != "$old" ]
	run connect "$new" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "renew" ]
	run connect "$old" "SELECT 1"
	[ "$status" -ne 0 ]
	[[ "$output" == *"SASL authentication failed"* ]]
}

@test "with --session, the new password's DSN names the session door" {
	run --separate-stderr bin/add-database --session rotate
	[ "$status" -eq 0 ]
	run --separate-stderr bin/new-password --session rotate
	[ "$status" -eq 0 ]
	new=$(printed DATABASE_URL)
	[[ "$new" == postgresql://rotate:*@pgbouncer-session:5432/rotate ]]
	run connect "$new" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "rotate" ]
}

@test "PostgREST's authenticator gets a new password, and only PGRST_DB_URI is printed" {
	run --separate-stderr bin/add-database --api feed
	[ "$status" -eq 0 ]
	old=$(printed PGRST_DB_URI)
	run connect "$old" "SELECT 1"
	[ "$status" -eq 0 ]
	run --separate-stderr bin/new-password feed_authenticator
	[ "$status" -eq 0 ]
	new=$(printed PGRST_DB_URI)
	[[ "$new" == postgresql://feed_authenticator:*@pgbouncer-session:5432/feed ]]
	[ -z "$(printed DATABASE_URL)" ]
	run connect "$new" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "feed_authenticator" ]
	run connect "$old" "SELECT 1"
	[ "$status" -ne 0 ]
	[[ "$output" == *"SASL authentication failed"* ]]
}

@test "the doors' auth user gets a new password, and both doors let users in after up" {
	run --separate-stderr bin/add-database gate
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	run --separate-stderr bin/new-password pgbouncer_auth
	[ "$status" -eq 0 ]
	[ -z "$(printed DATABASE_URL)" ]
	password=$(printed PGBOUNCER_AUTH_PASSWORD)
	[ "${#password}" -eq 32 ]
	sed -i "s/^PGBOUNCER_AUTH_PASSWORD=.*/PGBOUNCER_AUTH_PASSWORD=$password/" "$env_file"
	compose up --detach --wait postgres-18 pgbouncer-transaction pgbouncer-session
	run connect "$url" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "gate" ]
	run connect "${url/pgbouncer-transaction/pgbouncer-session}" "SELECT current_user"
	[ "$status" -eq 0 ]
	[ "$output" = "gate" ]
}

@test "new-password refuses the superuser, a user without its database, an anon role and a bad name, and changes nothing" {
	run --separate-stderr bin/add-database --api spare
	[ "$status" -eq 0 ]
	url=$(printed DATABASE_URL)
	api=$(printed PGRST_DB_URI)
	superuser "CREATE ROLE lone LOGIN PASSWORD 'lone-password'"
	local long name
	long=$(printf 'a%.0s' {1..64})
	for name in postgres lone spare_anon ghost_authenticator ghost pg_monitor '' my-app MyApp 1app "$long" "my'app"; do
		run --separate-stderr bin/new-password "$name"
		[ "$status" -eq 1 ]
	done
	run --separate-stderr bin/new-password postgres
	[[ "$stderr" == *"postgres is a superuser"* ]]
	run --separate-stderr bin/new-password
	[ "$status" -eq 1 ]
	[[ "$stderr" == *"usage: bin/new-password [--session] NAME"* ]]
	run --separate-stderr bin/new-password spare spare
	[ "$status" -eq 1 ]
	run --separate-stderr bin/new-password --api spare
	[ "$status" -eq 1 ]
	run connect "$url" "SELECT current_user"
	[ "$output" = "spare" ]
	run connect "$api" "SELECT current_user"
	[ "$output" = "spare_authenticator" ]
	run connect "postgresql://lone:lone-password@pgbouncer-transaction:5432/postgres" "SELECT current_user"
	[ "$output" = "lone" ]
	run connect "postgresql://postgres:postgres-password@pgbouncer-transaction:5432/postgres" "SELECT current_user"
	[ "$output" = "postgres" ]
}

@test "remove of a name that is not there drops nothing, and says so" {
	run --separate-stderr bin/remove-database ghost </dev/null
	[ "$status" -eq 0 ]
	[ "$output" = "There is no database ghost, and no user or role of that name. Nothing was dropped." ]
}
