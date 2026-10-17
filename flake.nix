{
  description = "gh-manage の開発環境 (Go と lint・CI のツール)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      # nixpkgs に無いツール。go.mod の go 行より古い Go ではビルドできないので devShell と同じ Go を使う
      extraTools =
        pkgs:
        let
          buildGoModule = pkgs.buildGoModule.override { go = pkgs.go_1_27; };
        in
        {
          spm-go = buildGoModule rec {
            pname = "spm-go";
            version = "0.11.1";
            src = pkgs.fetchFromGitHub {
              owner = "fdaines";
              repo = "spm-go";
              rev = "v${version}";
              hash = "sha256-lg6UX8N7Rys4wEUEdKmNoLzrURcqjS/9eyIKCMNFW3A=";
            };
            vendorHash = "sha256-EuHp0OPyVln/ytK9Vav/5U7EsUc80g3LVWGMqdpY3dg=";
          };

          go-arch-metrics = buildGoModule rec {
            pname = "go-arch-metrics";
            version = "2026.0919.3";
            src = pkgs.fetchFromGitHub {
              owner = "usadamasa";
              repo = "go-arch-metrics";
              rev = version;
              hash = "sha256-VXmnHI7sm4SYiQihzppyoMvBXKOUuqQJJaBhMSfGf4k=";
            };
            vendorHash = "sha256-g+yaVIx4jxpAQ/+WrGKxhVeliYx7nLQe/zsGpxV4Fn4=";
            subPackages = [
              "cmd/analyze-arch-lint"
              "cmd/analyze-modularity"
            ];
          };
        };
    in
    {
      packages = forAllSystems extraTools;

      devShells = forAllSystems (
        pkgs:
        let
          tools = extraTools pkgs;
        in
        {
          default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              # go.mod の go 行と同じ版。ずれたら GOTOOLCHAIN=local で即座に失敗させる
              go_1_27
              go-task
              gotools
              golangci-lint
              go-arch-lint
              gosec
              govulncheck
              yamllint
              tools.spm-go
              tools.go-arch-metrics
            ];
            env.GOTOOLCHAIN = "local";
          };
        }
      );
    };
}
