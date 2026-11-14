#!/bin/bash
# PR が閉じる issue (closingIssuesReferences) の種類と area のラベルを PR へ写す。
# .github/workflows/sync-pr-labels.yaml (reusable workflow) から呼ばれる。
#
# usage: sync-pr-labels.sh <PR の番号>
# env:   COPY_PATTERN  写すラベルの正規表現 (jq の test)。空なら種類と area
#
# release note の category は PR のラベルで決まる。epic と priority は
# issue の管理にだけ使うので写さない。足すだけで、PR から外すことはしない。
set -euo pipefail

if [ $# -ne 1 ] || ! [[ "$1" =~ ^[0-9]+$ ]]; then
  printf 'usage: %s <PR の番号>\n' "$0" >&2
  exit 1
fi
readonly PR="$1"

readonly DEFAULT_COPY_PATTERN='^(bug|enhancement|documentation|refactor|investigation|area: .+)$'
COPY_PATTERN="${COPY_PATTERN:-$DEFAULT_COPY_PATTERN}"
readonly COPY_PATTERN

pr_json=$(gh pr view "$PR" --json url,labels,closingIssuesReferences)
readonly pr_json

# PR の URL から repo の URL を取り、同じ repo の issue だけを読む (別 repo のラベルはこの repo に無い)
repo_url=$(jq -r '.url | sub("/pull/[0-9]+$"; "")' <<<"$pr_json")
readonly repo_url

issue_urls=$(jq -r --arg prefix "$repo_url/issues/" \
  '.closingIssuesReferences[].url | strings | select(startswith($prefix))' <<<"$pr_json")
readonly issue_urls

wanted=""
while IFS= read -r url; do
  [ -n "$url" ] || continue
  issue_json=$(gh issue view "$url" --json labels)
  wanted+="$(jq -r '.labels[].name' <<<"$issue_json")"$'\n'
done <<<"$issue_urls"

current=$(jq -r '.labels[].name' <<<"$pr_json")
readonly current

to_add=$(jq -nRr --arg pattern "$COPY_PATTERN" --arg current "$current" '
  [inputs | select(test($pattern))] | unique
  - ($current | split("\n"))
  | join(",")' <<<"$wanted")
readonly to_add

if [ -z "$to_add" ]; then
  printf 'sync-pr-labels: PR #%s に足すラベルは無い\n' "$PR" >&2
  exit 0
fi

gh pr edit "$PR" --add-label "$to_add"
printf 'sync-pr-labels: PR #%s に %s を足した\n' "$PR" "$to_add" >&2
