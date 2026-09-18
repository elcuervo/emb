/* ══════════════════════════════════════════════════════════════════════
   emb — the TL;DR switch
   One control per page, and one page only: the mode is a reading choice
   for the page in front of you, so it starts off on every navigation and
   is not remembered. The full page is what paints without this file: the
   `data-tldr` attribute only reveals each surface's `.tldr` line and
   hides its long form, so nothing is lost when scripting is off and the
   control is never shown as a dead button.
   ══════════════════════════════════════════════════════════════════════ */
(function () {
  'use strict';

  var root = document.documentElement;
  var buttons = document.querySelectorAll('[data-tldr-toggle]');
  if (!buttons.length) { return; }

  var on = false;
  var settle = 0;

  function paint(next) {
    on = next;
    root.toggleAttribute('data-tldr', on);
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].setAttribute('aria-pressed', on ? 'true' : 'false');
      buttons[i].hidden = false;
    }
    // One switch, one gesture: a brief flag lets the sheet cross-fade what the
    // attribute just changed, and it clears itself so nothing depends on it.
    root.classList.add('tldr-switching');
    window.clearTimeout(settle);
    settle = window.setTimeout(function () { root.classList.remove('tldr-switching'); }, 420);
  }

  paint(false);
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].addEventListener('click', function () { paint(!on); });
  }
})();
