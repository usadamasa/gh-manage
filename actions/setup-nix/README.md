# setup-nix

Nix を入れ、リポジトリの `flake.nix` の default devShell を読み込む。
devShell の PATH と env を `GITHUB_PATH` / `GITHUB_ENV` に書くので、以後の step はツールを素で呼べる。
ツールの版は呼ぶ側の `flake.lock` で決まる。

## 使い方

`actions/checkout` の後に置く。入力は無い。

```yaml
steps:
  - uses: actions/checkout@v7.0.1
  - uses: usadamasa/gh-manage/actions/setup-nix@main
  - run: task test
```
