/* emb — the demos gallery's client.
 *
 * One module for every plate. It owns three things and nothing else: the
 * sandbox call (the same `POST /api/exec {args, proto}` contract the console
 * uses), the browser-side vector search over the committed index, and the
 * loading states the plates render. A plate never talks to the network itself,
 * so a plate cannot invent a host, a reply, or a ranking.
 *
 * The sandbox origin is never typed here. It is taken from, in order:
 *
 *   1. this module's own `src`, when it was served from another origin (the
 *      way `terminal.js` learns the sandbox's origin);
 *   2. `window.embTerminal.origin`, when the page already loaded that module
 *      from the sandbox;
 *   3. `data-sandbox` on the page's own module tag — the host named once, in
 *      HTML, exactly as the landing names `terminal.js`'s host once. This is the
 *      one that applies here, and `just website-dev` rewrites it to the local
 *      bridge so the working tree and the deployed origin use the same file.
 *
 * Nothing is loaded until a plate asks: the wasm build and the index are
 * fetched on first use, so the gallery index costs no vector machinery.
 *
 *   import { gallery } from '/assets/js/demos.js';
 *   const g = gallery();
 *   const hits = await g.search('minilm', 'a ship in a storm', 7);
 *
 * States, in the console's own vocabulary: `idle`, `starting`, `unavailable`,
 * `capacity`, `error`, `ready`. A plate in any of them shows the condition and
 * offers `retry()`; no plate shows a result it did not get.
 */

import { motionAllowed, mechanism, playStages } from './mechanism.js';

/* The mechanism strip is drawn in its own module so the client page and the
 * documentation can use it without loading this one. Re-exported here because
 * every plate already imports its helpers from this file. */
export { motionAllowed, mechanism, playStages };

const VENDOR = '../vendor/sqlite-wasm-vec-0.1.9/sqlite3-bundler-friendly.mjs';
const MANIFEST = '../demo/manifest.json';
const QUANT = 127;

/* ── the sandbox origin ─────────────────────────────────────────────── */

function scriptTag() {
  return document.querySelector('script[data-sandbox]') ||
    document.querySelector('script[src*="demos.js"]');
}

function resolveOrigin() {
  const page = location.origin;
  const own = import.meta.url && new URL(import.meta.url).origin;
  if (own && own !== page) { return own; }
  const terminal = window.embTerminal && window.embTerminal.origin;
  if (terminal) { return terminal; }
  const tag = scriptTag();
  const named = tag && tag.getAttribute('data-sandbox');
  return named ? named.replace(/\/+$/, '') : own || '';
}

export const origin = resolveOrigin();

/* ── the sandbox call ───────────────────────────────────────────────── */

/* The bridge answers with one envelope discriminated by `kind`, and names its
 * own failure modes with `code`. Those are the states a plate renders. */
function stateOf(response, envelope) {
  const code = envelope && envelope.code;
  if (code === 'starting') { return 'starting'; }
  if (code === 'unavailable') { return 'unavailable'; }
  if (code === 'capacity') { return 'capacity'; }
  if (code === 'timeout') { return 'timeout'; }
  if (code === 'refused') { return 'refused'; }
  if (response && !response.ok) { return 'error'; }
  return null;
}

export class SandboxError extends Error {
  constructor(state, text) {
    super(text || state);
    this.state = state;
  }
}

async function exec(args, bin) {
  let response;
  try {
    response = await fetch(origin + '/api/exec', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(bin && bin.length ? { args: args, proto: 2, bin: bin } : { args: args, proto: 2 })
    });
  } catch (err) {
    throw new SandboxError('unavailable', 'the sandbox could not be reached');
  }
  let envelope;
  try {
    envelope = await response.json();
  } catch (err) {
    throw new SandboxError('error', 'the sandbox answered with something that is not a reply');
  }
  const state = stateOf(response, envelope);
  if (state) { throw new SandboxError(state, envelope && envelope.text); }
  if (envelope && envelope.kind === 'error') {
    throw new SandboxError('refused', envelope.text);
  }
  return envelope;
}

