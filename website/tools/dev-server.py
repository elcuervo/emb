#!/usr/bin/env python3
"""Serve `website/` locally with the console's sandbox origin pointed at a local bridge.

`just website` serves the tree exactly as published — and published means the
console loads its client module from `https://cli.emb.is`. That is correct for
the published page and wrong for a local test, where the bridge is on localhost.

This server serves the same tree with one substitution in HTML responses: the
production sandbox origin becomes the local one. Every other byte is the
published file, so the page under test differs from production in exactly one
respect, and the published page keeps a single source rather than growing a
dev-only branch.

    python3 website/tools/dev-server.py 8080 --sandbox http://127.0.0.1:8081

`just website-dev` starts this together with the sandbox bridge and a local
`emb`, which is the whole loop: open the printed address and the console runs
real commands. It binds `0.0.0.0`, so a phone on the same network can open the
site at `http://<your-lan-ip>:8080` and get the same live console — the module
origin is derived from whatever address the browser used, and the bridge
accepts the same host on its own port. Use `just website` (plain static
serving) for the ink probe and anything else that must see the published tree.
"""

from __future__ import annotations

import argparse
import functools
import http.server
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SITE_DIR = REPO_ROOT / "website"

# The origin the published page names. Rewritten, never removed.
PRODUCTION_SANDBOX = "https://cli.emb.is"


class Handler(http.server.SimpleHTTPRequestHandler):
    # The origin to substitute. Empty means "whatever host this request
    # arrived on", which is what makes the loop work from `localhost`, from
    # `127.0.0.1`, and from a LAN address without knowing any of them up front.
    sandbox = ""
    sandbox_port = 8081

    def do_GET(self) -> None:  # noqa: N802 - stdlib name
        path = Path(self.translate_path(self.path))
        if path.is_dir():
            path = path / "index.html"
        if path.suffix == ".html" and path.is_file():
            body = (
                path.read_text(encoding="utf-8")
                .replace(PRODUCTION_SANDBOX, self.sandbox_origin())
                .encode("utf-8")
            )
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            # Never cached: this is the working loop, and a heuristically
            # cached page is how a stale console keeps running after an edit.
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(body)
            return
        super().do_GET()

    def sandbox_origin(self) -> str:
        """The bridge's origin as the browser can reach it.

        Derived from the request's own Host so a phone on the LAN loads the
        module from the same address it loaded the page from, rather than
        from the phone's own loopback.
        """
        if self.sandbox:
            return self.sandbox
        host = self.headers.get("Host", "") or "127.0.0.1"
        if host.startswith("["):  # IPv6 literal, keep the brackets
            name = host.split("]")[0] + "]"
        else:
            name = host.split(":")[0]
        return f"http://{name}:{self.sandbox_port}"

    def log_message(self, fmt: str, *args: object) -> None:
        print(f"dev-server: {fmt % args}", flush=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("port", nargs="?", type=int, default=8080)
    parser.add_argument(
        "--sandbox",
        default="",
        help="origin the console's client module is loaded from; when empty it "
        "is derived from each request's Host (default: empty)",
    )
    parser.add_argument(
        "--sandbox-port",
        type=int,
        default=8081,
        help="port of the local bridge, used when --sandbox is empty "
        "(default: %(default)s)",
    )
    parser.add_argument("--bind", default="127.0.0.1")
    args = parser.parse_args()

    if not SITE_DIR.is_dir():
        raise SystemExit(f"dev-server: no site directory at {SITE_DIR}")

    Handler.sandbox = args.sandbox
    Handler.sandbox_port = args.sandbox_port
    handler = functools.partial(Handler, directory=str(SITE_DIR))
    with http.server.ThreadingHTTPServer((args.bind, args.port), handler) as httpd:
        target = args.sandbox or f"http://<this host>:{args.sandbox_port} (derived per request)"
        print(
            f"dev-server: http://{args.bind}:{args.port}/ → {SITE_DIR}\n"
            f"dev-server: console client module loaded from {target}",
            flush=True,
        )
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            pass
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
