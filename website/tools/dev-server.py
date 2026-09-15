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
`emb`, which is the whole loop: open http://localhost:8080 and the console runs
real commands. Use `just website` (plain static serving) for the ink probe and
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


class Handler(http.server.SimpleHTTPRequestHandler):
    sandbox = PRODUCTION_SANDBOX

    def do_GET(self) -> None:  # noqa: N802 - stdlib name
        path = Path(self.translate_path(self.path))
        if path.is_dir():
            path = path / "index.html"
        if path.suffix == ".html" and path.is_file():
            body = (
                path.read_text(encoding="utf-8")
                .replace(PRODUCTION_SANDBOX, self.sandbox)
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

    def log_message(self, fmt: str, *args: object) -> None:
        print(f"dev-server: {fmt % args}", flush=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("port", nargs="?", type=int, default=8080)
    parser.add_argument(
        "--sandbox",
        default="http://127.0.0.1:8081",
        help="origin the console's client module is loaded from (default: %(default)s)",
    )
    parser.add_argument("--bind", default="127.0.0.1")
    args = parser.parse_args()

    if not SITE_DIR.is_dir():
        raise SystemExit(f"dev-server: no site directory at {SITE_DIR}")

    Handler.sandbox = args.sandbox
    handler = functools.partial(Handler, directory=str(SITE_DIR))
    with http.server.ThreadingHTTPServer((args.bind, args.port), handler) as httpd:
        print(
            f"dev-server: http://{args.bind}:{args.port}/ → {SITE_DIR}\n"
            f"dev-server: console client module loaded from {args.sandbox}",
            flush=True,
        )
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            pass
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
