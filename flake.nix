{
  description = "emb - Redis-compatible embedding server and its product site";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    # `wrangler`, the Cloudflare CLI the site deploys and configures its Worker
    # with, gets its own pin. The main `nixpkgs` rev's `wrangler` is not in the
    # binary cache: adding it would build two derivations from source and fetch
    # ~2 GB of pnpm dependencies, which `websiteDeps` must not do (AGENTS.md).
    # This revision caches the whole wrangler closure. It is pinned to a commit
    # rather than to `nixos-unstable` so that the two inputs stay independent —
    # a `nix flake update nixpkgs` cannot drag the server's ONNX runtime and
    # toolchain forward just to keep wrangler cached.
    nixpkgs-wrangler.url = "github:NixOS/nixpkgs/eaad089433ca2bb662274377d33df3d0e51ef28b";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, nixpkgs-wrangler, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        # Only `wrangler` is taken from this second pin; nothing else.
        pkgsWrangler = import nixpkgs-wrangler { inherit system; };
        onnxruntime = pkgs.onnxruntime;

        archMap = {
          aarch64-darwin = { arch = "darwin-arm64";  hash = "sha256-+4S4suNJpZUnZ//oDM2GL8RAhN5H87DMPwt8nU5knPc="; };
          x86_64-darwin =  { arch = "darwin-x86_64"; hash = ""; };
          aarch64-linux =  { arch = "linux-aarch64"; hash = ""; };
          x86_64-linux  =  { arch = "linux-x86_64";  hash = "sha256-clVs3KeY3U6nzaujCOXw1oqMuTtnyW7fSFt6Dt17B/Q="; };
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

        # ── dependency lists ───────────────────────────────────────────
        # The repository has two halves that share almost nothing: a Go
        # server with a CGo ONNX runtime, and a static site with no build
        # step at all. Each half owns a list. Add to the list that owns the
        # thing, and `nix develop` (which takes both) keeps working for
        # everyone; `nix develop .#server` and `.#website` are the lean ones.

        # The server: toolchain, the runtime it links against, and the
        # clients and harnesses its test suites drive it with.
        serverDeps = (with pkgs; [
          go
          gopls
          golangci-lint
          just
          python3             # generator + bench scripts
          redis               # integration tests and the bench harness
          ruby_3_4
          bundler             # gems/emb and gems/emb-server
          act                 # run .github/workflows locally
          xan
        ]) ++ [
          onnxruntime
          libtokenizers
        ];

        # The website: nothing to build, so this is verification and asset
        # tooling rather than a runtime.
        #   agent-browser  the headless-browser CLI the agent harness drives.
        #                  nixpkgs ships the CLI only, so run
        #                  `just website-browser` once to fetch Chrome for
        #                  Testing, or point it at a Chromium you already have
        #                  with AGENT_BROWSER_EXECUTABLE_PATH.
        #   pillow/numpy   image measurement for the generated assets
        #   wrangler       the Cloudflare CLI for configuring and deploying
        #                  the site's Worker. It comes from the separate
        #                  `nixpkgs-wrangler` pin, because the main pin's copy
        #                  is not in the binary cache and would build from
        #                  source. See that input's comment.
        #   flyctl         the Fly.io CLI for deploying the sandbox
        #                  (`just sandbox-deploy`), which lives under
        #                  `website/repl/` and so belongs to the website half.
        #
        # `firefox` is deliberately absent: on aarch64-darwin the nixpkgs we
        # pin builds it from source, which is hours, not minutes. Check any
        # addition with:
        #   nix-store -qR $(nix eval --raw .#devShells.aarch64-darwin.website.drvPath) \
        #     | grep '\.source.*\.drv$'
        websiteDeps = (with pkgs; [
          python3
          python3Packages.pillow
          python3Packages.numpy
          nodejs_22
          agent-browser
          imagemagick
          pngquant
          optipng
          jpegoptim
          libwebp
          html-tidy
          flyctl
        ]) ++ [ pkgsWrangler.wrangler ];

        # The CGo/runtime environment the server binary needs. Everything
        # that runs `bin/emb` outside this shell fails to find the ONNX
        # shared library — see AGENTS.md.
        serverHook = ''
          export CGO_CFLAGS="-I${onnxruntime}/include/onnxruntime"
          export CGO_LDFLAGS="-L${onnxruntime}/lib -lonnxruntime -L${libtokenizers}/lib"
          export C_INCLUDE_PATH="${onnxruntime}/include/onnxruntime:$C_INCLUDE_PATH"
          export LIBRARY_PATH="${onnxruntime}/lib:${libtokenizers}/lib:$LIBRARY_PATH"
          # macOS runtime linker
          export DYLD_LIBRARY_PATH="${onnxruntime}/lib:$DYLD_LIBRARY_PATH"
          # Linux runtime linker
          export LD_LIBRARY_PATH="${onnxruntime}/lib:${libtokenizers}/lib:$LD_LIBRARY_PATH"
        '';

        websiteHook = ''
          # A local `npm install agent-browser` wins over the packaged one.
          if [ -d website/node_modules/.bin ]; then
            export PATH="$PWD/website/node_modules/.bin:$PATH"
          fi
          echo "website: \`just website\` serves website/ on :8080;" \
               "\`just website-dev\` serves it with a local sandbox behind the console;" \
               "\`just website-browser\` fetches Chrome for Testing once."
        '';
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

        # The site's browser CLI, installable on its own for editors and agent
        # harnesses that run outside `nix develop`:
        #   nix profile install .#agent-browser
        packages.agent-browser = pkgs.agent-browser;

        # The Cloudflare CLI on its own, from the dedicated pin above:
        #   nix profile install .#wrangler
        packages.wrangler = pkgsWrangler.wrangler;

        # The Fly.io CLI on its own, for deploying the sandbox
        # (`just sandbox-deploy`):
        #   nix profile install .#flyctl
        packages.flyctl = pkgs.flyctl;

        devShells = {
          # Both halves. This is the shell AGENTS.md points at, so it keeps
          # providing everything the documented commands need.
          default = pkgs.mkShell {
            buildInputs = serverDeps ++ websiteDeps;
            shellHook = serverHook + websiteHook;
          };

          # `nix develop .#server` — Go, ONNX, Redis, the Ruby clients.
          server = pkgs.mkShell {
            buildInputs = serverDeps;
            shellHook = serverHook;
          };

          # `nix develop .#website` — no Go, no ONNX: a browser, a renderer
          # and the image tools.
          website = pkgs.mkShell {
            buildInputs = websiteDeps;
            shellHook = websiteHook;
          };
        };
      });
}
