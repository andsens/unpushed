{
  description = "Tray icon showing unpushed and unpulled commits";
  inputs = {
    systems.url = "github:nix-systems/default-linux";
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    flake-parts.url = "github:hercules-ci/flake-parts";
  };
  outputs =
    { systems, flake-parts, ... }@inputs:
    flake-parts.lib.mkFlake { inherit inputs; } (
      { self, ... }:
      {
        systems = import systems;
        flake.homeModules.default = import ./nix/home-module.nix { inherit self; };
        perSystem =
          { pkgs, ... }:
          {
            packages.default = pkgs.callPackage ./nix/package.nix { };
          };
      }
    );
}
