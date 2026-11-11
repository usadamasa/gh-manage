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

## Nix store の cache

`nix-community/cache-nix-action` で `/nix/store` を GitHub Actions の cache に載せ、
devShell の closure を毎 job 落とさないようにする。
key は `nix-<runner.os>-<flake.lock と flake.nix のハッシュ>` で、どちらかが変わるまで同じ cache を使う。
key が一致しないときは同じ OS の直近の cache を前方一致で戻し、足りない分だけ取得する。
保存前の GC で store を 2 GiB までに抑え、devShell は GC root にして残す。

保存するのは default branch の run (`schedule` を含む) が成功したときだけで、PR と `merge_group` の run は戻すだけ。
GitHub の cache は保存した ref と default branch の分しか読めず、PR の ref に保存しても他の PR からは使えない。

### default branch で cache を作る

PR の CI だけでは保存されないので、各リポジトリに setup-nix だけを回す workflow を置く。

```yaml
# .github/workflows/nix-cache.yaml
name: Nix cache

"on":
  push:
    branches:
      - main
  schedule:
    - cron: "0 0 * * 1"
  workflow_dispatch:

concurrency:
  group: ${{ github.workflow }}
  cancel-in-progress: false

permissions:
  contents: read

jobs:
  save:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    steps:
      - uses: actions/checkout@v7.0.1
        with:
          persist-credentials: false
      - uses: usadamasa/gh-manage/actions/setup-nix@main
```

- CI の `on` に `push` を足す形にしない。CI の `cancel-in-progress` で続けてマージした run が止まると、
  post step の保存も止まる。main でテストと lint を回し直す分も無駄になる
- `paths` で `flake.*` に絞らない。7 日アクセスの無い cache は GitHub が消し、PR はもう保存しないので、
  消えた後は次の push か週次の `schedule` で作り直す。key が既にあれば保存は空振りで済む
- `runs-on` は CI の job と同じ OS にする (key に `runner.os` が入る)
- merge queue を使うリポジトリも追加の設定は要らない。`merge_group` の run は default branch の cache を戻し、保存しない。
  queue からのマージで `push` が起きて、この workflow が走る
- setup-nix の参照を新しい commit へ進める (`gh actions-lock --relock`) 変更に、この workflow も入れる。
  片方だけだと PR の run が保存しなくなり、cache がどこにも作られない

PR の ref に保存された古い cache は、7 日アクセスが無ければ GitHub が消すので放っておく。
先に消すなら `gh cache delete --all --ref refs/pull/<N>/merge` で PR ごとに消す。
