.PHONY: test build web-test dev-up

test:
	cd web && npm run build
	rm -rf internal/app/web/dist && mkdir -p internal/app/web && cp -r web/dist internal/app/web/dist
	go test ./...
	cd web && npm test

build:
	docker compose build

web-test:
	cd web && npm ci && npm run check && npm test && npm run build

dev-up:
	docker compose up --build -d --wait
