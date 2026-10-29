.PHONY: test build-dev build-prod dev e2e

# All checks: Go vet + tests, plus the JS engine parity/mask tests.
test:
	go vet ./...
	go test ./...
	node --test web/test/*.test.mjs

build-dev:
	go run ./cmd/build -env dev

build-prod:
	go run ./cmd/build -env prod

# Local dev: mock AI API on 127.0.0.1:18790 + static site on 127.0.0.1:8788.
# Page: http://localhost:8788/budol-bantay/
dev: build-dev
	@trap 'kill 0' INT TERM EXIT; \
	BUDOL_ENV=dev AI_MODE=mock BUDOL_LISTEN=127.0.0.1:18790 ALLOWED_ORIGINS=http://localhost:8788 go run ./cmd/budol-api & \
	python3 -m http.server 8788 --bind 127.0.0.1 -d dist & \
	wait

# Headless end-to-end (mock AI): builds, serves, screenshots, asserts.
e2e:
	bash scripts/e2e.sh