/* envelopeText flattens a scripted reply's envelope into the plain value a page
 * renders: hashes become objects, arrays become arrays, bulk strings become
 * strings, integers stay integers. A script's reply is the server's bytes; this
 * only stops the page from reading a wire format. */
export function envelopeValue(envelope) {
  if (!envelope || typeof envelope !== 'object') { return null; }
  switch (envelope.kind) {
    case 'status': return envelope.text;
    case 'int': return envelope.int;
    case 'double': return envelope.float;
    case 'nil': return null;
    case 'bulk': return envelope.text !== undefined ? envelope.text : envelope.b64;
    case 'array': return (envelope.elems || []).map(envelopeValue);
    case 'map': {
      const pairs = envelope.elems || [];
      const out = {};
      for (let i = 0; i + 1 < pairs.length; i += 2) {
        const key = pairs[i];
        out[key && key.text !== undefined ? key.text : String(key && key.int)] = envelopeValue(pairs[i + 1]);
      }
      return out;
    }
    default: return null;
  }
}

/* RESP2 has no map type, so a scripted reply's hash arrives as a flat
 * field/value array — `["dim", 384, "norm", 1]`. Pairing it back up is what
 * lets a plate read `reply.dim` instead of walking the wire format, and a
 * reply that is not a flat hash (an array of numbers, say) is passed through
 * untouched rather than guessed at. */
export function pairsToObject(items) {
  if (!Array.isArray(items) || items.length === 0 || items.length % 2 !== 0) { return null; }
  const out = {};
  for (let i = 0; i + 1 < items.length; i += 2) {
    if (typeof items[i] !== 'string') { return null; }
    out[items[i]] = items[i + 1];
  }
  return out;
}

/* ── the lazy wasm + index ──────────────────────────────────────────── */

let indexPromise = null;

/* The wasm module is initialised once per *page*, not once per module instance:
 * the sqlite3 build warns (and can wedge) when `sqlite3InitModule` is called
 * again while an initialisation is in flight, and a page may hold more than one
 * instance of this module — a plate that imports it and a second one that does
 * not share the first's closure. The promise is parked on `window` for that
 * reason, and it is the same promise every caller awaits. */
const WASM_PROMISE = '__embDemosWasm';

function loadWasm() {
  if (!window[WASM_PROMISE]) {
    window[WASM_PROMISE] = import(VENDOR).then((mod) => mod.default());
  }
  return window[WASM_PROMISE];
}

function quantize(vector) {
  const out = new Uint8Array(vector.length);
  for (let i = 0; i < vector.length; i++) {
    const scaled = Math.round(Math.max(-1, Math.min(1, vector[i])) * QUANT);
    out[i] = scaled & 0xff;
  }
  return out;
}

function openIndex() {
  if (indexPromise) { return indexPromise; }
  indexPromise = (async () => {
    const [sqlite3, response] = await Promise.all([
      loadWasm(),
      fetch(new URL(MANIFEST, import.meta.url))
    ]);
    if (!response.ok) {
      throw new SandboxError('unavailable', 'the atlas index is not on this origin');
    }
    const manifest = await response.json();
    const bytes = new Uint8Array(await (await fetch(new URL('../demo/' + manifest.asset.db, import.meta.url))).arrayBuffer());

    const db = new sqlite3.oo1.DB(':memory:');
    const pointer = sqlite3.wasm.allocFromTypedArray(bytes);
    const capi = sqlite3.capi;
    const rc = capi.sqlite3_deserialize(
      db.pointer, 'main', pointer, bytes.byteLength, bytes.byteLength,
      capi.SQLITE_DESERIALIZE_FREEONCLOSE
    );
    if (rc !== capi.SQLITE_OK) {
      throw new SandboxError('error', 'the atlas index could not be opened (sqlite-vec rc ' + rc + ')');
    }
    return { sqlite3: sqlite3, db: db, manifest: manifest };
  })();
  return indexPromise;
}

