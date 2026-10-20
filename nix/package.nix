{
  lib,
  buildGo126Module,
  makeWrapper,
  pkg-config,
  libglvnd,
  libx11,
  libxxf86vm,
  wayland,
  wayland-protocols,
  libxkbcommon,
  git,
  findutils,
  coreutils,
  util-linux,
  xdg-terminal-exec,
}:
buildGo126Module {
  name = "unpushed";
  meta.mainProgram = "unpushed";
  src = ../src;
  vendorHash = "sha256-ah47sX0wzM3SFA130JRR5VSFJCSvAX1w+wyT6xx5tE4=";
  # go-glfw builds for either X11 or Wayland, migrated_fynedo has fyne check
  # that UI calls come through fyne.Do
  tags = [
    "wayland"
    "migrated_fynedo"
  ];
  nativeBuildInputs = [
    makeWrapper
    pkg-config
  ];
  buildInputs = [
    libglvnd
    libx11
    libxxf86vm
    wayland
    wayland-protocols
    libxkbcommon
  ];
  nativeCheckInputs = [
    git
    util-linux
  ];
  # Suffixed, so the session's own git and terminal win
  # On Wayland the compositor finds the window's icon through the .desktop
  # file named for its app ID, which fyne sets to the app's unique ID
  postInstall = ''
    install -Dm644 ${../assets/icon.svg} $out/share/icons/hicolor/scalable/apps/io.github.andsens.unpushed.svg
    install -Dm644 /dev/stdin $out/share/applications/io.github.andsens.unpushed.desktop <<EOF
    [Desktop Entry]
    Type=Application
    Name=unpushed
    Comment=Tray icon for unpushed and unpulled git commits
    Exec=unpushed
    Icon=io.github.andsens.unpushed
    Categories=Development;
    EOF
    wrapProgram $out/bin/unpushed --suffix PATH : ${
      lib.makeBinPath [
        git
        findutils
        coreutils
        util-linux
        xdg-terminal-exec
      ]
    }
  '';
}
