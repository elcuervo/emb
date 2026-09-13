/* ══════════════════════════════════════════════════════════════════════
   emb — interaction
   Nothing here carries information: every panel, label and route is
   already in the markup. This file only sequences motion, syncs the
   hover states, and draws the terrain route.
   ══════════════════════════════════════════════════════════════════════ */
(function () {
  'use strict';

  var doc = document.documentElement;
  var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  function ready(fn) {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fn, { once: true });
    } else {
      fn();
    }
  }

  function clamp(n, min, max) {
    return n < min ? min : n > max ? max : n;
  }

  ready(function () {
    var pipeline = document.querySelector('.pipeline');
    var menu = document.querySelector('.mobile-nav');
    if (menu) {
      var menuToggle = menu.querySelector('summary');
      document.addEventListener('keydown', function (event) {
        if (event.key === 'Escape' && menu.open) {
          menu.open = false;
          menuToggle.focus();
        }
      });
      document.addEventListener('click', function (event) {
        if (!menu.contains(event.target) || event.target.closest('.mobile-nav a')) {
          menu.open = false;
        }
      });
      window.matchMedia('(max-width: 1000px)').addEventListener('change', function () {
        menu.open = false;
      });
    }

    /* ── 1. hero entrance ───────────────────────────────────────────
       The wordmark, spine and prose are painted from the start; the
       `is-ready` flag draws the orange spine once the display face is
       in place, so the type never measures itself against a fallback. */
    function enter() {
      if (enterTimer) { window.clearTimeout(enterTimer); enterTimer = 0; }
      requestAnimationFrame(function () { doc.classList.add('is-ready'); });
    }
    var enterTimer = 0;
    if (document.fonts && document.fonts.ready) {
      document.fonts.ready.then(enter, enter);
      enterTimer = window.setTimeout(enter, 1200); /* never wait on a stalled font */
    } else {
      enter();
    }

    /* ── 2 + 3. reveals and architecture activation ───────────────
       Position-driven rather than IntersectionObserver-driven: a jump
       link, a fast flick, a full-page capture or a print can move past
       a section without ever intersecting it, and content that is
       simply skipped must never stay invisible. The sweep reveals
       anything at or above the trigger line, including elements the
       reader has already scrolled beyond.

       Order: INPUT → INFERENCE → EMBEDDINGS → SERVE, one slab at a
       time, via the `.is-live` stagger in the stylesheet. */
    var pending = [].slice.call(document.querySelectorAll('[data-reveal]'));
    var pipelineLive = false;
    var sweeping = false;
    var TRIGGER = 0.92; /* fraction of viewport height */

    if (reduceMotion.matches) {
      pending.forEach(function (el) { el.classList.add('in'); });
      pending = [];
      if (pipeline) { pipeline.classList.add('is-live'); pipelineLive = true; }
    }

    function sweep() {
      var vh = window.innerHeight || doc.clientHeight;
      var line = vh * TRIGGER;

      if (pending.length) {
        pending = pending.filter(function (el) {
          if (el.getBoundingClientRect().top < line) {
            el.classList.add('in');
            return false;
          }
          return true;
        });
      }

      if (!pipelineLive && pipeline) {
        if (pipeline.getBoundingClientRect().top < vh * 0.85) {
          pipelineLive = true;
          window.setTimeout(function () { pipeline.classList.add('is-live'); }, 200);
        }
      }

      if (!pending.length && pipelineLive) {
        window.removeEventListener('scroll', queueSweep);
        window.removeEventListener('resize', queueSweep);
      }
    }

    function queueSweep() {
      if (sweeping) return;
      sweeping = true;
      window.requestAnimationFrame(function () {
        sweeping = false;
        sweep();
      });
    }

    if (pending.length || pipeline) {
      window.addEventListener('scroll', queueSweep, { passive: true });
      window.addEventListener('resize', queueSweep, { passive: true });
      window.requestAnimationFrame(sweep);
    }

    /* Scroll and resize cover a reader. They do not cover everything that can
       change the document's height under the trigger line: zoom, an
       orientation change, a late font or image that reflows the page, and
       anything printing or capturing the page in one pass. A ResizeObserver
       on the root catches all of those, so the sweep can never be left
       un-run with content still held at opacity 0. */
    if (window.ResizeObserver) {
      new window.ResizeObserver(queueSweep).observe(doc);
    }

    /* ── 4. hover: note ⇄ slab, both ways ─────────────────────────
       The emphasis is decoration: the stage number and its label are always
       visible, so nothing depends on it. It is wired in both directions
       because the brief asks the layer to answer as well, and on
       `pointerdown` because a touch device never fires mouseenter. There are
       no focus handlers: nothing inside a note is focusable, so a
       focusin/focusout pair could never run. */
    [].slice.call(document.querySelectorAll('.note[data-note]')).forEach(function (note) {
      var name = note.getAttribute('data-note');
      var slab = document.querySelector('.slab[data-slab="' + name + '"]');
      if (!slab) return;
      var on = function () { note.classList.add('is-hot'); slab.classList.add('is-hot'); };
      var off = function () { note.classList.remove('is-hot'); slab.classList.remove('is-hot'); };
      [note, slab].forEach(function (el) {
        el.addEventListener('mouseenter', on);
        el.addEventListener('mouseleave', off);
        el.addEventListener('pointerdown', on);
      });
      document.addEventListener('pointerdown', function (event) {
        if (!note.contains(event.target) && !slab.contains(event.target)) off();
      }, { passive: true });
    });

    /* ── 5. the signal becomes a route across the terrain ─────────
       The band is measured, not the artwork: on a short window the band can
       be taller than the artwork, and the line must still finish. The route
       completes at 86% of the band's travel, which is where the massif is
       fully in view on every viewport we checked — a denominator of the
       whole band leaves the last branch undrawn at the page bottom. */
    var land = document.querySelector('.landscape');
    var routes = [].slice.call(document.querySelectorAll('.route--signal'));

    if (land && routes.length && !reduceMotion.matches) {
      routes.forEach(function (r) { r.style.strokeDashoffset = '1'; });
      var ticking = false;

      var drawRoute = function () {
        ticking = false;
        var rect = land.getBoundingClientRect();
        var vh = window.innerHeight || doc.clientHeight;
        /* 0 when the band is about to enter, 1 once the massif is in view */
        var progress = clamp((vh - rect.top) / (rect.height * 0.86), 0, 1);
        routes.forEach(function (r) {
          var seq = Number(r.getAttribute('data-seq')) || 0;
          /* The trunk (seq 0) is the ridge route and draws across the whole
             travel. Each data branch then runs over the following 58% of it,
             14% apart, so the extra facts arrive in sequence rather than all
             at once. */
          var p = seq ? clamp((progress - seq * 0.14) / 0.58, 0, 1) : progress;
          r.style.strokeDashoffset = String(1 - p);
        });
      };

      var onRouteScroll = function () {
        if (ticking) return;
        ticking = true;
        window.requestAnimationFrame(drawRoute);
      };

      window.addEventListener('scroll', onRouteScroll, { passive: true });
      window.addEventListener('resize', onRouteScroll, { passive: true });
      drawRoute();

      /* A reader who turns the OS motion switch on mid-session should not be
         left with a line that is still being scrubbed by script: stop the
         listener and hand the finished state back to the stylesheet. */
      reduceMotion.addEventListener('change', function () {
        if (!reduceMotion.matches) return;
        window.removeEventListener('scroll', onRouteScroll);
        window.removeEventListener('resize', onRouteScroll);
        routes.forEach(function (r) { r.style.strokeDashoffset = ''; });
      });
    }

    /* ── 6. the console (placeholder) ───────────────────────────────
       Deterministic transcripts, no network, no endpoint. Every line is
       copied from the repository, and each entry names its source so a
       server change has a defined update path:

         README.md §intro          EMB, and EMB ... VALUES
         README.md §Reply formats  the VALUES envelope fields
         README.md §EMB.MULTI      the multi-model reply
         README.md §Example 2      the sst2 classifier reply
         README.md §Operations     EMB.READY
         examples/scripts/sst2.lua the SHA1 below, which is sha1() of that
                                   file's exact bytes -- the same value the
                                   server's scriptSHA() returns

       The executor is the seam. Replace `window.embConsole.exec` with a
       RESP client and the same markup, modes and states drive it. */
    var root = document.querySelector('[data-console="transcript"]');
    if (root) {
      var out = root.querySelector('#console-out');
      var form = root.querySelector('#console-form');
      var input = root.querySelector('#console-input') || root.querySelector('#console-in');
      var runBtn = root.querySelector('.console__run');
      var screen = root.querySelector('#console-screen');
      var tabs = [].slice.call(root.querySelectorAll('.console__mode'));

      var TRANSCRIPTS = {
        redis: {
          hint: 'EMB minilm "hello world"',
          idle: 'Type a command. Try EMB minilm "hello world".',
          entries: [
            { match: /^EMB\s+\S+\s+VALUES(\s|$)/i, lines: [
              { t: '\"dtype\"   FLOAT' },
              { t: '\"shape\"   [1 384]' },
              { t: '\"values\"  [-0.19744610786437988, 0.17766517400741577, …]', k: 'dim' },
              { t: 'self-describing envelope · 384 decimals', k: 'dim' }
            ] },
            { match: /^EMB\s+\S+(\s|$)/i, lines: [
              { t: '\\x7c\\x8e\\x80\\xbd…' },
              { t: '384 float32s × 4 bytes · 1.5 KB bulk string', k: 'dim' }
            ] },
            { match: /^EMB\.MULTI(\s|$)/i, lines: [
              { t: '1) \\x7c\\x8e\\x80\\xbd…   minilm · 384 floats' },
              { t: '2) \\x4a\\x9f\\x31\\xc2…   siglip2 · 768 floats' },
              { t: 'one round trip · MGET-style partial failures', k: 'dim' }
            ] },
            { match: /^EMB\.READY(\s|$)/i, lines: [ { t: 'OK' } ] },
            { match: /^PING(\s|$)/i, lines: [ { t: 'PONG' } ] },
            { match: /^EMB\.HELP(\s|$)|^HELP(\s|$)/i, lines: [
              { t: 'EMB  EMB.MULTI  EMB.MODELS  EMB.INFO  EMB.STATS  MONITOR' },
              { t: 'EMB.READY  EMB.EVAL  EMB.EVSHA  EMB.SCRIPT  EMB.CACHE.FLUSH', k: 'dim' }
            ] }
          ]
        },
        scripts: {
          hint: 'EMB.EVSHA sst2 "77c1…" 1 "this film is great" NEGATIVE POSITIVE',
          idle: 'Load a script once, then call it by SHA. Try EMB.SCRIPT LOAD sst2.',
          entries: [
            { match: /^EMB\.SCRIPT\s+LOAD(\s|$)/i, lines: [
              { t: '"77c1e0c01d3c43e8f07b262869d13c21b93b28f9"' },
              { t: 'compiled, cached per model', k: 'dim' }
            ] },
            { match: /^EMB\.EVSHA(\s|$)/i, lines: [
              { t: 'label       POSITIVE' },
              { t: 'confidence  0.99' },
              { t: 'scores      […]', k: 'dim' },
              { t: 'model(fn(input)) → model output', k: 'dim' }
            ] },
            { match: /^EMB\.SCRIPT\s+EXISTS(\s|$)/i, lines: [ { t: '1' } ] }
          ]
        }
      };

      var state = { mode: 'redis' };
      var timers = [];

      function lineEl(line) {
        var el = document.createElement('span');
        el.className = 'console__line' + (line.k ? ' console__line--' + line.k : '');
        if (line.k === 'echo') {
          var caret = document.createElement('span');
          caret.className = 'console__caret';
          caret.textContent = 'EMB ›';
          el.appendChild(caret);
          el.appendChild(document.createTextNode(' ' + line.t));
        } else {
          el.textContent = line.t;
        }
        return el;
      }

      function clearTimers() {
        timers.forEach(window.clearTimeout);
        timers = [];
      }

      /* Playback is line-by-line, not per character: the panel is a console,
         and a 40-character line typing itself out is noise, not information.
         Reduced motion collapses it to one frame. */
      function play(lines) {
        clearTimers();
        out.textContent = '';
        if (reduceMotion.matches) {
          lines.forEach(function (l) { out.appendChild(lineEl(l)); });
          setBusy(false);
          return;
        }
        lines.forEach(function (l, i) {
          if (i === 0) { out.appendChild(lineEl(l)); return; }
          timers.push(window.setTimeout(function () {
            out.appendChild(lineEl(l));
            if (i === lines.length - 1) setBusy(false);
          }, i * 110));
        });
      }

      function setBusy(busy) {
        if (input) input.disabled = busy;
        if (runBtn) runBtn.disabled = busy;
        screen.setAttribute('aria-busy', busy ? 'true' : 'false');
      }

      function respond(mode, command) {
        var spec = TRANSCRIPTS[mode] || TRANSCRIPTS.redis;
        var cmd = String(command).trim().replace(/\s+/g, ' ');
        var head = cmd.split(' ')[0] || '';
        var lines = [{ k: 'echo', t: cmd }];
        for (var i = 0; i < spec.entries.length; i++) {
          if (spec.entries[i].match.test(cmd)) {
            return lines.concat(spec.entries[i].lines);
          }
        }
        return lines.concat([
          { t: "-ERR unknown command '" + head + "'", k: 'err' },
          { t: 'Try: ' + spec.hint, k: 'dim' }
        ]);
      }

      function idle(mode) {
        var spec = TRANSCRIPTS[mode] || TRANSCRIPTS.redis;
        clearTimers();
        out.textContent = '';
        out.appendChild(lineEl({ t: spec.idle, k: 'dim' }));
        setBusy(false);
        if (input) {
          input.placeholder = spec.hint;
          input.value = '';
        }
      }

      function submit(command) {
        setBusy(true);
        out.textContent = '';
        out.appendChild(lineEl({ k: 'echo', t: command }));
        var result;
        try {
          result = window.embConsole.exec(command, state.mode);
        } catch (err) {
          play([{ t: '-ERR executor failed', k: 'err' }]);
          return;
        }
        Promise.resolve(result).then(function (lines) {
          play(Array.isArray(lines) ? lines : []);
        }, function () {
          play([{ t: '-ERR executor failed', k: 'err' }]);
        });
      }

      /* The default executor. It never touches the network: the transcripts
         above are the whole server. Called as exec(command, mode) so a live
         client knows which command surface it is answering. */
      window.embConsole = window.embConsole || {
        exec: function (command, mode) {
          return new Promise(function (resolve) {
            window.setTimeout(function () {
              resolve(respond(mode || state.mode, command));
            }, reduceMotion.matches ? 0 : 140);
          });
        }
      };

      if (form && input && out) {
        form.addEventListener('submit', function (event) {
          event.preventDefault();
          var command = input.value.trim();
          if (!command) return;
          input.value = '';
          submit(command);
        });

        tabs.forEach(function (tab, i) {
          tab.addEventListener('click', function () { select(i, false); });
        });

        var tablist = root.querySelector('.console__modes');
        function select(i, focus) {
          var mode = tabs[i].getAttribute('data-mode');
          tabs.forEach(function (tab, j) {
            var on = j === i;
            tab.setAttribute('aria-selected', on ? 'true' : 'false');
            tab.tabIndex = on ? 0 : -1;
          });
          state.mode = mode;
          screen.setAttribute('aria-labelledby', tabs[i].id);
          idle(mode);
          if (focus) tabs[i].focus();
        }

        if (tablist) {
          tablist.addEventListener('keydown', function (event) {
            var current = tabs.indexOf(document.activeElement);
            if (current < 0) current = tabs.indexOf(root.querySelector('[aria-selected="true"]'));
            var next = null;
            if (event.key === 'ArrowRight' || event.key === 'ArrowDown') next = (current + 1) % tabs.length;
            else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') next = (current - 1 + tabs.length) % tabs.length;
            else if (event.key === 'Home') next = 0;
            else if (event.key === 'End') next = tabs.length - 1;
            if (next === null) return;
            event.preventDefault();
            select(next, true);
          });
        }

        /* Idle is painted before the live region is armed, so loading the page
           does not announce a console hint. Results and mode hints after that
           are announced. */
        idle('redis');
        out.setAttribute('aria-live', 'polite');
      }
    }

  });
})();
