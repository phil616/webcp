.PHONY: build web-build run test integration integration-acme test-web test-web-dev check verify notices security dev dev-web release verify-release clean

CONFIG ?= webcp.json
ARCH ?= amd64
RELEASE_DIR ?= dist/releases

build: web-build
	python3 scripts/build.py --arch $(ARCH)

web-build:
	npm ci --prefix web
	npm run build:pro --prefix web
	python3 scripts/dependency-notices.py --sync-dist

run: build
	./bin/webcp -config $(CONFIG)

verify:
	python3 scripts/check-repo.py

notices:
	python3 scripts/dependency-notices.py

check: web-build verify
	go test -race ./...
	go vet ./...
	npm run format:check --prefix web

security:
	go run golang.org/x/vuln/cmd/govulncheck@v1.2.0 ./...
	npm audit --prefix web --audit-level=moderate

test: web-build
	go test -race ./...

integration:
	RESTY_INTEGRATION=1 go test -v ./internal/openresty ./internal/service -run 'Test.*Integration' -count=1

integration-acme:
	RESTY_ACME_INTEGRATION=1 go test -v ./internal/service -run '^TestSSLACMEIntegration$$' -count=1

# Temporary state and ownership labels keep these tests away from local user data.
test-web: build
	python3 scripts/test-web.py

test-web-dev: build
	python3 scripts/test-web.py --dev

release: web-build verify
	python3 scripts/package-release.py --output $(RELEASE_DIR)

dev:
	go run ./cmd/web-panel -config $(CONFIG)

dev-web:
	npm run dev --prefix web

verify-release:
	python3 scripts/verify-release.py $(RELEASE_DIR)

# Keep private data and historical binaries/backups under bin intact.
clean:
	rm -f bin/webcp
	rm -rf web/dist dist/releases
