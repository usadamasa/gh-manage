# verify-actions-lock

[gh-actions-lock](https://github.com/github/gh-actions-lock) を入れ、`gh actions-lock --verify` で
ワークフローの `uses:` が `.github/workflows/actions.lock` と一致するかを検証する。
拡張の版はこの action が固定する。版を上げると lockfile の schema 版が変わることがあるので、
呼ぶ側では `gh actions-lock` で `actions.lock` を作り直す。

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
