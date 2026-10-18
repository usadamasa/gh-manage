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
          go = pkgs.go_1_27;
          buildGoModule = pkgs.buildGoModule.override { inherit go; };
        in
        {
          go-arch-metrics = buildGoModule {
            pname = "go-arch-metrics";
            version = "2026.1018.1";
            src = pkgs.fetchFromGitHub {
              owner = "usadamasa";
              repo = "go-arch-metrics";
              # 2026.1018.1 の tag の commit。tag は付け替えられるので commit で固定する
              rev = "bae94cae339b9e0be5d5958ab09c8533c7376767";
              hash = "sha256-QvqTMBZK4cIihrQ6FINL6Cbp/vULsvGp1j9htTKsRtQ=";
            };
            vendorHash = "sha256-lAik/wRbmvSPkZe8XiXUCUp13BQiD97yWy57fvaF0H0=";
            # analyze-modularity は git が ignore するパッケージを git で調べる。テストも git を呼ぶ
            nativeCheckInputs = [ pkgs.git ];
            subPackages = [
              "cmd/analyze-arch-lint"
              "cmd/analyze-modularity"
              "cmd/vulncheck"
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
              check-jsonschema
              tools.go-arch-metrics
            ];
            env.GOTOOLCHAIN = "local";
          };
        }
      );
    };
}
