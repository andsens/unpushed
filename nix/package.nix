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
  vendorHash = "sha256-0dx5bZONDcsmPkq3x70ZV38pAOppwOPomCjK7ZAZEvM=";
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
  postInstall = ''
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
