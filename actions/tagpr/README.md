# tagpr

GitHub App のトークンを作り、そのトークンで [tagpr](https://github.com/Songmu/tagpr) を動かす。
App トークンの権限は contents: write、pull-requests: write、issues: read に固定する。
`create-github-app-token` と `tagpr` の版はこの action が固定する。

## 入力

| 名前 | 内容 |
| --- | --- |
| `client-id` | GitHub App の Client ID。gh-manage が `TAGPR_CLIENT_ID` (variable) として配る |
| `private-key` | GitHub App の秘密鍵。gh-manage が `TAGPR_PRIVATE_KEY` (secret) として配る |

## 出力

tagpr の出力をそのまま返す。

| 名前 | 内容 |
| --- | --- |
| `tag` | tagpr がタグを打ったときの semver タグ。打たなかったときは空 |
| `pull_request` | tagpr が作った PR の情報 (JSON) |
| `base_tag` | 比較の基準にした semver タグ。前のタグが無いときは空 |

## 使い方

`actions/checkout` の後に置く。reusable workflow にしないのは、`tag` を見て同じ job で
リリースの step を続けられるようにするため。

```yaml
jobs:
  tagpr:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7.0.1
        with:
          persist-credentials: false
      - uses: usadamasa/gh-manage/actions/tagpr@main
        id: tagpr
        with:
          client-id: ${{ vars.TAGPR_CLIENT_ID }}
          private-key: ${{ secrets.TAGPR_PRIVATE_KEY }}
      - if: steps.tagpr.outputs.tag != ''
        run: goreleaser release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```
