# CLAUDE.md

gh-manage は､自分 (usadamasa) が保有する GitHub リポジトリの設定を宣言的に管理する stateless な reconciler｡
設計の背景と決定事項は README.md､作業中の計画は PLAN.md (gitignore 対象) にある｡

## 構成

```
.
├── cmd/gh-manage/      # エントリポイント (main)
├── internal/
│   ├── cli/            # cobra のコマンド定義
│   ├── config/         # settings/ の読み込み､schema 検証､base + overlay の合成､snapshot の差分
│   ├── github/         # GitHub REST API クライアント (go-gh)
│   ├── log/            # 標準出力への出力 (forbidigo 対応)
│   └── version/        # バージョン解決
├── settings/           # 管理対象の宣言 (base.yaml + repos/<name>.yaml)｡step 3 の snapshot で作る
├── docs/               # セットアップ手順
└── Taskfile.yaml
```

## コマンド

```bash
task build    # bin/gh-manage をビルド
task test     # テスト (race + cover)
task lint     # yamllint, golangci-lint, go-arch-lint, analyze-*, govulncheck, gosec
task metrics  # spm-go でパッケージメトリクスを表示
task format   # go mod tidy, goimports, go fmt
task ci       # format + lint + test + build
```

ツールは aqua で固定する (`aqua install`)｡`.envrc` が PATH と `AQUA_POLICY_CONFIG` を通す｡

## コード規約

- 標準出力への出力は `internal/log` の `Logger` を使う (`log.Default.Printf` など｡`fmt.Print*` は forbidigo が止める)｡
  例外は `cmd/`､`internal/cli/`､`internal/log/`､テスト
- GitHub API の呼び出しは `internal/github` に閉じる｡他のパッケージからの go-gh の import は depguard が止める
- パッケージの依存方向は `.go-arch-lint.yml` で宣言する｡新しいパッケージを足したら component も足す
- しきい値 (認知的複雑度 20､関数 100 行､nestif の複雑度 5､保守性指数 20) は `.golangci.yml`
- テストは table-driven を基本にする｡`httptest.NewServer` は sandbox で bind できないので使わない｡
  HTTP は `http.RoundTripper` を注入してモックする

## 進め方

- 1 issue = 1 PR｡PR は Draft で作り assignee を usadamasa にする
- TDD: テストを書いて失敗を確認してから実装する
- 完了報告の前に `task ci` を通す
