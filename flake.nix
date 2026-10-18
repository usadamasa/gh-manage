{
  description = "gh-manage の開発環境 (Go と lint・CI のツール)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    # tag は付け替えられるので、固定は flake.lock の commit に任せる
    go-arch-metrics.url = "github:usadamasa/go-arch-metrics";
  };

  outputs =
    { nixpkgs, go-arch-metrics, ... }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system nixpkgs.legacyPackages.${system});
    in
    {
      devShells = forAllSystems (
        system: pkgs: {
          default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              # go.mod の go 行と同じ版。ずれたら GOTOOLCHAIN=local で即座に失敗させる
              go_1_27
              go-task
              gotools
              git
              golangci-lint
              go-arch-lint
              gosec
              govulncheck
              yamllint
              check-jsonschema
              # analyze-modularity、analyze-arch-lint、vulncheck。実行時の go・git・go-arch-lint は上の PATH から使う
              go-arch-metrics.packages.${system}.default
            ];
            env.GOTOOLCHAIN = "local";
          };
        }
      );
    };
}
