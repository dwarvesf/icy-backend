POSTGRES_CONTAINER?=icy_backend_local
POSTGRES_TEST_CONTAINER?=icy_backend_local_test
remove-infras:
	docker compose down --remove-orphans --volumes

shell:
	@if ! command -v devbox >/dev/null 2>&1; then curl -fsSL https://get.jetpack.io/devbox | bash; fi
	@devbox install
	@devbox shell
	
init:
	make remove-infras
	docker compose up -d
	@echo "Waiting for database connection..."
	@while ! docker exec ${POSTGRES_CONTAINER} pg_isready > /dev/null; do \
		sleep 1; \
	done
	@while ! docker exec $(POSTGRES_TEST_CONTAINER) pg_isready > /dev/null; do \
		sleep 1; \
	done

dev:
	go run ./cmd/server/main.go