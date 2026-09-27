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
- `prune` も同じ規則で解決する｡既定は false で､base → overlay の順に上書きする

schema に無いキーはエラーにする (typo を apply の前に止める)｡`repository.name` は rename を防ぐため受け付けない｡
合成結果は `gh-manage render [name]` で確認できる｡

```yaml
# settings/base.yaml
prune:                      # 宣言していないリソースを消すか｡既定 false (残す)
  rulesets: false
  variables: false
  secrets: false
  dependabot_secrets: false
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
variables:                  # Actions の variable｡値は平文で書く
  GO_VERSION: "1.24"
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
secrets:
  COPILOT_GITHUB_TOKEN:
    from_env: COPILOT_GITHUB_TOKEN
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
| `gh-manage apply [--yes]` | plan の結果を適用する |
| `gh-manage snapshot [--repo name] [--minimize]` | live を `settings/repos/<name>.yaml` に書き出す (管理下に入れるとき)｡既存のファイルは上書きする |

### snapshot

- `--repo` を省くと自分が owner の全リポジトリを書き出す｡archived と fork は `--include-archived` / `--include-forks` を付けたときだけ含める
- `--minimize` は base.yaml との差分だけを残す｡base にあって live に無いものは `null` で書くので､render すると live と同じになる
- secret は値を読めないので名前だけ書き､`from_env` に同じ名前を入れる
- 最後に rate limit の残り (`X-RateLimit-Remaining`) を stderr に出す

### 差分の見方

- 宣言したキーが live と一致していれば同じとみなす｡live 側にだけあるキー (server が埋める既定値など) は無視する
- ruleset の `rules` は `type` で対応付けて `parameters` を同じ規則で比べる｡list 全体を宣言するので､live にだけある type は差分になる
- 出力はリポジトリごとに `+ create` / `~ update (key: old -> new)` / `- delete` / `= no change`｡宣言されていないものは `!` の notice
- 宣言があって GitHub に無いリポジトリは create､archived のリポジトリは skip する
- secret は API が値を返さないので有無だけ比較する｡apply では宣言した secret を毎回暗号化して書き直す
- 宣言していない ruleset / variable / secret は､`prune` で true にした種別だけ削除し､それ以外は notice として表示する
- visibility を private から public に変える apply は `--allow-publish` を付けないと拒否する

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
