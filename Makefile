TAILWINDCSS := node_modules/.bin/tailwindcss
ESBUILD := node_modules/.bin/esbuild
TAILWIND_INPUT := assets/tailwind.css
TAILWIND_OUTPUT := public/tailwind.css
TEMPL := $(shell go env GOPATH)/bin/templ
AIR := $(shell go env GOPATH)/bin/air

JS_SRC := assets/app.js

.PHONY: css css-prod js js-prod templ run prod test build-linux

css: $(TAILWIND_OUTPUT)

$(TAILWIND_OUTPUT): $(TAILWIND_INPUT) tailwind.config.js package-lock.json | $(TAILWINDCSS)
	$(TAILWINDCSS) -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT) --minify

css-prod: $(TAILWIND_INPUT) tailwind.config.js package-lock.json | $(TAILWINDCSS)
	NODE_ENV=production $(TAILWINDCSS) -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT) --minify

js: | $(ESBUILD)
	$(ESBUILD) $(JS_SRC) --outdir=public --minify

js-prod: | $(ESBUILD)
	$(ESBUILD) $(JS_SRC) --outdir=public --minify

$(TAILWINDCSS) $(ESBUILD):
	npm install

templ:
	$(TEMPL) generate

run: | $(AIR)
	ADMIN_PASSWORD=dev-only ADMIN_SESSION_SECRET=dev-secret-change-in-prod $(AIR)

$(AIR):
	go install github.com/air-verse/air@latest

prod: templ css-prod js-prod
	ADMIN_PASSWORD=dev-only ADMIN_SESSION_SECRET=dev-secret-change-in-prod go run ./...

test:
	go test ./...

build-linux: templ css-prod js-prod
	GOOS=linux GOARCH=amd64 go build -o pubengine .
