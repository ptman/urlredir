# Copyright © Paul Tötterman <paul.totterman@gmail.com>. All rights reserved.
{
  description = "URL redirector";

  inputs = {
    nixpkgs.url = "nixpkgs/nixos-23.11";
    nixpkgs-unstable.url = "nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, nixpkgs-unstable }:
    let
      lastModifiedDate =
        self.lastModifiedDate or self.lastModified or "19700101";
      version = builtins.substring 0 8 lastModifiedDate;
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
      unstableFor =
        forAllSystems (system: import nixpkgs-unstable { inherit system; });
    in {
      packages = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};
        in {
          urlredir = pkgs.buildGoModule {
            pname = "urlredir";
            inherit version;
            src = ./.;
            vendorHash = "sha256-UjSRjWZVF8W09L6tJCUI5lC3oScV1UjCHoXadfArSXY=";
          };
        });

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
          unstable = unstableFor.${system};
        in {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              cloc
              docker
              entr
              git
              gnumake
              unstable.go_1_22
              (unstable.golangci-lint.override {
                buildGoModule = unstable.buildGo122Module;
              })
              wget
            ];
          };
        });

      defaultPackage = forAllSystems (system: self.packages.${system}.urlredir);
    };
}