function table(manifest, model) {
  const entry = (manifest.models || []).find((m) => m.name === model);
  if (!entry) {
    throw new SandboxError('error', 'the index holds no table for ' + model +
      '; it was built for ' + (manifest.models || []).map((m) => m.name).join(', '));
  }
  return entry;
}

function rows(db, sql, bind) {
  return db.exec({ sql: sql, bind: bind, rowMode: 'array', returnValue: 'resultRows' });
}

/* ── the gallery ────────────────────────────────────────────────────── */

export function gallery() {
  const listeners = new Set();
  let state = 'idle';
  let detail = null;

  function set(next, why) {
    if (next === state && why === detail) { return; }
    state = next;
    detail = why || null;
    listeners.forEach((fn) => fn(state, detail));
  }

  /* run wraps one attempt: it reports the state, runs it, and maps every
   * failure onto a state a plate can render. A caller either gets a value or a
   * SandboxError; it never gets a fabricated one. */
  async function run(work) {
    set('starting');
    try {
      const value = await work();
      set('ready');
      return value;
    } catch (err) {
      const next = err instanceof SandboxError ? err.state : 'error';
      set(next, err && err.message);
      throw err;
    }
  }

  async function embed(model, text) {
    // The `VALUES` reply is the server's typed tensor: a flat field/value list
    // of dtype, shape and values. Reading it through the field names rather than
    // by position is what keeps this working if the envelope grows a field.
    const flat = envelopeValue(await exec(['EMB', model, 'VALUES', text]));
    const tensor = {};
    if (Array.isArray(flat)) {
      for (let i = 0; i + 1 < flat.length; i += 2) { tensor[flat[i]] = flat[i + 1]; }
    }
    if (!Array.isArray(tensor.values) || !tensor.values.length) {
      throw new SandboxError('error', 'the sandbox returned no vector for ' + model);
    }
    return Float32Array.from(tensor.values.map(Number));
  }

  async function index() { return openIndex(); }

  return {
    get origin() { return origin; },
    get state() { return state; },
    get detail() { return detail; },
    onState(fn) { listeners.add(fn); return () => listeners.delete(fn); },

    /* The index's own numbers, so a caption cannot transcribe them. */
    async manifest() { return (await index()).manifest; },

    /* The model the sandbox says it serves, for a plate that must not name a
     * model the server does not have. */
    async models() {
      const flat = envelopeValue(await exec(['EMB.MODELS']));
      const out = [];
      for (let i = 0; i + 2 < flat.length + 1; i += 3) {
        out.push({ name: flat[i], dimension: Number(flat[i + 1]), status: flat[i + 2] });
      }
      return out;
    },

    /* Text in, the query's own vector out. The corpus is searched locally. */
    embed,

    /* Text in, ranked neighbours out: embed through the sandbox, then a
     * `vec0` k-nearest query over the committed index on this machine. */
    async search(model, text, k) {
      if (!text || !String(text).trim()) {
        throw new SandboxError('error', 'a search needs a query');
      }
      return run(async () => {
        const entry = table((await index()).manifest, model);
        const vector = await embed(model, text);
        const limit = Math.max(1, Math.min(Number(k) || 7, 32));
        const sql = 'SELECT rowid, distance FROM ' + entry.table +
          ' WHERE embedding MATCH vec_int8(?) AND k = ? ORDER BY distance';
        const hit = rows((await index()).db, sql, [quantize(vector), limit]);
        return hit.map((r) => ({ rowid: r[0], distance: r[1], model: model, vector: vector }));
      });
    },

    /* The passages a rowid set names, in the order asked. */
    async passages(rowids) {
      if (!rowids.length) { return []; }
      const db = (await index()).db;
      const marks = rowids.map(() => '?').join(', ');
      const found = rows(db,
        'SELECT rowid, id, work, year, idx, snippet, text FROM passages WHERE rowid IN (' + marks + ')',
        rowids);
      const byRow = new Map(found.map((r) => [r[0], {
        rowid: r[0], id: r[1], work: r[2], year: r[3], index: r[4], snippet: r[5], text: r[6]
      }]));
      return rowids.map((id) => byRow.get(id)).filter(Boolean);
    },

    /* The publication year of every passage, and nothing else: the atlas's
     * year order needs 2 782 years, not 2 782 passages. */
    async years() {
      const db = (await index()).db;
      return new Map(rows(db, 'SELECT rowid, year FROM passages').map((r) => [r[0], r[1]]));
    },

    /* The build-time projection and its hand-named regions. No projection work
     * happens here: the atlas draws what the index was built with. */
    async atlas(model) {
      const { db, manifest } = await index();
      table(manifest, model);
      const points = rows(db, 'SELECT rowid, x, y FROM projection WHERE model = ?', [model])
        .map((r) => ({ rowid: r[0], x: r[1], y: r[2] }));
      const regions = rows(db,
        'SELECT id, label, cx, cy, radius, count, top_works FROM regions WHERE model = ? ORDER BY id',
        [model]).map((r) => ({
          id: r[0], label: r[1], cx: r[2], cy: r[3], radius: r[4], count: r[5],
          works: JSON.parse(r[6])
        }));
      return { model: model, points: points, regions: regions };
    },

    /* A preloaded preset by digest: the sandbox runs its own bytes. The digest
     * comes from the page's stamped attribute, never from this module. */
    async preset(model, sha, texts, args) {
      if (!sha) { throw new SandboxError('error', 'no preset digest was stamped for this plate'); }
      const argv = ['EMB.EVSHA', model, sha, String(texts.length)].concat(texts, args || []);
      const reply = envelopeValue(await run(() => exec(argv)));
      // The server's own contract decides the shape: one text returns the value
      // itself, N texts return one value per text. A single hash and a list of
      // hashes are both arrays on the wire, so the count is the only reliable
      // discriminator -- guessing from the elements would read a one-hash reply
      // as a list of its own fields.
      if (texts.length === 1) { return pairsToObject(reply) || reply; }
      return Array.isArray(reply) ? reply.map((item) => pairsToObject(item) || item) : reply;
    },

    /* The image preset: raw bytes in, a label distribution out. `bytes` is a
     * Uint8Array read from a File; the bridge is told which argument is binary
     * (`bin`) and base64-decodes it back to bytes before emb sees it. Only this
     * preset and only the sandbox's own digest accept binary. */
    async imagePreset(model, sha, bytes, labels) {
      if (!sha) { throw new SandboxError('error', 'no preset digest was stamped for this plate'); }
      const argv = ['EMB.EVSHA', model, sha, '1', toBase64(bytes)].concat(labels);
      const reply = envelopeValue(await run(() => exec(argv, [4])));
      return pairsToObject(reply) || reply;
    },

    /* The raw reply envelope, for a plate that shows the reply's shape. */
    async raw(args) { return exec(args); },

    retry() { indexPromise = null; window[WASM_PROMISE] = null; set('idle'); }
  };
}

