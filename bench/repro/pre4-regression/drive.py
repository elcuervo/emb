#!/usr/bin/env python3
"""Plain-EMB load driver for the pre4 regression reproduction.

A dependency-free RESP client (stdlib only) that measures per-request latency
for the plain `EMB` command, so the comparison never includes the Ruby client's
pool behaviour. Three modes:

  sweep  one closed-loop client per level, latency percentiles per level
  probe  sequential single requests issued while background load runs
  hol    one wide command followed immediately by narrow ones (head-of-line)

Every mode prints one JSON object on stdout so a run can be recorded verbatim.
"""

import argparse
import json
import socket
import statistics
import sys
import threading
import time


class RespError(Exception):
    pass


def read_line(f):
    line = f.readline()
    if not line:
        raise RespError("connection closed")
    if not line.endswith(b"\r\n"):
        raise RespError("malformed frame: %r" % line[:40])
    return line[:-2]


def read_reply(f):
    """Consume one RESP reply and return its payload (None for simple types)."""
    line = read_line(f)
    kind, rest = line[:1], line[1:]
    if kind in (b"+", b":", b"-"):
        return rest
    if kind == b"$":
        n = int(rest)
        if n < 0:
            return None
        data = f.read(n + 2)
        if len(data) != n + 2:
            raise RespError("short bulk")
        return data[:n]
    if kind == b"*":
        n = int(rest)
        if n < 0:
            return None
        return [read_reply(f) for _ in range(n)]
    raise RespError("unknown reply kind %r" % kind)


class Client:
    def __init__(self, host, port, timeout=30.0):
        self.sock = socket.create_connection((host, port), timeout=timeout)
        self.sock.settimeout(timeout)
        self.f = self.sock.makefile("rb")
        self.fd = self.sock

    def close(self):
        try:
            self.sock.close()
        except OSError:
            pass

    def command(self, *args):
        out = [b"*%d\r\n" % len(args)]
        for a in args:
            if isinstance(a, str):
                a = a.encode()
            out.append(b"$%d\r\n%s\r\n" % (len(a), a))
        self.sock.sendall(b"".join(out))
        return read_reply(self.f)

    def emb(self, model, texts, timeout=30.0):
        """One plain EMB command; returns elapsed seconds and the error, if any."""
        start = time.perf_counter()
        try:
            reply = self.command("EMB", model, *texts)
        except (OSError, RespError) as err:
            return time.perf_counter() - start, str(err)
        elapsed = time.perf_counter() - start
        if isinstance(reply, bytes) and reply.startswith(b"ERR"):
            return elapsed, reply.decode(errors="replace")
        return elapsed, None


def percentiles(samples):
    if not samples:
        return {}
    s = sorted(samples)
    def at(q):
        idx = min(len(s) - 1, max(0, int(round(q * (len(s) - 1)))))
        return round(s[idx] * 1000.0, 3)  # ms
    return {
        "n": len(s),
        "min_ms": round(s[0] * 1000.0, 3),
        "p50_ms": at(0.50),
        "p95_ms": at(0.95),
        "p99_ms": at(0.99),
        "max_ms": round(s[-1] * 1000.0, 3),
        "mean_ms": round(statistics.fmean(s) * 1000.0, 3),
    }


