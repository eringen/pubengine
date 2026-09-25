#!/bin/sh
set -eu
go test ./...
node scripts/talkdom_test.cjs
node scripts/analytics_test.cjs
