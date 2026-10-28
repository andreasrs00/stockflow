#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
docker compose up -d db
docker compose --profile test run --build --rm backend-test
docker compose --profile test run --build --rm frontend-test
