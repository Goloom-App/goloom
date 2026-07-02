{
  description = "goloom development shell";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in {
        formatter = pkgs.nixpkgs-fmt;

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gnumake
            gopls
            gotools
            go-tools
            delve
            just
            jq
            nodejs_22
            pnpm
            postgresql
          ];

          shellHook = ''
            export CGO_ENABLED=0
            echo "goloom dev shell ready — run \`just\` for the task overview"
          '';
        };
      });
}
