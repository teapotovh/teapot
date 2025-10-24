let
  pkgs = import <nixpkgs> { config = { allowUnfree = true; }; };
  shellHook = ''
    # fixes libstdc++ issues and libgl.so issues
    LD_LIBRARY_PATH=${pkgs.stdenv.cc.cc.lib}/lib/
    unset SOURCE_DATE_EPOCH
  '';
in pkgs.mkShell {
  buildInputs = with pkgs; [
    gnumake
    protobuf

    go
    protoc-gen-go-vtproto

    python313
    python313Packages.venvShellHook
    python313Packages.ipython
    uv

    grpcurl
  ];
  venvDir = "./.venv";
  postVenvCreation = shellHook;
  postShellHook = shellHook;
}
