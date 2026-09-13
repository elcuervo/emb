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

  });
})();