def run_level(args, level, total):
    per_worker = max(1, total // level)
    ok_latencies, errors = [], []
    lock = threading.Lock()
    ready = threading.Barrier(level)

    def worker():
        try:
            client = Client(args.host, args.port, args.timeout)
        except OSError as err:
            # A peer that cannot connect must not wedge the others on the
            # barrier: break it, record why, and drop out.
            with lock:
                errors.append("connect: %s" % err)
            ready.abort()
            return
        try:
            try:
                ready.wait(timeout=args.timeout)
            except threading.BrokenBarrierError:
                return
            for _ in range(per_worker):
                elapsed, err = client.emb(args.model, [args.text], args.timeout)
                with lock:
                    if err:
                        errors.append(err)
                    else:
                        ok_latencies.append(elapsed)
        finally:
            client.close()

    threads = [threading.Thread(target=worker) for _ in range(level)]
    start = time.perf_counter()
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    wall = time.perf_counter() - start
    done = level * per_worker
    succ = len(ok_latencies)
    return {
        "concurrency": level,
        "requests": done,
        "requests_ok": succ,
        "wall_s": round(wall, 3),
        # Throughput and latency are the *successful* work: an admission refusal
        # (ERR busy) is not throughput, and its short latency must not enter the
        # percentile sample as if the arm had served it.
        "req_s": round(succ / wall, 1) if wall > 0 else None,
        "req_s_attempted": round(done / wall, 1) if wall > 0 else None,
        "errors": len(errors),
        "error_sample": errors[0] if errors else None,
        "latency": percentiles(ok_latencies),
    }


def background_load(args, stop, level):
    """Keep `level` clients busy until stop is set; counts completed requests."""
    counter = {"n": 0, "err": 0}
    lock = threading.Lock()
    barrier = threading.Barrier(level)

    def worker():
        try:
            client = Client(args.host, args.port, args.timeout)
        except OSError:
            barrier.abort()
            return
        try:
            try:
                barrier.wait(timeout=args.timeout)
            except threading.BrokenBarrierError:
                return
            while not stop.is_set():
                _, err = client.emb(args.model, [args.text], args.timeout)
                with lock:
                    counter["n"] += 1
                    if err:
                        counter["err"] += 1
        finally:
            client.close()

    threads = [threading.Thread(target=worker) for _ in range(level)]
    for t in threads:
        t.start()
    return counter, threads


def mode_sweep(args):
    levels = [int(x) for x in args.levels.split(",") if x.strip()]
    return {"mode": "sweep", "model": args.model, "levels": [run_level(args, l, args.requests) for l in levels]}


def mode_probe(args):
    """Single-request latency while the server is saturated by background load."""
    stop = threading.Event()
    counter, threads = background_load(args, stop, args.background)
    time.sleep(args.warmup)
    probe = Client(args.host, args.port, args.timeout)
    latencies, errors = [], []
    try:
        for _ in range(args.samples):
            elapsed, err = probe.emb(args.model, [args.text], args.timeout)
            latencies.append(elapsed)
            if err:
                errors.append(err)
    finally:
        probe.close()
        stop.set()
        for t in threads:
            t.join()
    return {
        "mode": "probe",
        "model": args.model,
        "background_concurrency": args.background,
        "background_requests": counter["n"],
        "background_errors": counter["err"],
        "samples": args.samples,
        "errors": len(errors),
        "error_sample": errors[0] if errors else None,
        "latency": percentiles(latencies),
    }


def mode_hol(args):
    """One wide command in flight, then narrow ones on separate connections — the
    head-of-line blocking probe. The narrow requests must overlap the wide
    command, which requires their own connections: redcon runs a connection's
    commands serially, so on one connection the narrow requests would only ever
    run after the wide command finished and the probe would measure sequential
    requests instead of blocking."""
    wide = ["reproduction item %d" % i for i in range(args.texts)]
    wide_client = Client(args.host, args.port, args.timeout)
    box = {}

    def run_wide():
        box["wide"] = wide_client.emb(args.model, wide, args.timeout)

    t = threading.Thread(target=run_wide)
    t.start()
    # Let the wide command reach the server and occupy its run before the narrow
    # probes arrive; without it the probe races the wide command's dispatch.
    time.sleep(0.01)
    narrow = []
    try:
        for _ in range(args.follow):
            c = Client(args.host, args.port, args.timeout)
            try:
                elapsed, err = c.emb(args.model, [args.text], args.timeout)
                narrow.append({"s": round(elapsed, 4), "err": err})
            finally:
                c.close()
    finally:
        t.join()
        wide_client.close()
    wide_elapsed, wide_err = box.get("wide", (0.0, "wide request did not run"))
    return {
        "mode": "hol",
        "model": args.model,
        "wide_texts": args.texts,
        "wide_s": round(wide_elapsed, 4),
        "wide_error": wide_err,
        "narrow": narrow,
        "narrow_latency": percentiles([x["s"] for x in narrow]),
    }


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--host", default="127.0.0.1")
    p.add_argument("--port", type=int, required=True)
    p.add_argument("--model", required=True)
    p.add_argument("--mode", choices=["sweep", "probe", "hol"], required=True)
    p.add_argument("--text", default="a representative search query")
    p.add_argument("--levels", default="1,8,32,128")
    p.add_argument("--requests", type=int, default=200, help="requests per level (sweep)")
    p.add_argument("--background", type=int, default=128)
    p.add_argument("--warmup", type=float, default=1.0)
    p.add_argument("--samples", type=int, default=25)
    p.add_argument("--texts", type=int, default=200, help="texts in the wide command (hol)")
    p.add_argument("--follow", type=int, default=20)
    p.add_argument("--timeout", type=float, default=30.0)
    args = p.parse_args()

    try:
        if args.mode == "sweep":
            result = mode_sweep(args)
        elif args.mode == "probe":
            result = mode_probe(args)
        else:
            result = mode_hol(args)
    except (OSError, RespError) as err:
        print(json.dumps({"error": str(err), "mode": args.mode}), file=sys.stdout)
        return 1
    print(json.dumps(result, indent=1, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