/* The gallery's one animation primitive: run `step(t)` with t from 0 to 1 over
 * `ms`, on the next frames. With reduced motion asked for it runs once at t = 1,
 * so every figure is still drawn and no result depends on the animation having
 * run. Plates animate their own attributes rather than depending on CSS
 * transitions of SVG geometry, which not every engine honours. */
export function tween(ms, step, done) {
  if (!motionAllowed()) {
    step(1);
    if (done) { done(); }
    return;
  }
  const start = performance.now();
  function frame(now) {
    const t = Math.min(1, (now - start) / ms);
    step(t);
    if (t < 1) { requestAnimationFrame(frame); } else if (done) { done(); }
  }
  requestAnimationFrame(frame);
}

/* One formatter for every elapsed time a plate shows. The bridge hands the
 * server's own integer microseconds to the page, and a figure should be read at
 * the scale of its magnitude: a 40 µs cache hit stays in microseconds, a 90 ms
 * batch becomes milliseconds instead of "90 000 µs", and a cold multi-second
 * pass becomes seconds. One function, so no two plates can disagree about the
 * unit.
 *
 * A plate whose bars are scaled in raw microseconds passes `unit: 'us'` so the
 * number beside a bar is in the bar's own unit; that pins the reading to
 * microseconds up to a full second (a 5.6 ms miss and a 192 µs hit then share
 * one scale) and only switches to seconds past it. */
