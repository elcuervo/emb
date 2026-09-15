/* emb — the sandbox terminal client.
 *
 * One module drives both surfaces: the landing page loads it from the sandbox
 * origin, and the sandbox's own terminal page loads the same file. A reply is
 * rendered here, once, so the two surfaces cannot drift apart.
 *
 * Contract:
 *   POST {base}/api/exec   {args: [string...], proto: 2|3}
 *     -> 200  one reply envelope, discriminated by `kind`
 *     -> 503  {"kind":"error","code":"starting"}     server not answering yet
 *     -> 502  {"kind":"error","code":"unavailable"}  server gone
 *     -> 429  {"kind":"error","code":"capacity"}     a spend bound refused it
 *     -> 504  {"kind":"error","code":"timeout"}      upstream deadline passed
 *   A transport failure (fetch rejects) is the offline condition.
 *
 * States: idle -> running -> result | error | starting | offline.
 */
(function (global) {
  'use strict';

  /* The origin this module was served from is the sandbox. Captured here so
     neither page has to hardcode a host in its copy. */
  var self = global.document && global.document.currentScript;
  var ORIGIN = self && self.src ? new URL(self.src).origin : '';

  /* tokenize splits one console line into an argv, honoring single and double
     quotes and backslash escapes, so a text containing spaces survives as one
     argument. It is deliberately not a shell: no globbing, no variables. */
  function tokenize(line) {
    var out = [];
    var cur = '';
    var quote = null;
    var esc = false;
    var started = false;
    var text = String(line == null ? '' : line);
    for (var i = 0; i < text.length; i++) {
      var c = text.charAt(i);
      if (esc) { cur += c; esc = false; started = true; continue; }
      if (quote) {
        if (c === '\\') { esc = true; continue; }
        if (c === quote) { quote = null; continue; }
        cur += c;
        started = true;
        continue;
      }
      if (c === '"' || c === "'") { quote = c; started = true; continue; }
      if (c === ' ' || c === '\t' || c === '\n') {
        if (started) { out.push(cur); cur = ''; started = false; }
        continue;
      }
      cur += c;
      started = true;
    }
    if (esc) { cur += '\\'; }
    if (quote) { throw new Error('unterminated ' + quote + ' quote'); }
    if (started) { out.push(cur); }
    return out;
  }

  function hexPreview(b64) {
    try {
      var bin = global.atob(b64 || '');
      var parts = [];
      for (var i = 0; i < Math.min(4, bin.length); i++) {
        var h = bin.charCodeAt(i).toString(16);
        parts.push('\\x' + (h.length < 2 ? '0' + h : h));
      }
      return parts.join('');
    } catch (err) {
      return '?';
    }
  }

  function bulkSize(b64) {
    var n = Math.floor((String(b64 || '').length * 3) / 4);
    return n >= 1024 ? (n / 1024).toFixed(1) + ' KB' : n + ' B';
  }

  function scalar(env, pad) {
    if (!env || typeof env !== 'object') { return { t: pad + '(nil)', k: 'dim' }; }
    switch (env.kind) {
      case 'status': return { t: pad + env.text };
      case 'error': return { t: pad + '-ERR ' + env.text, k: 'err' };
      case 'int': return { t: pad + String(env.int) };
      case 'double': return { t: pad + String(env.float) };
      case 'nil': return { t: pad + '(nil)', k: 'dim' };
      default: return null;
    }
  }

  /* format renders one reply envelope into console lines of the same shape
     either surface paints: {t: text, k: 'echo'|'dim'|'err'|undefined}. */
  function format(env) {
    var out = [];
    render(env, out, '');
    return out;
  }

  function render(env, out, pad) {
    if (!env || typeof env !== 'object') { out.push({ t: pad + '(nil)', k: 'dim' }); return; }

    if (env.kind === 'bulk') {
      if (env.vector) {
        out.push({ t: pad + hexPreview(env.b64) + '…' });
        out.push({
          t: pad + env.vector.count + ' ' + env.vector.dtype + 's × 4 bytes · ' + bulkSize(env.b64) + ' bulk string',
          k: 'dim'
        });
      } else {
        out.push({ t: pad + (env.text || '') });
      }
      return;
    }

    var line = scalar(env, pad);
    if (line) { out.push(line); return; }

    if (env.kind === 'array') {
      var elems = env.elems || [];
      if (!elems.length) { out.push({ t: pad + '(empty array)', k: 'dim' }); return; }
      for (var i = 0; i < elems.length; i++) {
        var p = pad + (i + 1) + ') ';
        var s = scalar(elems[i], p);
        if (s) { out.push(s); continue; }
        if (elems[i] && elems[i].kind === 'bulk') {
          render(elems[i], out, p);
          continue;
        }
        out.push({ t: p + (elems[i] && elems[i].kind === 'map' ? '{' : '[') });
        render(elems[i], out, pad + '   ');
        out.push({ t: pad + '   ' + (elems[i] && elems[i].kind === 'map' ? '}' : ']') });
      }
      return;
    }

    if (env.kind === 'map') {
      var pairs = env.elems || [];
      if (!pairs.length) { out.push({ t: pad + '(empty map)', k: 'dim' }); return; }
      for (var j = 0; j + 1 < pairs.length; j += 2) {
        var key = pairs[j];
        var label = key && key.text !== undefined ? key.text : String(key && key.int);
        var v = pairs[j + 1];
        var vline = scalar(v, '');
        if (vline) {
          out.push({ t: pad + label + '  ' + vline.t, k: vline.k });
        } else if (v && v.kind === 'bulk') {
          var before = out.length;
          render(v, out, '');
          out.splice(before, 0, { t: pad + label, k: 'key' });
          for (var b = before + 1; b < out.length; b++) { out[b].t = '  ' + out[b].t; }
        } else {
          out.push({ t: pad + label, k: 'key' });
          render(v, out, pad + '  ');
        }
      }
      return;
    }

    out.push({ t: pad + '(unknown reply kind)', k: 'err' });
  }

  /* create wires a console to the sandbox. onState(name, detail) and
     onLines(transcript) are the whole rendering surface; the page owns the
     DOM. onLines always receives the complete transcript, so a transient line
     ("starting…") can be replaced without the page tracking deltas. */
  function create(opts) {
    opts = opts || {};
    var base = String(opts.base === undefined ? ORIGIN : opts.base).replace(/\/+$/, '');
    var onState = opts.onState || function () {};
    var onLines = opts.onLines || function () {};
    var proto = opts.proto === 3 ? 3 : 2;
    var state = 'idle';
    var last = null;
    /* The command history is the REPL half of the client: both surfaces own an
       input, so recall lives here rather than being implemented twice. It is a
       bounded list because a page is open for a long time and nothing prunes
       it. */
    var history = [];
    var maxHistory = 50;
    var cursor = -1; /* one past the end while walking; -1 when not walking */
    var draft = '';
    var attempts = 0;
    var maxAttempts = opts.maxAttempts === undefined ? 8 : opts.maxAttempts;
    var retryDelay = opts.retryDelay === undefined ? 1000 : opts.retryDelay;
    var transcript = [];
    var transientStart = -1;

    function emit() { onLines(transcript.slice()); }

    function set(next, detail) {
      state = next;
      onState(next, detail || {});
    }

    /* push appends a permanent block, dropping any transient block before it. */
    function push(lines) {
      if (transientStart >= 0) { transcript = transcript.slice(0, transientStart); }
      transcript = transcript.concat(lines);
      transientStart = transcript.length;
      emit();
    }

    /* pushTransient replaces the previous transient block in place, so a
       retry does not stack "starting…" lines. */
    function pushTransient(lines) {
      if (transientStart >= 0) { transcript = transcript.slice(0, transientStart); }
      transientStart = transcript.length;
      transcript = transcript.concat(lines);
      emit();
    }

    function request(args) {
      return global.fetch(base + '/api/exec', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ args: args, proto: proto })
      }).then(function (res) {
        return res.json();
      });
    }

    function run(args) {
      set('running');
      request(args).then(function (env) {
        if (env && env.code === 'starting' && attempts < maxAttempts) {
          attempts += 1;
          set('starting', env);
          pushTransient([{ t: 'starting — the sandbox is waking; retrying…', k: 'dim' }]);
          // Back off gently: a cold machine that is loading its models answers
          // "starting" for tens of seconds, and hammering it helps nobody.
          global.setTimeout(function () { run(args); }, retryDelay * Math.min(attempts, 4));
          return;
        }
        attempts = 0;
        if (env && env.code === 'unavailable') {
          push([{ t: 'offline — ' + (env.text || 'the sandbox server is unavailable'), k: 'dim' }]);
          set('offline', env);
          return;
        }
        push(format(env));
        set(env && env.kind === 'error' ? 'error' : 'result', env);
      }, function () {
        push([{ t: 'offline — the sandbox could not be reached', k: 'dim' }]);
        set('offline', { kind: 'error', code: 'offline', text: 'the sandbox could not be reached' });
      });
    }

    return {
      get state() { return state; },
      get proto() { return proto; },
      setProto: function (v) { proto = Number(v) === 3 ? 3 : 2; },
      /* recall walks the history one entry at a time: -1 is older, +1 is newer.
         It stops at both ends rather than wrapping, and the line the reader was
         typing is held as `draft` for the whole walk, so walking away and back
         does not lose it. `current` is the page's input text at the start of
         the walk; the return value is what the page should put in the input. */
      recall: function (dir, current) {
        if (!history.length) { return null; }
        if (cursor < 0) {
          cursor = history.length;
          draft = String(current == null ? '' : current);
        }
        var next = cursor + (dir < 0 ? -1 : 1);
        if (next < 0) { next = 0; }
        if (next > history.length) { next = history.length; }
        cursor = next;
        return cursor === history.length ? draft : history[cursor];
      },
      submit: function (text) {
        var args;
        try {
          args = tokenize(text);
        } catch (err) {
          transcript = [{ t: '-ERR ' + err.message, k: 'err' }];
          transientStart = transcript.length;
          emit();
          set('error', { kind: 'error', code: 'refused', text: err.message });
          return;
        }
        if (!args.length) { return; }
        attempts = 0;
        last = args;
        var line = String(text).trim().replace(/\s+/g, ' ');
        /* A command chosen from the examples is submitted, not run, so it lands
           in the history the same way a typed one does. Consecutive repeats are
           stored once, as in readline: a second identical command immediately
           after the first would otherwise cost two keystrokes to walk past. */
        if (history[history.length - 1] !== line) {
          history.push(line);
          if (history.length > maxHistory) { history.shift(); }
        }
        cursor = -1;
        transcript = [{ t: line, k: 'echo' }];
        transientStart = transcript.length;
        emit();
        run(args);
      },
      retry: function () {
        if (!last) { set('idle'); return; }
        attempts = 0;
        run(last);
      }
    };
  }

  var api = { origin: ORIGIN, tokenize: tokenize, format: format, create: create };
  global.embTerminal = api;
  if (typeof module === 'object' && module.exports) { module.exports = api; }
})(typeof window !== 'undefined' ? window : globalThis);
