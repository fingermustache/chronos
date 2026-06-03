DB_URL := postgres://chronos:chronos@localhost:5432/chronos?sslmode=disable

.PHONY: db/start db/stop db/migrate db/seed db/reset

## Start the dev database
db/start:
	docker compose up -d db
	docker compose wait db

## Stop the dev database
db/stop:
	docker compose down

## Apply all migrations
db/migrate:
	migrate -path src/database/migrations -database "$(DB_URL)" up

## Seed the dev database
db/seed:
	psql $(DB_URL) -f src/database/migrations/seed.sql

## Full reset — wipe volume, restart, migrate, seed
db/reset:
	docker compose down -v
	docker compose up -d db
	docker compose wait db
	$(MAKE) db/migrate
	$(MAKE) db/seed
