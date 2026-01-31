let
  pkgs = import <nixpkgs> { config = { allowUnfree = true; }; };
  shellHook = ''
    # fixes libstdc++ issues and libgl.so issues
    LD_LIBRARY_PATH=${pkgs.stdenv.cc.cc.lib}/lib/
    unset SOURCE_DATE_EPOCH
  '';
  bazel = pkgs.writeShellScriptBin "bazel" ''
    exec ${pkgs.steam-run}/bin/steam-run ${pkgs.bazelisk}/bin/bazelisk "$@"
  '';
in pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    gopls
    golangci-lint
    golangci-lint-langserver

    python313

    grpcurl

    bazelisk
    steam-run
    bazel
    python313Packages.pip-tools
  ];
}
