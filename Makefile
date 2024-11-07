POSTGRES_CONTAINER?=icy_backend_local
POSTGRES_TEST_CONTAINER?=icy_backend_local_test
remove-infras:
	docker compose down --remove-orphans --volumes

shell:
	@if ! command -v devbox >/dev/null 2>&1; then curl -fsSL https://get.jetpack.io/devbox | bash; fi
	@devbox install
	@devbox shell

init:
	devbox run init-db
	@devbox services start
	@devbox services ls

reset:
	devbox run clean-db
	devbox run init-db
	@devbox services start
	@devbox services ls

dev:
	go run ./cmd/server/main.go
