/* ══════════════════════════════════════════════════════════════════════
   emb — the TL;DR switch
   One preference, remembered, applied to the document element. The full
   page is what paints without this file: the `data-tldr` attribute only
   reveals each surface's `.tldr` line and hides its long form, so nothing
   is lost when scripting is off and the control is never shown as a dead
   button.
   ══════════════════════════════════════════════════════════════════════ */
(function () {
  'use strict';

  var KEY = 'emb.tldr';
  var root = document.documentElement;
  var buttons = document.querySelectorAll('[data-tldr-toggle]');
  if (!buttons.length) { return; }

  function read() {
    try { return window.localStorage.getItem(KEY) === '1'; } catch (e) { return false; }
  }
  function write(on) {
    try { window.localStorage.setItem(KEY, on ? '1' : '0'); } catch (e) { /* private mode */ }
  }
  function paint(on) {
    root.toggleAttribute('data-tldr', on);
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].setAttribute('aria-pressed', on ? 'true' : 'false');
      buttons[i].hidden = false;
    }
  }

  paint(read());
  for (var i = 0; i < buttons.length; i++) {
    buttons[i].addEventListener('click', function () {
      var next = !root.hasAttribute('data-tldr');
      write(next);
      paint(next);
    });
  }
})();
