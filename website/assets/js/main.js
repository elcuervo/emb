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

  function boot() {
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
      /* The mask band that hides the line behind this plate. It has to travel
         with the plate: leaving it behind shears the line off below a lifted
         plate, which reads as a cut rather than as depth. */
      var hole = document.querySelector('.sig-hole[data-hole="' + name + '"]');
      var on = function () {
        note.classList.add('is-hot');
        slab.classList.add('is-hot');
        if (hole) hole.classList.add('is-hot');
      };
      var off = function () {
        note.classList.remove('is-hot');
        slab.classList.remove('is-hot');
        if (hole) hole.classList.remove('is-hot');
      };
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
    var land = document.querySelector('.terrain');
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
        /* One route, two halves of the same trunk: both run the full travel.
           The staged data branches that used to fork off it are gone, so
           there is no per-path offset left to compute. */
        routes.forEach(function (r) { r.style.strokeDashoffset = String(1 - progress); });
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

    /* ── 6. the console (live) ──────────────────────────────────────
       The executor is `window.embTerminal`: the module the sandbox serves
       at cli.emb.is/terminal.js, and the same module its own terminal page
       loads. A reply is therefore rendered once, for both surfaces.

       Nothing here fabricates a reply. When the module is missing or the
       sandbox cannot be reached, the console says so and offers a retry:
       there is no transcript behind this seam.

       The two preset digests are stamped into the section's attributes by
       `website/tools/stamp-presets.py` from the bytes the server preloaded,
       so a command here cannot name a digest the sandbox does not have. */
    var root = document.querySelector('[data-console="live"]');
    if (root) {
      var out = root.querySelector('#console-out');
      var form = root.querySelector('#console-form');
      var input = root.querySelector('#console-input') || root.querySelector('#console-in');
      var runBtn = root.querySelector('.console__run');
      var screen = root.querySelector('#console-screen');
      var stateEl = root.querySelector('#console-state');
      var statusEl = root.querySelector('#console-status-t');

      var PRESETS = {
        embed: root.getAttribute('data-emb-preset-embed') || '',
        classify: root.getAttribute('data-emb-preset-classify') || ''
      };

      /* The demonstration list is built from the digests the section carries
         rather than typed here: a preset whose bytes change cannot leave the
         console offering a digest the sandbox answers `no such script` to.
         Each row is a submitted command, not a special path, so running one is
         indistinguishable from typing it — including for the history the arrow
         keys walk. */
      /* `t` is the command that is submitted; `d` is the label drawn on the row
         when the command is too long to sit on one line beside its note. The
         digest is elided on the row and printed in full the moment the command
         runs: the row is something to click, and a 40-character SHA spent in a
         menu is a row that wraps for no reader's benefit. */
      var EXAMPLES = [
        { t: 'EMB.HELP', n: 'what it permits' },
        { t: 'EMB minilm "hello world"', n: '384 float32s, as bytes' },
        { t: 'EMB minilm VALUES "hello world"', n: 'the same vector, typed' },
        { t: 'EMB.MULTI minilm "hello world" sst2 "this film is great"', n: 'two models, one call' }
      ];
      if (PRESETS.embed) {
        EXAMPLES.push({
          t: 'EMB.EVSHA minilm ' + PRESETS.embed + ' 1 "hello world" "hello there"',
          d: 'EMB.EVSHA minilm ' + PRESETS.embed.slice(0, 8) + '… 1 "hello world" "hello there"',
          n: 'dim · norm · cosine'
        });
      }
      if (PRESETS.classify) {
        EXAMPLES.push({
          t: 'EMB.EVSHA sst2 ' + PRESETS.classify + ' 1 "this film is great" NEGATIVE POSITIVE',
          d: 'EMB.EVSHA sst2 ' + PRESETS.classify.slice(0, 8) + '… 1 "this film is great" NEGATIVE POSITIVE',
          n: 'labelled reply'
        });
      }
      EXAMPLES.push({ t: 'EMB.MODELS', n: 'what is loaded' });

      /* The strip names the console's own condition. The label is the state's
         name in the reader's vocabulary; the attribute is what the styles read,
         so the indicator's colour is a rule rather than a second string here. */
      var STATUS = {
        idle: 'IDLE',
        running: 'RUNNING',
        starting: 'WAKING',
        result: 'READY',
        error: 'ERROR',
        offline: 'OFFLINE'
      };

      var painted = [];
      var timers = [];

      function sameLine(a, b) { return a && b && a.t === b.t && a.k === b.k; }
      function clearTimers() { timers.forEach(window.clearTimeout); timers = []; }

      /* Lines are plain {t, k} pairs produced by the shared client, so both
         surfaces paint the same thing: the console here, the terminal page
         there. Code highlighting was a transcript-era device and is gone. */
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

      /* Paint the complete transcript, stepping only the lines that arrived
         since the last frame. A transcript that is not an extension of the
         painted one (the client replaced a transient line) repaints whole.
         Reduced motion collapses the step to one frame. */
      function paint(lines) {
        var prefix = painted.length <= lines.length;
        for (var i = 0; prefix && i < painted.length; i++) {
          if (!sameLine(painted[i], lines[i])) prefix = false;
        }
        /* Whether to follow is decided before the transcript grows: a reader at
           the end of a full panel stays there, and a reader who has scrolled
           back through a long reply is not yanked away by the next line. A
           repaint that is not an extension of what is on screen is a new
           command, and its output is followed. */
        var follow = !prefix || atEnd();
        if (!prefix) {
          clearTimers();
          out.textContent = '';
          painted = [];
        }
        if (painted.length === lines.length) return;
        var fresh = lines.slice(painted.length);
        painted = lines.slice();
        if (reduceMotion.matches || fresh.length > 8) {
          fresh.forEach(function (l) { out.appendChild(lineEl(l)); });
          if (follow) { screen.scrollTop = screen.scrollHeight; }
          return;
        }
        fresh.forEach(function (l, i) {
          if (i === 0) {
            out.appendChild(lineEl(l));
            if (follow) { screen.scrollTop = screen.scrollHeight; }
            return;
          }
          timers.push(window.setTimeout(function () {
            out.appendChild(lineEl(l));
            if (follow) { screen.scrollTop = screen.scrollHeight; }
          }, i * 90));
        });
      }

      /* A terminal shows its newest line, and this one has a bounded height, so
         a reply longer than the panel scrolls instead of growing the plate. */
      function atEnd() {
        return screen.scrollHeight - screen.scrollTop - screen.clientHeight < 4;
      }

      /* The example rows are the poster's ruled numbered entry, made operable:
         the row's number is the page's own `01` / `02` ladder, and the button
         carries the command as its accessible name, with the note that says
         what the command returns as part of the same name. */
      function examplesEl() {
        /* The heading and the list go straight into the band: a wrapper here
           would carry the band's own class and its padding twice. */
        var box = document.createDocumentFragment();
        var head = document.createElement('p');
        head.className = 'console__examples-h';
        head.textContent = 'EXAMPLES';
        box.appendChild(head);
        var list = document.createElement('ol');
        list.className = 'console__examples-list';
        EXAMPLES.forEach(function (ex, i) {
          var li = document.createElement('li');
          li.className = 'console__example';
          var num = document.createElement('span');
          num.className = 'console__example-n';
          num.textContent = (i + 1 < 10 ? '0' : '') + (i + 1);
          var cmd = document.createElement('button');
          cmd.type = 'button';
          cmd.className = 'console__example-cmd';
          /* The row is the command, and what it returns, in one name: the two
             texts sit in separate flex boxes, so without this the name would
             be read as one run-on word. */
          cmd.setAttribute('aria-label', ex.t + ' — ' + ex.n);
          /* The command sits in its own box so a long digest can be broken:
             beside the note it is a flex item whose minimum is its longest
             word, and a 40-character SHA is wider than a phone. */
          var label = document.createElement('span');
          label.className = 'console__example-t';
          label.textContent = ex.d || ex.t;
          cmd.appendChild(label);
          var note = document.createElement('span');
          note.className = 'console__example-note';
          note.setAttribute('aria-hidden', 'true');
          note.textContent = ex.n;
          cmd.appendChild(note);
          cmd.addEventListener('click', function () { submitCommand(ex.t); });
          li.appendChild(num);
          li.appendChild(cmd);
          list.appendChild(li);
        });
        box.appendChild(list);
        return box;
      }

      var examplesBox = root.querySelector('#console-examples');
      if (examplesBox) {
        examplesBox.appendChild(examplesEl());
        examplesBox.hidden = false;
      }

      var term = null;
      if (window.embTerminal) {
        term = window.embTerminal.create({
          base: window.embTerminal.origin,
          onLines: paint,
          onState: function (state, detail) {
            var busy = state === 'running' || state === 'starting';
            if (input) input.disabled = busy;
            if (runBtn) runBtn.disabled = busy;
            screen.setAttribute('aria-busy', busy ? 'true' : 'false');
            root.setAttribute('data-state', state);
            if (statusEl) statusEl.textContent = STATUS[state] || STATUS.idle;
            if (!stateEl) return;
            stateEl.hidden = true;
            stateEl.textContent = '';
            if (state === 'offline') {
              stateEl.hidden = false;
              var retry = document.createElement('button');
              retry.type = 'button';
              retry.textContent = 'Retry';
              retry.addEventListener('click', function () { term.retry(); });
              stateEl.appendChild(document.createTextNode((detail && detail.text ? detail.text : 'offline') + ' '));
              stateEl.appendChild(retry);
            }
          }
        });
      } else {
        /* The module did not load, so the sandbox is unreachable: disable
           the controls rather than answer from a transcript. The offline
           line is painted below, after the idle paint that would otherwise
           overwrite it. */
        if (input) input.disabled = true;
        if (runBtn) runBtn.disabled = true;
      }

      function submitCommand(text) {
        if (input) input.value = '';
        if (term) term.submit(text);
      }

      if (form && input && out) {
        form.addEventListener('submit', function (event) {
          event.preventDefault();
          var command = input.value.trim();
          if (!command) return;
          submitCommand(command);
        });

        /* Recall is the REPL's other half. It is feature-detected because the
           client module is served from the sandbox's own origin: a page can be
           newer than the module it loads, and it must still submit a command
           when it is. */
        input.addEventListener('keydown', function (event) {
          if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return;
          if (!term || !term.recall) return;
          var next = term.recall(event.key === 'ArrowUp' ? -1 : 1, input.value);
          if (next === null) return;
          event.preventDefault();
          input.value = next;
          input.setSelectionRange(next.length, next.length);
        });

        /* The live region is armed after the panel is built, so loading the
           page does not announce the example list. Replies are announced. */
        if (term) {
          screen.scrollTop = screen.scrollHeight;
        } else {
          paint([{ t: 'offline — the sandbox client could not be loaded', k: 'dim' }]);
        }
        out.setAttribute('aria-live', 'polite');
      }
    }
  }

  /* The enhancement class is set INLINE in the document, before this file
     loads, and the styles that hold the four plates, the signal spine and
     every `[data-reveal]` section at `opacity: 0` are all gated on it. That
     makes this file and `html.js` a matched pair with no failure path: if the
     script parses but throws before it finishes booting, the hero is
     permanently blank and nothing on the page can recover it. Hand the page
     back to its no-JS state instead, which is complete by design. */
  ready(function () {
    try {
      boot();
    } catch (err) {
      doc.classList.remove('js');
      throw err;
    }
  });
})();