export function duration(us, unit) {
  const n = Number(us) || 0;
  if (n >= 1e6 || (unit !== 'us' && n >= 999500)) { return (n / 1e6).toFixed(2) + ' s'; }
  if (unit === 'us' || n < 1000) { return n.toLocaleString('en-US') + ' µs'; }
  return (n / 1e3).toFixed(n < 1e4 ? 1 : 0) + ' ms';
}

export const PROTO = 2;

/* ── the plate's small rendering surface ────────────────────────────── */
/* Every plate draws the same three things: a ruled figure caption carrying the
 * index's numbers, a passage as an attributed quotation, and the state of the
 * instrument. They live here so the six plates cannot drift apart, and they
 * build DOM nodes rather than HTML strings, so a reader's own text is never
 * interpolated into markup. */

export function el(tag, attrs, children) {
  const node = document.createElement(tag);
  if (attrs) {
    Object.keys(attrs).forEach((key) => {
      const value = attrs[key];
      if (value === null || value === undefined) { return; }
      if (key === 'class') { node.className = value; }
      else if (key === 'text') { node.textContent = value; }
      else if (key === 'hidden') { node.hidden = !!value; }
      else if (key.slice(0, 2) === 'on') { node.addEventListener(key.slice(2), value); }
      else { node.setAttribute(key, value); }
    });
  }
  (children || []).forEach((child) => {
    if (child === null || child === undefined) { return; }
    node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
  });
  return node;
}

/* A figure caption: a label and its parts, emphasised values in ink. */
export function figure(label, parts) {
  const children = [el('b', { text: label })];
  (parts || []).forEach((part) => {
    children.push(document.createTextNode(' · '));
    if (part && typeof part === 'object') {
      children.push(el('b', { class: part.key ? 'fig__k' : null, text: String(part.v) }));
    } else {
      children.push(document.createTextNode(String(part)));
    }
  });
  return el('p', { class: 'fig' }, children);
}

/* A passage, set as a quotation: the display face for the text, the mono voice
 * for its work and year. Emphasis is the accent and a rule, never a second
 * hue and never italics-as-gothic. */
export function quotation(passage, rank, hit) {
  const children = [];
  if (rank !== undefined && rank !== null) {
    children.push(el('span', { class: 'quote__n', text: String(rank) }));
  }
  children.push(el('p', { class: 'quote__text', text: passage.text || passage.snippet }));
  children.push(el('p', {
    class: 'quote__src',
    text: (passage.work || '') + (passage.year ? ' · ' + passage.year : '')
  }));
  return el('figure', { class: 'quote' + (hit ? ' quote--hit' : '') }, children);
}

/* The instrument's own state, in the console's vocabulary, with a retry. A
 * plate in any non-ready state shows no result. It fills a stable element
 * rather than replacing it: a plate replaces its state line many times, and an
 * element swapped out from under the caller stops updating after the first. */
export function showState(node, state, message, onRetry) {
  const text = {
    idle: 'ready when you are',
    starting: 'starting — the sandbox is waking; this may take a moment',
    running: 'asking the sandbox…',
    unavailable: 'unavailable — ' + (message || 'the sandbox cannot be reached'),
    capacity: 'at capacity — ' + (message || 'the sandbox is at its rate limit; retry shortly'),
    timeout: 'timeout — ' + (message || 'the sandbox did not answer in time'),
    refused: 'refused — ' + (message || 'the sandbox declined that command'),
    error: 'error — ' + (message || 'something went wrong'),
    ready: ''
  }[state] ?? state;
  const children = [document.createTextNode(text)];
  if (onRetry && state !== 'idle' && state !== 'ready' && state !== 'running') {
    children.push(el('button', { class: 'btn btn--sm', type: 'button', text: 'retry', onclick: onRetry }));
  }
  node.replaceChildren(...children);
}

