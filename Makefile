down:
	docker compose down --remove-orphans --volumes

# Initialize the database and start the services
# Assumes devbox run init-db handles DB setup after the container is running
init: down
	@ make stop || true
	@ echo "Starting PostgreSQL container..."
	@ docker compose up -d
	@ echo "Waiting for PostgreSQL to be ready..."

shell:
	@if ! command -v devbox >/dev/null 2>&1; then curl -fsSL https://get.jetpack.io/devbox | bash; fi
	@devbox install
	@devbox shell

# Run the server
dev:
	@echo "Starting server..."
	@devbox run server

gen-swagger:
	swag init --parseDependency -g ./cmd/server/main.go
	
migrate-up:
	devbox run migrate-up

migrate-down:
	devbox run migrate-down
