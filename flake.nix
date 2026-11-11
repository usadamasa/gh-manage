{
  description = "gh-manage の開発環境 (Go と lint・CI のツール)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    # tag は付け替えられるので、固定は flake.lock の commit に任せる
    go-arch-metrics.url = "github:usadamasa/go-arch-metrics";
    # package は nixpkgs の go_latest でビルドされる。vulncheck は自分をビルドした Go より新しい系列の
    # go 行のモジュールを測れないので、この flake の nixpkgs に揃えて go_1_27 に追いつかせる。
    # 下の devGo との patch 差 (go_latest の 1.27.3 でビルドし、go 行は 1.27.4) は測定を妨げない
    go-arch-metrics.inputs.nixpkgs.follows = "nixpkgs";
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
      # nixos-unstable の go_1_27 は 1.27.3 で、net/http の MaxBytesReader のデータ競合 (CVE-2026-97054) と
      # cgo 経由でビルド時に任意コードが実行される cmd/go などの脆弱性 (CVE-2026-97033〜97043) が残る。
      # channel の go_1_27 が 1.27.4 より古いときだけソースを差し替える。Dependabot の nix PR が 1.27.4 以上の
      # nixpkgs を取り込んだ時点で channel の go_1_27 に戻り、以後のパッチ更新は flake.lock と一緒に上がる。
      # 戻った後の差し替えの分岐は使われないので、そのとき消す
      devGo =
        pkgs:
        if pkgs.lib.versionOlder pkgs.go_1_27.version "1.27.4" then
          pkgs.go_1_27.overrideAttrs (old: rec {
            version = "1.27.4";
            src = pkgs.fetchurl {
              url = "https://go.dev/dl/go${version}.src.tar.gz";
              hash = "sha256-c8+4VxrsDAEXdofbhxY9Rqog6XeWcncokaoXZU2aYJk=";
            };
          })
        else
          pkgs.go_1_27;
    in
    {
      devShells = forAllSystems (
        system: pkgs: {
          default = pkgs.mkShellNoCC {
            packages = with pkgs; [
              # go.mod の go 行と同じ系列。ずれたら GOTOOLCHAIN=local で即座に失敗させる
              (devGo pkgs)
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
