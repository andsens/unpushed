{
  lib,
  buildGo126Module,
  makeWrapper,
  git,
  findutils,
  coreutils,
  util-linux,
}:
buildGo126Module {
  name = "unpushed";
  meta.mainProgram = "unpushed";
  src = ../src;
  vendorHash = "sha256-9MIvDcm5TuTUGEhsjyB9HNf+6b0MOMbsCAvUy57mNuA=";
  env.CGO_ENABLED = 0;
  nativeBuildInputs = [ makeWrapper ];
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
