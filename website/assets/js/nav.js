/* ══════════════════════════════════════════════════════════════════════
   emb — the masthead's mobile panel, and which page this is
   The panel is a native `<details>`, so it opens, closes and takes focus
   with no script at all. Two things a disclosure cannot do on its own are
   here instead: close when the reader taps away or presses Escape, and say
   which of its links is the page they are on. Both need every page that
   carries the panel, which is why this file exists separately from
   `main.js` (the landing's motion) and `tldr.js` (the reading bar).
   ══════════════════════════════════════════════════════════════════════ */
(function () {
  'use strict';

  function boot() {
    var menu = document.querySelector('.mobile-nav');
    if (menu) {
      var summary = menu.querySelector('summary');

      /* Escape closes it and puts focus back where it came from: without this
         a keyboard reader who opens the panel has to tab through it to leave,
         and on a phone there is no Escape at all — the tap below is the one
         that matters there. */
      document.addEventListener('keydown', function (event) {
        if (event.key === 'Escape' && menu.open) {
          menu.open = false;
          summary.focus();
        }
      });

      /* A tap outside, or on one of the panel's own links, closes it. The tap
         that opens it is inside `menu`, so it does not close it again.

         The first branch is the scrim the stylesheet stretches over the
         viewport while the panel is open: its clicks report the panel itself as
         the target, and the panel owns that tap — the page beneath must not act
         on it (measured on /docs/, where an outside tap navigated). */
      document.addEventListener('click', function (event) {
        if (event.target === menu) {
          menu.open = false;
          event.preventDefault();
          return;
        }
        if (!menu.contains(event.target) || event.target.closest('.mobile-nav a')) {
          menu.open = false;
        }
      });

      /* The panel is only shown below this width; leaving it open across a
         resize would strand it. */
      window.matchMedia('(max-width: 1000px)').addEventListener('change', function () {
        menu.open = false;
      });
    }

    /* ── which page is this ─────────────────────────────────────────
       The navs are per-page markup and the plates link relatively, so the
       current page is derived from the resolved address rather than typed
       into twenty files. `link.pathname` is the browser's own resolution,
       which is what makes `../docs/index.html` and `docs/index.html` the same
       answer, and `aria-current` is what a screen reader announces and what
       the stylesheet marks. A link carrying a fragment is left alone:
       `docs/index.html#install` is a place on a page, not this page. */
    var here = location.pathname.replace(/index\.html$/, '');
    var links = document.querySelectorAll('.nav a, .mobile-nav a');
    for (var i = 0; i < links.length; i++) {
      if (links[i].hash) { continue; }
      if (links[i].pathname.replace(/index\.html$/, '') === here) {
        links[i].setAttribute('aria-current', 'page');
      }
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, { once: true });
  } else {
    boot();
  }
})();
