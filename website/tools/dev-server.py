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

    python3 website/tools/dev-server.py 8080 --sandbox-port 8081

Run it together with the bridge and a local `emb` via `just website-dev`,
which is the whole loop: open the printed address and the console runs real
commands. Use `just website` (plain static serving) for the ink probe and
anything else that must see the published tree.
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

# The modules whose bytes change while their names do not. A browser that keeps
# one heuristically cached page dies on the import, so the working loop names
# them from their own mtimes: `demos.js` gains an export, the page names it, and
# the template hands the page the current bytes instead of yesterday's.
LIVE_MODULES = ("demos.js", "tldr.js")


def module_token() -> str:
    newest = 0.0
    for name in LIVE_MODULES:
        try:
            newest = max(newest, (SITE_DIR / "assets" / "js" / name).stat().st_mtime)
        except OSError:
            pass
    return str(int(newest))


class Handler(http.server.SimpleHTTPRequestHandler):
    # The bridge's port on this machine. The host is taken from each request, so
    # the loop works from `localhost`, from `127.0.0.1`, and from a LAN address
    # without knowing any of them up front.
    sandbox_port = 8081

    def send_response(self, code: int, message: str | None = None) -> None:
        # Nothing in the working loop is cached. The published tree pins its own
        # policy in `_headers` (revalidate, never immutable); this server is for
        # the loop, where the answer is always the file on disk. Sending it for
        # every response, not just the page, is what keeps a stale module from
        # outliving the export it is missing.
        super().send_response(code, message)
        self.send_header("Cache-Control", "no-store")

    def do_GET(self) -> None:  # noqa: N802 - stdlib name
        path = Path(self.translate_path(self.path))
        if path.is_dir():
            path = path / "index.html"
        if path.suffix == ".html" and path.is_file():
            html = path.read_text(encoding="utf-8").replace(PRODUCTION_SANDBOX, self.sandbox_origin())
            token = module_token()
            for name in LIVE_MODULES:
                html = html.replace("assets/js/" + name, "assets/js/" + name + "?v=" + token)
            body = html.encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        super().do_GET()

    def sandbox_origin(self) -> str:
        """The bridge's origin as the browser can reach it.

        Derived from the request's own Host so the module loads from the same
        address the page did, rather than from a fixed one.
        """
        host = self.headers.get("Host", "") or "127.0.0.1"
        return f"http://{host.split(':')[0]}:{self.sandbox_port}"

    def log_message(self, fmt: str, *args: object) -> None:
        print(f"dev-server: {fmt % args}", flush=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("port", nargs="?", type=int, default=8080)
    parser.add_argument(
        "--sandbox-port",
        type=int,
        default=8081,
        help="port of the local bridge (default: %(default)s)",
    )
    parser.add_argument("--bind", default="127.0.0.1")
    args = parser.parse_args()

    if not SITE_DIR.is_dir():
        raise SystemExit(f"dev-server: no site directory at {SITE_DIR}")

    Handler.sandbox_port = args.sandbox_port
    handler = functools.partial(Handler, directory=str(SITE_DIR))
    with http.server.ThreadingHTTPServer((args.bind, args.port), handler) as httpd:
        print(
            f"dev-server: http://{args.bind}:{args.port}/ → {SITE_DIR}\n"
            f"dev-server: console client module loaded from "
            f"http://<this host>:{args.sandbox_port}",
            flush=True,
        )
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            pass
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
