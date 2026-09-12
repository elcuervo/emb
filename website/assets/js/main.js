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

    /* ── 1. hero entrance ───────────────────────────────────────────
       The wordmark, spine and prose are painted from the start; the
       `is-ready` flag draws the orange spine once the display face is
       in place, so the type never measures itself against a fallback. */
    function enter() {
      requestAnimationFrame(function () { doc.classList.add('is-ready'); });
    }
    if (document.fonts && document.fonts.ready) {
      document.fonts.ready.then(enter, enter);
      window.setTimeout(enter, 1200); /* never wait on a stalled font */
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

    /* ── 4. hover: note ⇄ slab ──────────────────────────────────── */
    [].slice.call(document.querySelectorAll('.note[data-note]')).forEach(function (note) {
      var slab = document.querySelector('.slab[data-slab="' + note.getAttribute('data-note') + '"]');
      if (!slab) return;
      var on = function () { note.classList.add('is-hot'); slab.classList.add('is-hot'); };
      var off = function () { note.classList.remove('is-hot'); slab.classList.remove('is-hot'); };
      note.addEventListener('mouseenter', on);
      note.addEventListener('mouseleave', off);
      note.addEventListener('focusin', on);
      note.addEventListener('focusout', off);
    });

    /* ── 5. the signal becomes a route across the terrain ───────── */
    var frame = document.querySelector('.landscape__plate');
    var routes = [].slice.call(document.querySelectorAll('.route--signal'));
    var land = document.querySelector('.landscape');

    if (frame && routes.length && land && !reduceMotion.matches) {
      routes.forEach(function (r) { r.style.strokeDashoffset = '1'; });
      var ticking = false;

      var drawRoute = function () {
        ticking = false;
        var rect = frame.getBoundingClientRect();
        var vh = window.innerHeight || doc.clientHeight;
        /* 0 when the frame is about to enter, 1 once it is in view */
        var progress = clamp((vh - rect.top) / rect.height, 0, 1);
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
    }

  });
})();
