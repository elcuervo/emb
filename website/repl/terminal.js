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

  /* highlight splits one line of a console into the token classes the site
     already uses for its specimens: a command or a reply's own keyword, a
     quoted string, a number. It is deliberately small -- it is not a shell
     parser and not a Lua one, and it marks only what it can recognise without
     ambiguity, so anything it does not understand comes back as one plain run.

     `command` says the line is something a reader submitted rather than a
     reply. Only a command has a command word at its head and only a command
     carries the reply-format keywords; a reply is matched against the value
     words it can actually contain, so a labelled `POSITIVE` is left alone
     rather than dressed as a command.

     A run is matched whole before it is classified, which is what keeps `sst2`
     a model name and a 40-character digest a digest instead of a shower of
     numbers. */
  var RUN = /("[^"]*"|'[^']*')|([A-Za-z0-9_][A-Za-z0-9_.-]*)/g;
  var NUMBER = /^-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?$/;
  var COMMAND_HEAD = /^[A-Z][A-Z0-9]*(?:\.[A-Z0-9]+)*$/;
  var FORMAT_WORD = /^(VALUES|BLOB)$/;
  var VALUE_WORD = /^(FLOAT64|FLOAT32|FLOAT|INT64|INT|UINT8|STRING|NIL|BOOL|OK|PONG)$/;

  function highlight(text, command) {
    var line = String(text == null ? '' : text);
    var out = [];
    var last = 0;
    var m;
    RUN.lastIndex = 0;
    while ((m = RUN.exec(line)) !== null) {
      if (m.index > last) { out.push({ t: line.slice(last, m.index) }); }
      var run = m[0];
      var kind = null;
      if (m[1]) {
        kind = 'str';
      } else if (NUMBER.test(run)) {
        kind = 'num';
      } else if (m.index === 0 && command && COMMAND_HEAD.test(run)) {
        kind = 'cmd';
      } else if (command ? FORMAT_WORD.test(run) : VALUE_WORD.test(run)) {
        kind = 'cmd';
      }
      out.push(kind ? { t: run, c: kind } : { t: run });
      last = m.index + run.length;
    }
    if (last < line.length) { out.push({ t: line.slice(last) }); }
    return out;
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
    /* A retry timer can outlive the command that set it: a reader who submits
       again while a cold sandbox is retrying must not have the older command's
       reply land on top of the newer one. Every submission bumps `generation`,
       and a callback from an older one is ignored. */
    var generation = 0;
    var maxAttempts = opts.maxAttempts === undefined ? 8 : opts.maxAttempts;
    var retryDelay = opts.retryDelay === undefined ? 1000 : opts.retryDelay;
    var transcript = [];
    var transientStart = -1;
    /* When the reader's command left. It is set at submit rather than at each
       attempt, so a sandbox that has to wake up reports the wait it actually
       cost rather than the last retry's slice of it. */
    var startedAt = 0;

    function nowMs() {
      return global.performance && global.performance.now
        ? global.performance.now()
        : Date.now();
    }

    /* The speed the panel is boasting about, in the form redis-cli prints it.
       Sub-millisecond and second-scale are both real here: a warm reply is a
       few milliseconds and a cold sandbox is tens of seconds. */
    function timing() {
      var ms = nowMs() - startedAt;
      var text = ms < 10 ? (Math.round(ms * 10) / 10) + ' ms'
        : ms < 1000 ? Math.round(ms) + ' ms'
        : (ms / 1000).toFixed(2) + ' s';
      return { t: '(' + text + ')', k: 'time' };
    }

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

    function run(args, gen) {
      set('running');
      request(args).then(function (env) {
        if (gen !== generation) { return; }
        if (env && env.code === 'starting' && attempts < maxAttempts) {
          attempts += 1;
          set('starting', env);
          pushTransient([{ t: 'starting — the sandbox is waking; retrying…', k: 'dim' }]);
          // Back off gently: a cold machine that is loading its models answers
          // "starting" for tens of seconds, and hammering it helps nobody.
          global.setTimeout(function () {
            if (gen === generation) { run(args, gen); }
          }, retryDelay * Math.min(attempts, 4));
          return;
        }
        attempts = 0;
        if (env && env.code === 'unavailable') {
          push([{ t: 'offline — ' + (env.text || 'the sandbox server is unavailable'), k: 'dim' }]);
          set('offline', env);
          return;
        }
        push(format(env).concat([timing()]));
        set(env && env.kind === 'error' ? 'error' : 'result', env);
      }, function () {
        if (gen !== generation) { return; }
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
        generation += 1;
        attempts = 0;
        last = args;
        startedAt = nowMs();
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
        run(args, generation);
      },
      retry: function () {
        if (!last) { set('idle'); return; }
        generation += 1;
        attempts = 0;
        startedAt = nowMs();
        run(last, generation);
      }
    };
  }

  var api = { origin: ORIGIN, tokenize: tokenize, format: format, highlight: highlight, create: create };
  global.embTerminal = api;
  if (typeof module === 'object' && module.exports) { module.exports = api; }
})(typeof window !== 'undefined' ? window : globalThis);
