# gh-manage

自分が保有する GitHub リポジトリの設定を､宣言的な YAML から構成管理する stateless な reconciler｡
Terraform のような state ファイルは持たず､毎回 GitHub API から live の設定を読んで差分を出し､変わった分だけ適用する｡

## 管理するもの

| 対象 | 内容 |
| --- | --- |
| 一般設定 | delete_branch_on_merge､wiki / projects / issues の有効・無効､merge 方式､visibility､description､topics |
| ruleset | default branch の保護 (削除・force push の禁止､PR 必須､required status checks) |
| variables / secrets | Actions の variable と secret､Dependabot の secret を指定したリポジトリへ配布 |
| リポジトリ作成 | 宣言があって GitHub に無いリポジトリを作る |

削除と archive は行わない｡

## 設定ファイル

```text
settings/
  base.yaml          # 全リポジトリ共通
  repos/<name>.yaml  # リポジトリごとの差分｡ファイルがある = 管理対象
```

overlay は base に deep merge する｡

- map は再帰的に merge する
- `null` を書くと base のキーを削除する (`secrets: {FOO: null}` で base の secret FOO を外す)
- list は丸ごと置き換える｡ruleset の `rules` に 1 つ足すときも､overlay で list 全体を書き直す
- list の要素の中の `null` は削除でなく値として残す (`code_coverage` の `minimum_coverage: null` など)
- `prune` も同じ規則で解決する｡既定は false で､base → overlay の順に上書きする
- `managed_topic` の topic は plan が desired の topics に足す (`topics` を宣言していないリポジトリは live の topics に足す)｡
  `render` と snapshot の overlay には出ない

schema に無いキーはエラーにする (typo を apply の前に止める)｡`repository.name` は rename を防ぐため受け付けない｡
合成結果は `gh-manage render [name]` で確認できる｡

```yaml
# settings/base.yaml
prune:                      # 宣言していないリソースを消すか｡既定 false (残す)
  rulesets: false
  variables: false
  secrets: false
  dependabot_secrets: false
managed_topic: gh-managed   # 管理下のリポジトリに必ず付ける topic｡GitHub 側で管理対象を見分けるための印
repository:                 # PATCH /repos/{owner}/{repo} のフィールドをそのまま書く
  visibility: public
  has_wiki: false
  has_projects: false
  delete_branch_on_merge: true
rulesets:                   # キーが ruleset の名前｡名前で upsert する
  main:
    target: branch
    enforcement: active
    conditions:
      ref_name:
        include: ["~DEFAULT_BRANCH"]
        exclude: []
    rules:                  # REST API の rules[] をそのまま書く
      - type: deletion
      - type: non_fast_forward
topics: [go, cli]           # PUT /repos/{owner}/{repo}/topics で丸ごと置き換える
variables:                  # Actions の variable｡値は平文か from_env (plan / apply のときに環境変数から読む) で書く
  GO_VERSION: "1.24"
  TAGPR_CLIENT_ID:
    from_env: TAGPR_CLIENT_ID
secrets:                    # 値は環境変数から読む｡受け付けるのは from_env だけで､平文の値は schema 検証で弾く
  TAGPR_PRIVATE_KEY:
    from_env: TAGPR_PRIVATE_KEY
dependabot_secrets:         # Dependabot の secret｡書き方は secrets と同じ
  NPM_TOKEN:
    from_env: NPM_TOKEN
```

```yaml
# settings/repos/agents-config.yaml
repository:
  visibility: private
```

## インストール

```bash
go install github.com/usadamasa/gh-manage/cmd/gh-manage@latest
```

## コマンド

| コマンド | 役割 |
| --- | --- |
| `gh-manage render [name]` | base と overlay を合成した desired state を表示する｡name を省くと全リポジトリを名前の順に出す |
| `gh-manage plan [--format text\|json]` | live を読んで差分を出す｡exit 0 = 差分なし､2 = 差分あり､1 = エラー |
| `gh-manage apply [--yes] [--allow-publish]` | plan を表示し､確認のうえ適用する｡`--yes` で確認を省く (CI 用) |
| `gh-manage snapshot [--repo name] [--minimize]` | live を `settings/repos/<name>.yaml` に書き出す (管理下に入れるとき)｡既存のファイルは上書きする |

各コマンドの細かい挙動と plan の差分の読み方は skill
[running-gh-manage](.claude/skills/running-gh-manage/SKILL.md) にある｡

## 認証

fine-grained PAT 1 本で動く｡作り方は [docs/setup-token.md](docs/setup-token.md)｡

- ローカル: `GH_TOKEN` 環境変数か `gh auth token`
- GitHub Actions: secret `GH_MANAGE_TOKEN`

## 運用 (GitOps)

| トリガー | 動作 |
| --- | --- |
| `settings/**` を変える PR | `plan` を実行し､差分を PR に出す |
| main への push | `apply` を実行する |
| 毎週 | `plan` を実行し､手で変えた drift があれば失敗する |

workflow は `.github/workflows/settings.yaml`｡secret の置き場所と job の詳細は [docs/setup-token.md](docs/setup-token.md#github-actions-で使う)｡

別の workflow `.github/workflows/workbench.yaml` が､毎時 Workbench (Project 5) に未登録の issue / PR を追加する｡
詳細は [docs/setup-token.md](docs/setup-token.md#workbench-への自動追加)｡

## 開発

### 前提

- Go 1.27 以上
- [aqua](https://aquaproj.github.io/) (ツールのバージョン固定)
- [direnv](https://direnv.net/) (任意｡`.envrc` が PATH と aqua の policy を通す)

### セットアップ

```bash
aqua install
direnv allow
```

### コマンド

```bash
task build    # bin/gh-manage をビルド
task test     # テスト
task lint     # yamllint, golangci-lint, go-arch-lint, analyze-*, govulncheck, gosec
task metrics  # spm-go でパッケージメトリクスを表示
task format   # go mod tidy, goimports, go fmt
task ci       # format + lint + test + build
```

## なぜ自作か

stateless で個人アカウントに対応し､一般設定・ruleset・secret を扱えて､base + overlay で書ける OSS が無かった (2026-09 時点)｡

| 候補 | 見送った理由 |
| --- | --- |
| Terraform / OpenTofu / Pulumi | state ファイルを持つ｡secret が state に平文で残る |
| github/safe-settings, eclipse-csi/otterdog | organization 専用 |
| repository-settings/app (Probot Settings) | ruleset と secret を扱えない |
| noirbizarre/gh-settings | secret を扱えない |
| Vivswan/github-settings-as-code | 機能は揃うが OSI 準拠でないライセンス､開始 2 か月・メンテナー 1 人 (動作は試していない) |

## License

MIT License - see [LICENSE](LICENSE) for details.
