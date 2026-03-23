#!/usr/bin/env bash

set -euo pipefail

docker compose -f deploy/local/docker-compose.yml up -d
