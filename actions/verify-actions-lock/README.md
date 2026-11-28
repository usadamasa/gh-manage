# verify-actions-lock

[gh-actions-lock](https://github.com/github/gh-actions-lock) を入れ、`gh actions-lock --verify` で
ワークフローの `uses:` が `.github/workflows/actions.lock` と一致するかを検証する。
拡張の版はこの action が固定する。版を上げると、schema 版が同じでも verify の判定が変わることがある。
呼ぶ側の `actions.lock` が落ちたら、`gh actions-lock --relock` で記録し直す。

## 使い方

`actions/checkout` の後に置く。入力は無い。

```yaml
jobs:
  actions-lock:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7.0.1
      - uses: usadamasa/gh-manage/actions/verify-actions-lock@main
```

## private な action に依存するリポジトリ

`GH_TOKEN` は `github.token` 固定なので、private な action に依存するリポジトリではこの action を使えない
(`github.token` は呼んだリポジトリの外にある private なリポジトリを読めない)。
そのリポジトリでは検証を workflow に直接書き、private な action を読める token (GitHub App token など) を
別のステップで作って `gh actions-lock --verify` の `GH_TOKEN` に渡す。
