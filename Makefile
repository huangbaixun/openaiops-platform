.PHONY: up down build seed smoke fmt fmt-go fmt-fe lint lint-go lint-fe lint-ch test test-go test-go-integration test-fe e2e migrate-up migrate-down migrate-ch-up seed-traces demo-traces seed-logs demo-logs seed-topology demo-topology

up:
	docker-compose -f deploy/docker-compose.yml up -d

down:
	docker-compose -f deploy/docker-compose.yml down

build:
	docker-compose -f deploy/docker-compose.yml build

seed:
	docker-compose -f deploy/docker-compose.yml exec -T postgres psql -U openaiops -d openaiops < deploy/seed.sql

smoke:
	@KEY=test-key-acme; \
	curl -sf -H "Authorization: Bearer $$KEY" http://localhost:8080/healthz | jq .

fmt: fmt-go fmt-fe

fmt-go:
	@if [ -d backend ]; then cd backend && gofmt -w . && go vet ./...; else echo "skip fmt-go: backend/ not yet created"; fi

fmt-fe:
	@if [ -d frontend ]; then cd frontend && npm run format; else echo "skip fmt-fe: frontend/ not yet created"; fi

lint: lint-go lint-fe lint-ch

lint-go:
	@if [ -d backend ]; then cd backend && golangci-lint run ./...; else echo "skip lint-go: backend/ not yet created"; fi

lint-fe:
	@if [ -d frontend ]; then cd frontend && npm run lint; else echo "skip lint-fe: frontend/ not yet created"; fi

lint-ch:
	@./deploy/lint-no-bare-ch.sh

test: test-go test-fe

test-go:
	cd backend && go test ./...

test-go-integration:
	cd backend && go test -tags=integration -count=1 -timeout 240s ./...

test-fe:
	cd frontend && npm run test

e2e:
	cd frontend && npx playwright test

# Migration targets require DATABASE_URL env var set, e.g.:
#   export DATABASE_URL=postgres://openaiops:openaiops@localhost:5432/openaiops?sslmode=disable
# When running against the local compose stack (deploy/docker-compose.yml), use the URL above.
migrate-up:
	cd backend && goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	cd backend && goose -dir migrations postgres "$$DATABASE_URL" down

# Apply ClickHouse migrations against the running compose stack. Re-runs are
# idempotent (skips already-applied versions via the _schema_migrations table).
# See docs/decisions/0002-clickhouse-schema-migrations.md.
migrate-ch-up:
	docker-compose -f deploy/docker-compose.yml run --rm ch-migrate

# Seed deterministic trace data into the running ingester (used by CI e2e + manual sanity).
# Honors INGESTER_OTLP_GRPC_HOST_PORT for local SignOz collision override.
seed-traces:
	cd backend && go run ./cmd/seed-traces \
		--target=localhost:$${INGESTER_OTLP_GRPC_HOST_PORT:-4317} \
		--tenant-key=test-key-acme \
		--spans=5 \
		--traces=3

# Start the hot-r.o.d. demo service that streams lifelike business traffic into the ingester.
demo-traces:
	docker compose -f deploy/docker-compose.yml --profile demo up -d hot-r-o-d

# Seed deterministic log data into the running log-ingester (used by CI e2e + manual sanity).
# Honors LOG_INGESTER_OTLP_GRPC_HOST_PORT for local override.
seed-logs:
	cd backend && go run ./cmd/seed-logs

demo-logs: seed-traces seed-logs
	@echo "demo data seeded — open https://localhost/logs and https://localhost/traces"

# Seed a hot-r.o.d.-style 4-service topology (frontend→checkout→payment + checkout→redis)
# into the running ingester. Honors INGESTER_OTLP_GRPC_HOST_PORT for local override.
# topo-engine derives topology_edges_v1 + service_stats_v1 rows on its next 1-min tick.
seed-topology:
	cd backend && API_KEY=$${API_KEY:-test-key-acme} \
		INGESTER_OTLP_GRPC_HOST_PORT=$${INGESTER_OTLP_GRPC_HOST_PORT:-4317} \
		go run ./cmd/seed-topology

demo-topology: seed-topology
	@echo "Waiting ~120s for topo-engine to process the closed minute bucket..."
	@sleep 120
	@echo "demo topology seeded — open https://localhost/overview, /topology, /services/checkout"
