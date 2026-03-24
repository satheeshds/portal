# Accounting Service

Make targets are provided for common workflows:

- `make image` – build the Docker image (`IMAGE` defaults to `satheeshds/accounting`, `TAG` defaults to `latest`).
- `make push` – push the Docker image.
- `make compose-up` – start the stack with docker compose (uses `ENV_FILE`, default `.env`).
- `make compose-down` – stop the stack.
- `make build` – build the Go binary.
- `make test` – run the Go test suite.

Notes:
- Docker BuildKit is enabled by default (`DOCKER_BUILDKIT=1`).
- Override `IMAGE`, `TAG`, `DOCKER`, `COMPOSE`, or `ENV_FILE` as needed, e.g. `ENV_FILE=.env.local make compose-up`.
- Ensure required environment variables (e.g. `AUTH_USER`, `AUTH_PASS`, `DB_PATH`) are set via your env file or shell before running compose.
