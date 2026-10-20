{ self }:
{
  config,
  pkgs,
  lib,
  ...
}:
let
  cfg = config.services.unpushed;
in
{
  options.services.unpushed = {
    enable = lib.mkEnableOption "the tray icon for unpushed commits, configured in ~/.config/unpushed.json";
    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "unpushed.packages.\${system}.default";
      description = "The unpushed package.";
    };
  };
  config = lib.mkIf cfg.enable {
    systemd.user.services.unpushed = {
      Unit = {
        Description = "Tray icon for unpushed commits";
        PartOf = [ "graphical-session.target" ];
        After = [ "graphical-session.target" ];
      };
      Service = {
        ExecStart = lib.getExe cfg.package;
        Restart = "on-failure";
      };
      Install.WantedBy = [ "graphical-session.target" ];
    };
  };
}
