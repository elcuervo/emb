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

        archMap = {
          aarch64-darwin = { arch = "darwin-arm64";  hash = "sha256-+4S4suNJpZUnZ//oDM2GL8RAhN5H87DMPwt8nU5knPc="; };
          x86_64-darwin =  { arch = "darwin-x86_64"; hash = ""; };
          aarch64-linux =  { arch = "linux-aarch64"; hash = ""; };
          x86_64-linux  =  { arch = "linux-x86_64";  hash = ""; };
        };

        ltInfo = builtins.getAttr system archMap;

        libtokenizers = pkgs.stdenv.mkDerivation {
          pname = "libtokenizers";
          version = "1.27.0";
          src = pkgs.fetchurl {
            url = "https://github.com/daulet/tokenizers/releases/download/v1.27.0/libtokenizers.${ltInfo.arch}.tar.gz";
            hash = ltInfo.hash;
          };
          dontBuild = true;
          dontUnpack = true;
          installPhase = ''
            mkdir -p $out/lib
            tar xzf $src -C $out/lib
          '';
        };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "emb";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

          buildInputs = [ onnxruntime libtokenizers ];

          preBuild = ''
            export CGO_CFLAGS="-I${onnxruntime}/include/onnxruntime"
            export CGO_LDFLAGS="-L${onnxruntime}/lib -lonnxruntime -L${libtokenizers}/lib"
          '';
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            golangci-lint
            just
            onnxruntime
            libtokenizers
            python3
            redis
          ];

          shellHook = ''
            export CGO_CFLAGS="-I${onnxruntime}/include/onnxruntime"
            export CGO_LDFLAGS="-L${onnxruntime}/lib -lonnxruntime -L${libtokenizers}/lib"
            export C_INCLUDE_PATH="${onnxruntime}/include/onnxruntime:$C_INCLUDE_PATH"
            export LIBRARY_PATH="${onnxruntime}/lib:${libtokenizers}/lib:$LIBRARY_PATH"
            export DYLD_LIBRARY_PATH="${onnxruntime}/lib:$DYLD_LIBRARY_PATH"
          '';
        };
      });
}
