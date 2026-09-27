# fine-grained PAT の作り方

gh-manage は fine-grained personal access token (PAT) 1 本で動く｡GitHub App は要らない｡
リポジトリの作成 (`POST /user/repos`) も fine-grained PAT で呼べる｡作成の権限は独立した項目ではなく
Administration に含まれる (作成画面の説明は "Repository creation, deletion, settings, teams, and collaborators"､2026-09 に確認)｡GitHub App の installation token での作成は
評価していない｡

## 作成手順

<https://github.com/settings/personal-access-tokens/new> を開き､次のとおり設定する｡

| 項目 | 値 |
| --- | --- |
| Token name | `gh-manage` |
| Expiration | 任意 (期限切れの前に作り直す) |
| Resource owner | `usadamasa` (個人アカウント) |
| Repository access | **All repositories** (新しく作るリポジトリも対象に入れるため) |

Repository permissions は次のとおり付ける｡

| Permission | Access | 使う場所 |
| --- | --- | --- |
| Administration | Read and write | リポジトリの作成 (`POST /user/repos`)､一般設定の PATCH､ruleset の作成と更新 |
| Metadata | Read-only | (自動で付く) |
| Secrets | Read and write | Actions の secret |
| Variables | Read and write | Actions の variable |
| Dependabot secrets | Read and write | Dependabot の secret |

Generate token を押し､値を控える (再表示できない)｡

## ローカルで使う

```bash
export GH_TOKEN=github_pat_...
gh-manage plan
```

`GH_TOKEN` が無ければ `gh auth token` の値を使う｡`gh auth login` で得た OAuth token (`repo` scope) でも
リポジトリ作成以外は動く｡

1Password に置くなら､`op run` で注入する｡`.env.op` には `GH_TOKEN=op://<vault>/<item>/credential` のように
1Password の参照を書く｡

```bash
op run --env-file=.env.op -- gh-manage plan
```

## GitHub Actions で使う

gh-manage リポジトリの Actions secret に置く｡

| Secret | 内容 |
| --- | --- |
| `GH_MANAGE_TOKEN` | 上で作った fine-grained PAT |
| `TAGPR_PRIVATE_KEY` | 配布する secret の値 (settings の `from_env: TAGPR_PRIVATE_KEY` が参照する) |
| `COPILOT_GITHUB_TOKEN` | 配布する secret の値 (settings の `from_env: COPILOT_GITHUB_TOKEN` が参照する) |

```bash
gh secret set GH_MANAGE_TOKEN --repo usadamasa/gh-manage
```

配布する secret の名前を settings に足したら､同じ名前で gh-manage の Actions secret にも値を置く｡
workflow は secret を同名の環境変数として gh-manage に渡す｡

`from_env` で書いた variable の値は gh-manage の Actions variable に置く｡

| Variable | 内容 |
| --- | --- |
| `TAGPR_CLIENT_ID` | 配布する variable の値 (settings の `from_env: TAGPR_CLIENT_ID` が参照する) |

```bash
gh variable set TAGPR_CLIENT_ID --repo usadamasa/gh-manage
```

variable は plan で値を比べるので､workflow は Plan / Apply / Drift のすべてに渡す｡

workflow は `.github/workflows/settings.yaml`｡

| トリガー | job | 使う secret |
| --- | --- | --- |
| `settings/**` を変える PR | Plan: plan を job summary と PR コメント (1 件を上書き) に出す｡差分 (exit 2) は成功扱い | `GH_MANAGE_TOKEN` |
| main への push | Apply: `apply --yes` | `GH_MANAGE_TOKEN` と配布する secret |
| 毎週月曜 09:00 JST と手動実行 | Drift: plan｡差分 (exit 2) があれば失敗する | `GH_MANAGE_TOKEN` |

- `settings/base.yaml` が無い間 (bootstrap 前) は､どの job も何もせずに成功する
- fork からの PR には secret が渡らないので Plan を走らせない
- PR コメントは workflow の `GITHUB_TOKEN` で書く (PAT は使わない)
- 配布する secret を足したら､workflow の Apply の `env` にも同じ名前で足す
- `from_env` の variable を足したら､workflow の Plan / Apply / Drift の `env` に `${{ vars.<名前> }}` で足す
