{
  description = "gh-manage の開発環境 (Go と lint・CI のツール)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    # tag は付け替えられるので、固定は flake.lock の commit に任せる
    go-arch-metrics.url = "github:usadamasa/go-arch-metrics";
    # package は nixpkgs の go_latest でビルドされる。vulncheck は自分をビルドした Go より新しい系列の
    # go 行のモジュールを測れないので、この flake の nixpkgs に揃えて go_1_27 に追いつかせる。
    # go_latest が 1.27.4 より古いあいだは、go-arch-metrics 側が下の devGo と同じ配布バイナリでビルドする
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
      # channel の go_1_27 が 1.27.4 より古いときだけ go.dev の配布バイナリに差し替える (ソースからビルドすると
      # binary cache に無く、flake を変える PR の CI は main が nix store を保存するまで run ごとに devShell の
      # 準備に約 3 分かかる)。Dependabot の nix PR が 1.27.4 以上の nixpkgs を取り込んだ時点で channel の
      # go_1_27 に戻り、以後のパッチ更新は flake.lock と一緒に上がる。戻った後の差し替えの分岐は使われないので、
      # そのとき消す
      devGo =
        pkgs:
        if pkgs.lib.versionOlder pkgs.go_1_27.version "1.27.4" then
          pkgs.stdenvNoCC.mkDerivation rec {
            pname = "go-bin";
            version = "1.27.4";
            src =
              let
                platform = with pkgs.stdenvNoCC.hostPlatform.go; "${GOOS}-${GOARCH}";
              in
              pkgs.fetchurl {
                url = "https://go.dev/dl/go${version}.${platform}.tar.gz";
                # https://go.dev/dl/?mode=json&include=all の sha256
                sha256 =
                  {
                    darwin-arm64 = "91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4";
                    linux-amd64 = "7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840";
                    linux-arm64 = "3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92";
                  }
                  .${platform};
              };
            # strip は darwin の署名を壊す。配布物をそのまま置く
            dontFixup = true;
            installPhase = ''
              mkdir -p $out/share $out/bin
              cp -r . $out/share/go
              ln -s $out/share/go/bin/go $out/share/go/bin/gofmt $out/bin/
            '';
          }
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
          # 一時: setup-nix の devshell input の検証用
          ci-test = pkgs.mkShellNoCC {
            packages = [ pkgs.hello ];
          };
        }
      );
    };
}
