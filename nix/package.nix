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
}:
buildGo126Module {
  name = "unpushed";
  meta.mainProgram = "unpushed";
  src = ../src;
  vendorHash = "sha256-cj7Z1I3Bg3Q2n6hBR9zyVHoWzOwrtUFg/u4zfak3jt4=";
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
      ]
    }
  '';
}
