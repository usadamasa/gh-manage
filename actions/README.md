# actions

他のリポジトリから呼ばれる composite action。

| action | 内容 |
| --- | --- |
| [setup-nix](setup-nix/README.md) | Nix を入れ、devShell を後続の step へ渡す |
| [tagpr](tagpr/README.md) | GitHub App のトークンを作り、tagpr を動かす |
| [verify-actions-lock](verify-actions-lock/README.md) | gh-actions-lock を入れ、ワークフローが `actions.lock` と一致するかを検証する |

```yaml
- uses: usadamasa/gh-manage/actions/<name>@main
```
