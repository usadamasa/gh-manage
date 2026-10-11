#!/usr/bin/env bash
# Workbench (usadamasa の Project 5) に未登録の open な issue / PR を追加し､Kind を埋める｡
# Priority は空のまま残す (空 = 未振り分け)｡手順の背景は skill managing-github-projects の triage｡
#
# 環境変数:
#   GH_TOKEN  Project の読み書きができる token
#   DRY_RUN   1 なら追加せず対象だけ出す
set -euo pipefail

readonly OWNER=usadamasa
readonly PROJECT=5
# gh project に login を渡すと user と organization を同時に引き､read:org の無い token では
# "unknown owner type" で落ちる｡Project の持ち主は token の持ち主なので viewer で引く
readonly PROJECT_OWNER=@me
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
readonly REPOS_TSV="${script_dir}/repos.tsv"

for cmd in gh jq; do
  if ! command -v "$cmd" >/dev/null; then
    printf 'error: %s が見つからない\n' "$cmd" >&2
    exit 1
  fi
done

# repos.tsv から --repo 引数と repo -> Kind の対応を作る
repo_args=()
kinds='{}'
while IFS=$'\t' read -r repo kind; do
  [[ -z "$repo" || "$repo" == \#* ]] && continue
  repo_args+=(--repo "${OWNER}/${repo}")
  kinds=$(jq -c --arg r "${OWNER}/${repo}" --arg k "$kind" '. + {($r): $k}' <<<"$kinds")
done <"$REPOS_TSV"

# dependabot の PR はリポジトリ側で､tagpr の Release PR は release-check で扱うので載せない
items=$(gh search issues "${repo_args[@]}" --state open --include-prs --limit 500 \
  --json url,repository \
  -- "-project:${OWNER}/${PROJECT}" -author:app/dependabot -label:tagpr)
count=$(jq length <<<"$items")
printf '未登録: %s 件\n' "$count"
if [[ "$count" -eq 0 ]]; then
  exit 0
fi

if [[ "${DRY_RUN:-}" == 1 ]]; then
  jq -r '.[] | "\(.repository.nameWithOwner)\t\(.url)"' <<<"$items"
  exit 0
fi

project_id=$(gh project view "$PROJECT" --owner "$PROJECT_OWNER" --format json --jq .id)
kind_field=$(gh project field-list "$PROJECT" --owner "$PROJECT_OWNER" --format json \
  --jq '.fields[] | select(.name == "Kind")')
kind_field_id=$(jq -r .id <<<"$kind_field")
if [[ -z "$kind_field_id" ]]; then
  printf 'error: Project %s に Kind フィールドが無い\n' "$PROJECT" >&2
  exit 1
fi

failed=0
while IFS=$'\t' read -r repo url; do
  kind=$(jq -r --arg r "$repo" '.[$r] // empty' <<<"$kinds")
  option_id=$(jq -r --arg k "$kind" '.options[] | select(.name == $k) | .id' <<<"$kind_field")
  if [[ -z "$option_id" ]]; then
    printf 'error: %s の Kind %s が Project の選択肢に無い\n' "$repo" "$kind" >&2
    failed=1
    continue
  fi
  if ! item_id=$(gh project item-add "$PROJECT" --owner "$PROJECT_OWNER" --url "$url" --format json --jq .id); then
    printf 'error: 追加に失敗: %s\n' "$url" >&2
    failed=1
    continue
  fi
  if ! gh project item-edit --id "$item_id" --project-id "$project_id" \
    --field-id "$kind_field_id" --single-select-option-id "$option_id" >/dev/null; then
    printf 'error: Kind の設定に失敗: %s\n' "$url" >&2
    failed=1
    continue
  fi
  printf 'added\t%s\t%s\n' "$kind" "$url"
done < <(jq -r '.[] | [.repository.nameWithOwner, .url] | @tsv' <<<"$items")

exit "$failed"
