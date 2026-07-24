#!/bin/sh
# Includes temporary scaffold generation and compilation when templ is installed.
set -eu
go test ./...
node scripts/analytics_test.cjs
