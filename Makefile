.PHONY: up down e2e perf setup test backup purge api openapi pgsql connect

WEB_PORT := 3000
WEB_LOG := $(CURDIR)/.build/web.log

# Docker, API, then the web app in the background.
up:
	@colima start && docker compose up -d
	@$(MAKE) --no-print-directory api
	@mkdir -p .build
	@if lsof -ti tcp:$(WEB_PORT) -sTCP:LISTEN >/dev/null; then \
		echo "Port $(WEB_PORT) already in use: web app not started"; \
	else \
		cd web && bun install --silent && \
		{ nohup bun run dev > $(WEB_LOG) 2>&1 < /dev/null & } && \
		echo "http://localhost:$(WEB_PORT) (log: .build/web.log)"; \
	fi

# Keeps Colima running: other projects may use it. --profile api: a plain down leaves the API out.
# TODO: stops whatever listens on the web port, add a pid file if another app ever uses it.
down:
	@lsof -ti tcp:$(WEB_PORT) -sTCP:LISTEN | xargs kill 2>/dev/null || true
	@docker compose --profile api down

# The browser tests run against the stack, in the installed Chrome.
e2e: up
	@cd web && bun run e2e

# The app at scale (#138): the account perf of 50,000 generated notes, made once (14 s), then the timings of its API
# and pages, a table to paste in an issue. PERF_NOTES only counts when the account does not exist yet.
# The pages are those of prod (#166): the generated app, served with a second API on its own origin, the nuxt dev
# of :3000 left running.
PERF_PASSWORD := perf-password
PERF_NOTES := 50000
PERF_PORT := 8090
perf: up
	@echo "$(PERF_PASSWORD)" | docker exec -i note-api /tide-server user perf perf $(PERF_NOTES) --password-stdin
	@cd web && NUXT_PUBLIC_API_BASE= bun --bun run generate > $(CURDIR)/.build/perf-generate.log
	@docker rm -f note-perf >/dev/null 2>&1 || true
	@docker run -d --name note-perf --network tide_default --env-file .env.docker -e CONFIG_FILE= \
		-v $(CURDIR)/web/.output/public:/web:ro -p 127.0.0.1:$(PERF_PORT):$(PERF_PORT) tide-api \
		serve --addr :$(PERF_PORT) --web /web --origin=http://localhost:$(PERF_PORT) >/dev/null
	@until curl -sf http://localhost:$(PERF_PORT)/ >/dev/null; do sleep 0.2; done
	@cd web && E2E_APP=http://localhost:$(PERF_PORT) E2E_API=http://localhost:$(PERF_PORT) \
		PERF_PASSWORD=$(PERF_PASSWORD) node scripts/perf.mjs; status=$$?; docker rm -f note-perf >/dev/null; exit $$status

# Not `go tool lefthook install`: the hook would call a build cache path that expires, then skip its checks without failing.
setup:
	go install github.com/evilmartians/lefthook/v2
	go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
	"$$(go env GOBIN | grep . || echo "$$(go env GOPATH)/bin")/lefthook" install --force

# Every run has a base and a Silo bucket of its own, removed after it, even on Ctrl+C: agents run the tests at once,
# and the schema, purges and sweeps of one broke the others'. A migration being written never reaches the API's base.
# -p 1: the packages of a run share its base, and deadlock when they create the schema and tags at the same time.
# A plain go test (the note-app command) shares <POSTGRES_DB>_test and notes-test, one package at a time (postgrey).
test:
	@set -a && . ./.env && set +a && db=$${POSTGRES_DB}_test_$$$$ && bucket=notes-test-$$$$ && \
	trap 'docker compose exec -T -e DB=$$db postgres sh -c "dropdb -U \"\$$POSTGRES_USER\" --if-exists --force \$$DB"; \
		docker compose exec -T -e BUCKET=$$bucket silo sh -c "mcli alias set local http://127.0.0.1:9000 \"\$$MINIO_ROOT_USER\" \"\$$MINIO_ROOT_PASSWORD\" >/dev/null && mcli rb --force local/\$$BUCKET >/dev/null 2>&1"; true' EXIT && \
	trap 'exit 130' INT TERM && \
	docker compose exec -T -e DB=$$db postgres sh -c 'createdb -U "$$POSTGRES_USER" "$$DB"' && \
	POSTGRES_DB=$$db S3_TEST_BUCKET=$$bucket CONFIG_FILE=$(CURDIR)/config/config.yaml go test -p 1 $(or $(ARGS),./...)

