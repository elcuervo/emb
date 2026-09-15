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
      var protoEl = root.querySelector('#console-proto');
      var tabs = [].slice.call(root.querySelectorAll('.console__mode'));

      var PRESETS = {
        classify: root.getAttribute('data-emb-preset-classify') || ''
      };
      var HINTS = {
        redis: 'EMB minilm VALUES "hello world"',
        scripts: 'EMB.EVSHA sst2 ' + PRESETS.classify + ' 1 "this film is great" NEGATIVE POSITIVE'
      };
      var IDLE = {
        redis: 'Type a command. Try EMB minilm "hello world".',
        scripts: 'Call the preloaded classifier by its digest.'
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
          return;
        }
        fresh.forEach(function (l, i) {
          if (i === 0) { out.appendChild(lineEl(l)); return; }
          timers.push(window.setTimeout(function () { out.appendChild(lineEl(l)); }, i * 90));
        });
      }

      var term = null;
      if (window.embTerminal) {
        term = window.embTerminal.create({
          base: window.embTerminal.origin,
          proto: Number(protoEl && protoEl.value) || 2,
          onLines: paint,
          onState: function (state, detail) {
            var busy = state === 'running' || state === 'starting';
            if (input) input.disabled = busy;
            if (runBtn) runBtn.disabled = busy;
            screen.setAttribute('aria-busy', busy ? 'true' : 'false');
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

      function idle(mode) {
        paint([{ t: IDLE[mode] || IDLE.redis, k: 'dim' }]);
        if (input) {
          input.placeholder = HINTS[mode] || HINTS.redis;
          input.value = '';
        }
      }

      function select(i, focus) {
        var mode = tabs[i].getAttribute('data-mode');
        tabs.forEach(function (tab, j) {
          var on = j === i;
          tab.setAttribute('aria-selected', on ? 'true' : 'false');
          tab.tabIndex = on ? 0 : -1;
        });
        if (term) term.setMode(mode);
        screen.setAttribute('aria-labelledby', tabs[i].id);
        idle(mode);
        if (focus) tabs[i].focus();
      }

      if (form && input && out) {
        form.addEventListener('submit', function (event) {
          event.preventDefault();
          var command = input.value.trim();
          if (!command || !term) return;
          input.value = '';
          term.submit(command);
        });

        tabs.forEach(function (tab, i) {
          tab.addEventListener('click', function () { select(i, false); });
        });

        var tablist = root.querySelector('.console__modes');
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

        if (protoEl) {
          protoEl.addEventListener('change', function () {
            if (term) term.setProto(protoEl.value);
          });
        }

        /* Idle is painted before the live region is armed, so loading the
           page does not announce a hint. Replies after that are announced. */
        if (term) {
          idle('redis');
        } else {
          paint([{ t: 'offline — the sandbox client could not be loaded', k: 'dim' }]);
        }
        out.setAttribute('aria-live', 'polite');
      }
    }

    /* ── 7. the emb-top plate (live) ────────────────────────────────
       The plate plays the recorded take of a real run. `data-topviz-cast`
       carries that take's URL — written by `website/tools/topviz/publish.py` —
       and the player is the vendored one this page serves itself.

       The player builds its terminal into the element it is given, so its
       mount is an empty div of its own and the frame sits beside it. The frame
       is not a fallback to be replaced: it is the still state, and it is what a
       reduced-motion reader, a reader with scripting off, or a plate whose take
       cannot be read, keeps. So this enhancement only swaps the two when it can
       actually play something, and puts the frame back when it cannot. */
    var live = document.querySelector('.topviz__live[data-topviz-cast]');
    var plate = live && live.closest('.topviz__play');
    var still = plate && plate.querySelector('[data-topviz-frame]');
    if (live && !reduceMotion.matches) {
      var cast = live.getAttribute('data-topviz-cast');
      var play = function () {
        /* Below the narrow breakpoint the plate is hidden and the run's figures
           stand in for it: there is nothing on screen to animate, so the take
           is not fetched at all. */
        if (!live.offsetWidth) return;
        if (!window.AsciinemaPlayer || !cast) return;
        var player = window.AsciinemaPlayer.create(cast, live, {
          autoplay: true,
          loop: true,
          controls: true,
          speed: 1.8,
          fit: 'width',
          theme: 'plate',
          poster: 'npt:0:20',
          terminalFontFamily: '"JetBrains Mono", ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
          terminalLineHeight: 1.05
        });
        if (still) still.hidden = true;
        /* A take that will not load or parse must not leave an empty plate: the
           player has already emptied its mount by now, so hand the plate back
           to the frame. */
        player.addEventListener('error', function () {
          player.dispose();
          live.hidden = true;
          if (still) still.hidden = false;
        });
      };
      /* The terminal's grid is measured from the type, so the face has to be
         in place before the player is created; until then, and if the font
         never settles, the frame is what the plate shows. */
      if (document.fonts && document.fonts.ready) {
        document.fonts.ready.then(play, play);
      } else {
        play();
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
