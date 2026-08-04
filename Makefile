.PHONY: run build test test-cover up down logs loadtest mint-token

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test ./... -race

test-cover:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out

up:
	docker compose up --build

down:
	docker compose down -v

logs:
	docker compose logs -f

# Mint a local test JWT: make mint-token MUSER=u1 MORG=o1
mint-token:
	go run ./cmd/mint-token -user=$(or $(MUSER),test-user-1) -org=$(or $(MORG),test-org-1)

# Run the load test tool against a locally running instance:
#   make loadtest CONNS=1000 MESSAGES=50 TOKEN=<jwt>
loadtest:
	go run ./cmd/loadtest -conns=$(or $(CONNS),100) -messages=$(or $(MESSAGES),20) -token=$(TOKEN)
