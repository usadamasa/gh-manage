---
name: running-gh-manage
description: Use when running gh-manage snapshot or apply, reading what a plan's diff means, or deciding why a key shows up (or does not) in plan output. Also when a secret, variable, or ruleset looks deleted or unchanged in plan and the reason is unclear.
---

# gh-manage を動かす

gh-manage は desired (settings/) と live (GitHub API) を毎回比べる stateless な reconciler。
各コマンドの挙動と plan の読み方は、コードではなくここに置く。README はコマンドの一覧だけを持つ。

## snapshot

`gh-manage snapshot [--repo name] [--minimize]` は live を `settings/repos/<name>.yaml` に書き出す (管理下に入れるとき)。
既存のファイルは上書きする。

- `--repo` を省くと自分が owner の全リポジトリを書き出す。archived と fork は `--include-archived` /
  `--include-forks` を付けたときだけ含める
- `--minimize` は base.yaml との差分だけを残す。base にあって live に無いものは `null` で書くので、render すると live と同じになる
- secret は値を読めないので名前だけ書き、`from_env` に同じ名前を入れる
- variable は live の値を平文で書く。環境変数から読ませたいものは手で `from_env` に書き換える
- 最後に rate limit の残り (`X-RateLimit-Remaining`) を stderr に出す

## apply

`gh-manage apply [--yes] [--allow-publish]` は plan を表示し、確認のうえ適用する。`--yes` で確認を省く (CI 用)。

- repository は変わったキーだけを `PATCH` する。宣言していないキーは送らない
- ruleset は名前で探して、あれば `PUT`、無ければ `POST` で宣言全体を書く。削除は `prune.rulesets` のときだけ
- 宣言があって GitHub に無いリポジトリは `POST /user/repos` (`auto_init: true`) で作り、続けて宣言した設定をすべて入れる
- variable は無ければ `POST`、値が違えば `PATCH` する。削除は `prune.variables` のときだけ
- `from_env` の variable は plan の時点で環境変数から値を読む (値を比べるため)。無い (空も含む) と plan も apply も止まる
- secret (Actions / Dependabot) は宣言したものを差分の有無にかかわらず毎回書き直す。
  リポジトリの公開鍵を取得し、`from_env` の環境変数の値を sealed box (`golang.org/x/crypto/nacl/box`) で封緘して `PUT` する。
  値は plan にも出力にも出さず、書き直す secret の名前だけを確認の前に表示する。
  削除は `prune.secrets` / `prune.dependabot_secrets` のときだけ
- `from_env` の環境変数が 1 つでも無い (空も含む) と、何も書き込まずに止める
- private を public にする変更が 1 件でもあれば、`--allow-publish` が無い限り何も書き込まずに止める

## plan の差分の見方

`gh-manage plan [--format text|json]` は live を読んで差分を出す。exit 0 = 差分なし、2 = 差分あり、1 = エラー。

- 宣言したキーが live と一致していれば同じとみなす。live 側にだけあるキー (server が埋める既定値など) は無視する
- ruleset の `rules` は `type` で対応付けて `parameters` を同じ規則で比べる。
  list 全体を宣言するので、live にだけある type は差分になる
- list / map の update は YAML の diff で出す。old 側 (`--format json` の `old` も) は判定に使ったキーだけに絞るので、
  live にだけあるキーは `-` 行に出ない
- 出力はリポジトリごとに `+ create` / `~ update (key: old -> new)` / `- delete` / `= no change`。
  宣言されていないものは `!` の notice
- 宣言があって GitHub に無いリポジトリは create、archived のリポジトリは skip する
- secret は API が値を返さないので有無だけ比較する。apply では宣言した secret を毎回暗号化して書き直す
- 宣言していない ruleset / variable / secret は、`prune` で true にした種別だけ削除し、それ以外は notice として表示する
- visibility を private から public に変える apply は `--allow-publish` を付けないと拒否する

## よくある誤読

| 見え方 | 実際 |
| ---- | ---- |
| plan に出ないキーが live にある | desired に書いていないキーは比べない。管理したいなら desired に書く |
| secret が毎回「変更なし」なのに apply で書き直される | 値を比べられないので、宣言した secret は常に書き直す仕様 |
| 宣言していない variable が消えない | `prune.variables` が false。notice (`!`) に出るだけで削除しない |
| `rules` の diff の並びが live と違う | 判定は type で突き合わせるので並び順は差分にならない。表示では old 側を desired の順に並べ、desired に無い type を末尾に回す |
