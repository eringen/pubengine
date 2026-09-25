TEMPL := go run github.com/a-h/templ/cmd/templ@v0.3.960

.PHONY: generate test check-generated bench

generate:
	$(TEMPL) generate -path analytics/templates

check-generated: generate
	git diff --exit-code -- analytics/templates/*_templ.go

test:
	go test -race ./...
	go vet ./...
	npm test

bench:
	go test ./... -run '^$$' -bench . -benchmem
