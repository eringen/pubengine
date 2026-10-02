TEMPL := go run github.com/a-h/templ/cmd/templ@v0.3.1020
SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0

.PHONY: generate test check-generated bench

generate:
	$(TEMPL) generate -path analytics/templates
	$(SQLC) generate -f analytics/sqlcgen/sqlc.yaml

check-generated: generate
	npm run check:talkdom
	git diff --exit-code -- analytics/templates/*_templ.go analytics/sqlcgen

test:
	go test -race ./...
	go vet ./...
	npm test

bench:
	go test ./... -run '^$$' -bench . -benchmem
