.PHONY: up down build seed smoke fmt lint test test-go test-fe e2e migrate-up migrate-down

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

fmt:
	cd backend && gofmt -w . && go vet ./...
	cd frontend && npm run format

lint:
	cd backend && golangci-lint run ./...
	cd frontend && npm run lint

test: test-go test-fe

test-go:
	cd backend && go test ./...

test-fe:
	cd frontend && npm run test

e2e:
	cd frontend && npx playwright test

migrate-up:
	cd backend && goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	cd backend && goose -dir migrations postgres "$$DATABASE_URL" down
