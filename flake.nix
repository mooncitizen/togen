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
            playwright-driver.browsers

            # acceptance
            terraform

            # github
            gh

            # tasks
            just
          ];

          shellHook = ''
            export TF_PLUGIN_CACHE_DIR="''${TF_PLUGIN_CACHE_DIR:-$HOME/.terraform.d/plugin-cache}"
            mkdir -p "$TF_PLUGIN_CACHE_DIR"

            # Vitest browser mode drives the chromium from this driver, so
            # ui/package.json pins the npm playwright to the same version.
            export PLAYWRIGHT_BROWSERS_PATH="${pkgs.playwright-driver.browsers}"
            export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true

            # The canvas embeds IBM Plex; `just fonts` copies the faces from here.
            export IBM_PLEX="${pkgs.ibm-plex}/share/fonts/opentype"
          '';
        };
      });
}