/* The commands a plate shows: the exact argv it issued, in the site's own
 * command vocabulary, so a reader can run it themselves. */
export function argvLine(args) {
  const line = args.map((arg) => (/\s/.test(arg) ? '"' + arg + '"' : arg)).join(' ');
  return el('div', { class: 'code' }, [
    el('pre', { class: 'code__body' }, [el('code', { class: 't-cmd', text: line })])
  ]);
}

/* ── the code specimens ─────────────────────────────────────────────── */
/* The specimens are written on the page as plain text and marked up here, not
 * by hand: one tokenizer for every plate, using the same four classes the rest
 * of the site already styles — a command, a reply keyword or Lua keyword, a
 * quoted string, a number, a dim comment. A shell comment (`#`, where it
 * opens a line or follows whitespace, so Lua's `#` length operator survives) or
 * a Lua/SQL comment (`--`) is the plate's own annotation, so dimming it is what
 * separates "what is sent" from "what it means" without a second hue. A `<...>`
 * placeholder is dimmed the same way. Anything the tokenizer does not recognise
 * is plain. */
const SPEC_TOKEN = /("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')|((?:^[ \t]*|[ \t])#[^\n]*|--[^\n]*)|(<[^>\n]*>)|(\b\d+(?:\.\d+)?\b)|(\b[A-Z][A-Z0-9_]*(?:\.[A-Z0-9_]+)*\b)|(\b(?:local|function|end|if|then|else|elseif|for|while|do|repeat|until|break|in|return|and|or|not|nil|true|false)\b)/gm;

function markSpecimen(code) {
  const text = code.textContent;
  const fragment = document.createDocumentFragment();
  let last = 0;
  let m;
  SPEC_TOKEN.lastIndex = 0;
  while ((m = SPEC_TOKEN.exec(text)) !== null) {
    if (m.index > last) { fragment.appendChild(document.createTextNode(text.slice(last, m.index))); }
    const cls = m[1] ? 't-str' : m[2] ? 't-dim' : m[3] ? 't-dim' : m[4] ? 't-num' : 't-cmd';
    const span = document.createElement('span');
    span.className = cls;
    span.textContent = m[0];
    fragment.appendChild(span);
    last = m.index + m[0].length;
  }
  if (last < text.length) { fragment.appendChild(document.createTextNode(text.slice(last))); }
  code.replaceChildren(fragment);
}

/* The `BLOB` reply is the server's packed little-endian float32, base64'd by the
 * bridge. Decoding is explicit about the byte order rather than leaning on the
 * host's, so a big-endian browser would still read the server's bytes. */
export function b64Float32(b64) {
  const binary = atob(b64 || '');
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) { bytes[i] = binary.charCodeAt(i); }
  const view = new DataView(bytes.buffer);
  const out = new Float32Array(bytes.length >> 2);
  for (let i = 0; i < out.length; i++) { out[i] = view.getFloat32(i * 4, true); }
  return out;
}

/* The first few bytes of a reply, as the console prints them: proof that the
 * vector is bytes and not a sentence. */
export function hexPreview(bytes, n) {
  const count = Math.min(n || 8, bytes.length);
  const parts = [];
  for (let i = 0; i < count; i++) {
    const h = bytes[i].toString(16);
    parts.push('\\x' + (h.length < 2 ? '0' + h : h));
  }
  return parts.join('') + (bytes.length > count ? '…' : '');
}

/* Base64 in chunks: `String.fromCharCode.apply` on a whole image would blow the
 * argument limit, and the bridge expects standard base64 that its decoder reads
 * back to the same bytes. */
export function toBase64(bytes) {
  let binary = '';
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
  }
  return btoa(binary);
}

/* Every static specimen on the page, once. The module is deferred, so the markup
 * is parsed before this runs; a specimen the module builds later (`argvLine`)
 * already carries its own class and is left alone. With scripting off, the
 * specimen is still the same complete text, just uncoloured. */
document.querySelectorAll('.code__body > code:not([class])').forEach(markSpecimen);
