POSTGRES_CONTAINER?=icy_backend_local
POSTGRES_TEST_CONTAINER?=icy_backend_local_test

remove-infras:
	docker compose down --remove-orphans --volumes

shell:
	@if ! command -v devbox >/dev/null 2>&1; then curl -fsSL https://get.jetpack.io/devbox | bash; fi
	@devbox install
	@devbox shell

# Initialize the database and start the services
init:
	@ make stop || true
	@if [ -n "$$(ls -A ./data/dev 2>/dev/null)" ] || [ -n "$$(ls -A ./data/test 2>/dev/null)" ]; then \
		echo "Error: ./data/dev or ./data/test is not empty. Please run 'make clean' first."; \
		exit 1; \
	fi
	devbox run init-db
	@devbox services start
	@devbox services ls

# Clean the database
clean:
	@ make stop || true
	devbox run clean-db	

# Reset the database and start the services
reset:
	@ make stop || true
	@ make clean
	devbox run init-db
	@devbox services start
	@devbox services ls

# Start the services including the database
start:
	@devbox services start
	@devbox services ls

# Stop the services including the database
stop:
	@devbox services stop	

# Run the server
dev:
	go run ./cmd/server/main.go

gen-swagger:
	swag init --parseDependency -g ./cmd/server/main.go
	
migrate:
	devbox run migrate

migrate-down:
	devbox run migrate-down
