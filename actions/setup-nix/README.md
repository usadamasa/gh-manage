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
key は `nix-<runner.os>-<flake.lock のハッシュ>` で、`flake.lock` が変わるまで同じ cache を使う。
key が一致しないときは同じ OS の直近の cache を前方一致で戻し、足りない分だけ取得する。
保存は job が成功して終わるときに、key と一致する cache が無い場合だけ行う。
保存前の GC で store を 2 GiB までに抑え、devShell は GC root にして残す。
GitHub の cache は同じ ref・base ref・default branch の分しか読めないので、
default branch で一度も走っていない key は、PR の最初の run で miss になる (同じ PR の 2 回目からは hit する)。
古い cache の purge はしない (`actions: write` が要る)。容量を超えた分は GitHub が古い順に消す。

呼ぶ側が `actions.lock` (gh-actions-lock) で `usadamasa/gh-manage@main` を固定しているなら、
ランナーは記録した commit を動かすので、`gh actions-lock` で lockfile を作り直すまでこの変更は効かない。
作り直した lockfile では、`usadamasa/gh-manage@main` の `uses:` に
`nix-community/cache-nix-action` が載っていることを確かめる。
