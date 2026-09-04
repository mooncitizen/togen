{
  description = "Togen development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        # Terraform is BSL, which nix classes as unfree. Allowed by name so
        # `nix develop` works without NIXPKGS_ALLOW_UNFREE and --impure.
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfreePredicate =
            pkg: builtins.elem (nixpkgs.lib.getName pkg) [ "terraform" ];
        };
      in
      {
        devShells.default = pkgs.mkShell {
          name = "togen";

          packages = with pkgs; [
            # cli, resolvers, emitters
            go
            gopls
            golangci-lint
            goreleaser

            # canvas
            nodejs_22
            pnpm

            # acceptance
            terraform

            # github
            gh
          ];

          shellHook = ''
            export TF_PLUGIN_CACHE_DIR="$HOME/.terraform.d/plugin-cache"
            mkdir -p "$TF_PLUGIN_CACHE_DIR"
          '';
        };
      });
}
