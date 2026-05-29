{
  description = "NixPeek - attrPath-first Nix package search TUI";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems f;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          nixpeek = pkgs.buildGoModule {
            pname = "nixpeek";
            version = self.rev or "dirty";
            src = self;
            vendorHash = "sha256-ecodI5ImNp1bkpNcUDKnKK/uUzJwOQ+u2lTz6eq7kM4=";
            subPackages = [ "cmd/nixpeek" ];
            ldflags = [ "-s" "-w" ];
            meta = with pkgs.lib; {
              description = "Cross-platform TUI for searching Nix packages with attrPath-first UX";
              homepage = "https://github.com/voidhartt/NixPeek";
              license = licenses.mit;
              mainProgram = "nixpeek";
              platforms = platforms.unix;
            };
          };
        in
        {
          default = nixpeek;
          nixpeek = nixpeek;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/nixpeek";
        };
      });
    };
}
