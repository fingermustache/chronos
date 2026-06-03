ifneq (,$(wildcard .env))
include .env
export
endif

DB_USER ?= chronos
DB_PASSWORD ?= chronos
DB_HOST ?= localhost
DB_PORT ?= 5432
DB_NAME ?= chronos
DB_SSLMODE ?= disable
DB_URL := postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

.PHONY: db/start db/stop db/migrate db/seed db/reset db/wait

## Wait for the dev database to become ready
db/wait:
	@until pg_isready -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME); do \
		echo "Waiting for database..."; \
		sleep 2; \
	done

## Start the dev database
db/start:
	docker compose up -d db
	$(MAKE) db/wait

## Stop the dev database
db/stop:
	docker compose down

## Apply all migrations
db/migrate:
	migrate -path src/database/migrations -database "$(DB_URL)" up

## Seed the dev database
db/seed:
	psql "$(DB_URL)" -f src/database/migrations/seed.sql

## Full reset — wipe volume, restart, migrate, seed
db/reset:
	docker compose down -v
	docker compose up -d db
	$(MAKE) db/wait
	$(MAKE) db/migrate
	$(MAKE) db/seed
