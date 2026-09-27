# fine-grained PAT の作り方

gh-manage は fine-grained personal access token (PAT) 1 本で動く｡GitHub App は要らない｡
リポジトリの作成 (`POST /user/repos`) は GitHub App の installation token では呼べず、fine-grained PAT の
"Repository creation" 権限が必要なため｡

## 作成手順

1. <https://github.com/settings/personal-access-tokens/new> を開く
2. 次のとおり設定する

| 項目 | 値 |
| --- | --- |
| Token name | `gh-manage` |
| Expiration | 任意 (期限切れの前に作り直す) |
| Resource owner | `usadamasa` (個人アカウント) |
| Repository access | **All repositories** (新しく作るリポジトリも対象に入れるため) |

3. Repository permissions を次のとおり付ける

| Permission | Access | 使う場所 |
| --- | --- | --- |
| Administration | Read and write | 一般設定の PATCH、ruleset の作成と更新 |
| Metadata | Read-only | (自動で付く) |
| Actions | Read and write | Actions の variable |
| Secrets | Read and write | Actions の secret |
| Variables | Read and write | Actions の variable |
| Dependabot secrets | Read and write | Dependabot の secret |
| Repository creation | Read and write | `POST /user/repos` (無ければ Administration だけで作成以外は動く) |

4. Generate token を押し、値を控える (再表示できない)

## ローカルで使う

```bash
export GH_TOKEN=github_pat_...
gh-manage plan
```

`GH_TOKEN` が無ければ `gh auth token` の値を使う｡`gh auth login` の classic token (`repo` scope) でも
リポジトリ作成以外は動く｡

1Password に置くなら `op run` で注入する:

```bash
op run --env-file=.env.op -- gh-manage plan
```

## GitHub Actions で使う

gh-manage リポジトリの Actions secret に置く｡

| Secret | 内容 |
| --- | --- |
| `GH_MANAGE_TOKEN` | 上で作った fine-grained PAT |
| `TAGPR_PRIVATE_KEY` | 配布する secret の値 (settings の `from_env: TAGPR_PRIVATE_KEY` が参照する) |
| `COPILOT_GITHUB_TOKEN` | 同上 |

```bash
gh secret set GH_MANAGE_TOKEN --repo usadamasa/gh-manage
```

配布する secret の名前を settings に足したら、同じ名前で gh-manage の Actions secret にも値を置く｡
workflow は secret を同名の環境変数として gh-manage に渡す｡
