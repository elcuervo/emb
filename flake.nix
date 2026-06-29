{
  description = "emb - Redis-compatible embedding server";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        onnxruntime = pkgs.onnxruntime;
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "emb";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

          buildInputs = [ onnxruntime ];

          preBuild = ''
            export CGO_CFLAGS="-I${onnxruntime}/include/onnxruntime"
            export CGO_LDFLAGS="-L${onnxruntime}/lib -lonnxruntime"
          '';
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            onnxruntime
            python3
            redis
          ];

          shellHook = ''
            export CGO_CFLAGS="-I${onnxruntime}/include/onnxruntime"
            export CGO_LDFLAGS="-L${onnxruntime}/lib -lonnxruntime"
            export C_INCLUDE_PATH="${onnxruntime}/include/onnxruntime:$C_INCLUDE_PATH"
            export LIBRARY_PATH="${onnxruntime}/lib:$LIBRARY_PATH"
            export DYLD_LIBRARY_PATH="${onnxruntime}/lib:$DYLD_LIBRARY_PATH"
            export PATH="$PWD/scripts:$PATH"
          '';
        };

        devShells.model-setup = pkgs.mkShell {
          buildInputs = with pkgs; [
            python3
          ];

          shellHook = ''
            export PATH="$PWD/scripts:$PATH"
          '';
        };
      });
}
