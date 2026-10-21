{ self }:
{
  config,
  pkgs,
  lib,
  ...
}:
let
  cfg = config.services.unpushed;
  inherit (lib) types mkOption;
  optional =
    type: description:
    mkOption {
      type = types.nullOr type;
      default = null;
      inherit description;
    };
  settings = lib.filterAttrsRecursive (_: v: v != null) cfg.settings;
  file = (pkgs.formats.json { }).generate "unpushed.json" settings;
in
{
  options.services.unpushed = {
    enable = lib.mkEnableOption "the tray icon for unpushed commits";
    package = mkOption {
      type = types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "unpushed.packages.\${system}.default";
      description = "The unpushed package.";
    };
    settings = {
      roots = mkOption {
        type = types.attrsOf (
          types.submodule {
            options.include = mkOption {
              type = types.bool;
              description = "Whether to search the directory, or leave it out of the search below another root.";
            };
          }
        );
        default = { };
        example = {
          "~/src".include = true;
          "~/src/archive".include = false;
        };
        description = "Directories to search for repositories, by path.";
      };
      repos = mkOption {
        type = types.attrsOf (
          types.submodule {
            options = {
              ignore = optional types.bool "Whether to leave the repository out.";
              checkRemote = optional types.bool "Whether to query the remotes for commits to pull.";
              pinned = optional types.bool "Whether to list the repository first in the menu.";
            };
          }
        );
        default = { };
        example."~/src/work/api" = {
          checkRemote = true;
          pinned = true;
        };
        description = "Settings for single repositories, by path.";
      };
      depth = optional types.ints.positive "How many levels below a root a repository may be.";
      max = optional types.ints.positive "How many repositories the menu lists.";
      scanInterval = optional types.str "How often to search the roots, such as `\"30m\"`.";
      remoteInterval = optional types.str "How often to query the remotes of the repositories set to `checkRemote`.";
    };
  };
  config = lib.mkIf cfg.enable {
    # For the .desktop file and icon the window is matched with
    home.packages = [ cfg.package ];
    systemd.user.services.unpushed = {
      Unit = {
        Description = "Tray icon for unpushed commits";
        PartOf = [ "graphical-session.target" ];
        After = [ "graphical-session.target" ];
      };
      Service = {
        # The settings window writes the user's config on top of this one
        ExecStart = "${lib.getExe cfg.package} --config ${file}";
        Restart = "on-failure";
      };
      Install.WantedBy = [ "graphical-session.target" ];
    };
  };
}
