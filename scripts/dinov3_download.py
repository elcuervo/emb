#!/usr/bin/env python3
"""Downloads a DINOv3 ONNX export, sidecar weights and metadata included.

Usage: dinov3_download.py <repo> <dir>

The published DINOv3 exports keep their weights in an external `<graph>_data`
sidecar recorded in the repository listing, and only some of them ship
`config.json` / `preprocessor_config.json`. This fetches the files the server
needs to load and serve the model:

  * the fp32 graph (`onnx/model.onnx`, or the repository's only `.onnx`),
  * its `<graph>_data` sidecar when the repo publishes one,
  * `config.json` and `preprocessor_config.json` when present (at the root or
    beside the graph).

The sidecar name always comes from the listing, so a new export layout needs no
change here.

Two facts are recorded rather than assumed: a download lands in a `.part` file
and is renamed only once complete, so an interrupted run cannot leave a file the
next run accepts; and the directory names the repository it holds, so a second
repository asked for the same directory is refused instead of silently keeping
the first one's weights (both ViT-S and ViT-B publish `model.onnx`).
"""
import json
import os
import sys
import urllib.request

BASE = "https://huggingface.co"
SOURCE = ".source"


def fetch(url, dest):
    part = dest + ".part"
    try:
        urllib.request.urlretrieve(url, part)
        os.replace(part, dest)
    finally:
        if os.path.exists(part):
            os.remove(part)


def recorded_repo(dest_dir):
    try:
        with open(os.path.join(dest_dir, SOURCE), encoding="utf-8") as handle:
            return handle.read().strip()
    except FileNotFoundError:
        return ""


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: dinov3_download.py <repo> <dir>")
    repo, dest_dir = sys.argv[1], sys.argv[2]
    os.makedirs(dest_dir, exist_ok=True)

    recorded = recorded_repo(dest_dir)
    if recorded and recorded != repo:
        sys.exit(f"{dest_dir} already holds {recorded}; use another directory "
                 f"to fetch {repo}, or delete this one")

    with urllib.request.urlopen(f"{BASE}/api/models/{repo}") as resp:
        listing = json.load(resp)
    files = [s["rfilename"] for s in listing.get("siblings", [])]

    graphs = [f for f in files if f.endswith(".onnx") and "quantized" not in f and "_q4" not in f]
    if not graphs:
        sys.exit(f"no fp32 ONNX graph in {repo}")
    # Prefer the conventional onnx/model.onnx, then the shortest path.
    graph = min(graphs, key=lambda f: (not f.endswith("onnx/model.onnx"), len(f)))
    graph_name = os.path.basename(graph)

    wanted = [(graph, graph_name)]
    for sidecar in (f"{graph}_data", f"{graph}.data"):
        if sidecar in files:
            wanted.append((sidecar, os.path.basename(sidecar)))
    for meta in ("config.json", "preprocessor_config.json"):
        beside = f"{os.path.dirname(graph)}/{meta}".lstrip("/")
        for candidate in (beside, meta):
            if candidate in files:
                wanted.append((candidate, meta))
                break

    for remote, local in wanted:
        target = os.path.join(dest_dir, local)
        if os.path.exists(target) and os.path.getsize(target) > 0:
            print(f"✓ {local} (exists)")
            continue
        print(f"  {remote} -> {target}")
        fetch(f"{BASE}/{repo}/resolve/main/{remote}", target)

    with open(os.path.join(dest_dir, SOURCE), "w", encoding="utf-8") as handle:
        handle.write(repo + "\n")
    print(f"model: {os.path.join(dest_dir, graph_name)}")


if __name__ == "__main__":
    main()
