#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

expected_image='    image: "${SUB2API_IMAGE:-ghcr.io/2464693633/sub2api}:${SUB2API_IMAGE_TAG:-latest}"'
expected_repo='      - UPDATE_GITHUB_REPO=${UPDATE_GITHUB_REPO:-2464693633/sub2api}'

for compose_file in \
  deploy/docker-compose.yml \
  deploy/docker-compose.local.yml \
  deploy/docker-compose.standalone.yml
do
  test "$(grep -Fxc "$expected_image" "$compose_file")" -eq 1
  test "$(grep -Fxc "$expected_repo" "$compose_file")" -eq 1
done

grep -Fxq 'SUB2API_IMAGE=ghcr.io/2464693633/sub2api' deploy/.env.example
grep -Fxq 'SUB2API_IMAGE_TAG=latest' deploy/.env.example
grep -Fxq 'UPDATE_GITHUB_REPO=2464693633/sub2api' deploy/.env.example
grep -Fq 'GITHUB_REPO="${GITHUB_REPO:-2464693633/sub2api}"' deploy/install.sh
grep -Fq 'GITHUB_REPO="${GITHUB_REPO:-2464693633/sub2api}"' deploy/docker-deploy.sh
grep -Fq 'UpdateRepository = "2464693633/sub2api"' backend/cmd/server/main.go
grep -Fq -- '-X main.UpdateRepository={{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}' .goreleaser.yaml
grep -Fq 'chown -R sub2api:sub2api /app' Dockerfile

printf 'fork release configuration test passed\n'
