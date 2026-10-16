# Tasks

## 1. The agreement check

- [x] 1.1 Extend `website/tools/published-tree.py` so every take mount
      (`data-topviz-cast`) must carry a recorded-run label naming `emb-top v…`,
      and every one must name the same version; on failure it names each take's
      page and the version it declares. Verify: `python3
      website/tools/published-tree.py` fails on today's tree, naming the pre6
      plates against the fleet plate's pre5.
- [x] 1.2 Prove the check is falsifiable in both directions on a scratch copy:
      an edited copy whose fleet label is set to the single-node version passes,
      and one whose label is removed fails. Verify: both runs' output, quoted in
      the change notes.
- [x] 1.3 State the new invariant in the tool's docstring, including why a
      capture's version is not part of the stamped set. Verify: the docstring
      names the check and the two runs from 1.1 and 1.2 still behave the same.

## 2. The fleet take

- [x] 2.1 Re-record the fleet take with `just website-clusterviz` inside
      `nix develop` (four topviz models, about a minute of load). Verify: the
      served cast and the documentation capture are new content-named files, the
      previous pair is gone, and `website/tools/published-tree.py` passes with
      both plates declaring one `emb-top` version.
- [x] 2.2 Check what the rig wrote into the documentation: the `clusterviz`
      block of `docs/operations.md` and the `fleetviz` region of
      `website/docs/index.html` carry the new names, frame, caption and figures.
      Verify: `git status` shows exactly one fleet cast and one fleet gif, both
      referenced, and the fleet plate's caption names the new run's date and
      version.
- [x] 2.3 Read the re-recorded fleet frames for a sizing line. If they carry
      one, re-record the stress figure with `just website-stressviz`, because its
      title states the sizing lines the frames carried. Verify: the frames file
      and the plate's title agree, and the outcome is recorded in the change
      notes either way.

## 3. The hosted docs surface's wire fields

- [x] 3.1 Update the `EMB.INFO` row of `website/docs/index.html` to name the
      embedding path's dispatch/admission/run timings and runs and the model's
      own cache counters, as `docs/commands.md` and `docs/operations.md` state
      them. Verify: field by field against those two files and against a running
      node's `EMB.INFO` reply.
- [x] 3.2 Update the `MONITOR` row to name `cached_texts`, `attributed`,
      `dispatch_us`, `admit_us` and `run_us`, and that they follow `err` so the
      original six fields keep their positions. Verify: the same comparison, plus
      the row still states the ring bound and the incremental fetch.
- [x] 3.3 Check the surface introduces no field and no scope the shipped code
      does not have: every field it names exists in `internal/server`'s reply
      builders, and no counter is described with a scope the code no longer uses.
      Verify: the comparison recorded in the change notes, and the surface
      rendered through the repository's own loop (`just website`, then the docs
      surface) showing both rows.

## 4. Integration

- [x] 4.1 Correct the verification note in
      `openspec/changes/emb-top-course-of-action/tasks.md`'s task 7.2 to record
      that only the single-node take was re-recorded and that this change carries
      the fleet one. Verify: the note reads true against the artifacts that change
      left behind.
- [x] 4.2 Run the served-tree family together and confirm they agree:
      `stamp-version.py --check`, `stamp-presets.py --check`,
      `stamp-examples.py --check` and `published-tree.py`. Verify: every one
      passes on the final tree.
- [x] 4.3 Open the documentation surface and confirm the fleet plate plays the
      new take, holds its still frame with scripting off, and captions the new
      run. Verify: observed in a browser with scripting on and off.

## Workflow follow-up

- Archive `emb-top-course-of-action` first; its deltas are not yet in
  `openspec/specs/`, and this change's fleet-view claims stack on them.
- Archive this change and verify the synced result.