# The base, then the images of Silo beside it. Restore:
#   docker compose exec -T postgres sh -c 'psql -U "$$POSTGRES_USER" "$$POSTGRES_DB"' < backups/<file>.sql
#   docker compose cp backups/<file>.files silo:/tmp/restore && docker compose exec silo sh -c \
#     'mcli alias set local http://127.0.0.1:9000 "$$MINIO_ROOT_USER" "$$MINIO_ROOT_PASSWORD" && mcli mirror /tmp/restore local/<bucket>'
backup:
	@mkdir -p backups
	@set -a && . ./.env && set +a && \
	file=backups/tide-$$(date +%Y%m%d-%H%M%S); \
	docker compose exec -T postgres sh -c 'pg_dump --clean --if-exists -U "$$POSTGRES_USER" "$$POSTGRES_DB"' > $$file.tmp \
		&& mv $$file.tmp $$file.sql && echo $$file.sql \
		|| { rm -f $$file.tmp; exit 1; }; \
	docker compose exec -T silo sh -c 'rm -rf /tmp/backup && mkdir /tmp/backup \
		&& mcli alias set local http://127.0.0.1:9000 "$$MINIO_ROOT_USER" "$$MINIO_ROOT_PASSWORD" >/dev/null \
		&& mcli mirror --quiet local/'"$$S3_BUCKET"' /tmp/backup >/dev/null' \
		&& docker compose cp silo:/tmp/backup $$file.files && echo $$file.files

# Every note, folder, tag and image of every account, after a backup. The accounts, their settings and sessions stay.
# One account empties its own from the settings of the web app (DELETE /api/me/data).
purge:
	@printf 'Delete the notes, folders, tags, images, snippets and saved searches of every account? Type purge: ' && read answer && [ "$$answer" = purge ]
	@$(MAKE) --no-print-directory backup
	@set -a && . ./.env && set +a && \
	docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" "$$POSTGRES_DB" -c "TRUNCATE items, tags, files CASCADE" -c "UPDATE users SET avatar = NULL, snippets = '"'"'[]'"'"', saved_searches = '"'"'[]'"'"'"' && \
	docker compose exec -T silo sh -c 'mcli alias set local http://127.0.0.1:9000 "$$MINIO_ROOT_USER" "$$MINIO_ROOT_PASSWORD" >/dev/null \
		&& mcli rm --recursive --force --quiet local/'"$$S3_BUCKET"

# Logs the CLI in to a server, the commands acting there until the next connect: `make connect TO=fedora` (the
# server at TIDE_FEDORA_URL of .env) or `make connect TO=local` (the API of `make api`).
# TODO: NOTES_FEDORA_URL is read while TIDE_FEDORA_URL is unset, remove it with the NOTES_ fallback of config.Env.
connect:
	@set -a && . ./.env && set +a && \
	case "$(TO)" in \
		fedora) url=$${TIDE_FEDORA_URL:-$$NOTES_FEDORA_URL} ;; \
		local) url=http://localhost:8080 ;; \
		*) echo "usage: make connect TO=fedora|local"; exit 1 ;; \
	esac; \
	test -n "$$url" || { echo "set TIDE_FEDORA_URL in .env"; exit 1; }; \
	go run ./cmd/tide login "$$url" && go run ./cmd/tide whoami

# psql on the base: `make pgsql` opens it, `make pgsql QUERY="ALTER TABLE ..."` runs one statement and stops at an error.
pgsql:
	@set -a && . ./.env && set +a && \
	if [ -n "$$QUERY" ]; then \
		docker compose exec -T -e QUERY postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" "$$POSTGRES_DB" -c "$$QUERY"'; \
	else \
		docker compose exec postgres sh -c 'psql -U "$$POSTGRES_USER" "$$POSTGRES_DB"'; \
	fi

api:
	@VERSION=$$(git describe --tags --always --dirty) docker compose up -d --build api

# Generates docs/swagger.yaml from the @ comments of core/api; no docs.go, the binary stays out of it.
# Every field is required unless tagged binding:"optional". Then the CLI's client of the API from it.
openapi:
	go tool swag init --v3.1 --requiredByDefault -d core/api,core/domain,core/services -g router.go -o docs --outputTypes yaml
	go tool oapi-codegen -generate types,client -package apiclient -o cli/services/apiclient/client.gen.go docs/swagger.yaml
