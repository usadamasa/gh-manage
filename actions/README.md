# actions

他のリポジトリから呼ばれる composite action。gh-manage 自身だけが使う action は `.github/actions/` に置く。

| action | 内容 |
| --- | --- |
| [setup-nix](setup-nix/README.md) | Nix を入れ、devShell を後続の step へ渡す |

```yaml
- uses: usadamasa/gh-manage/actions/<name>@main
```
