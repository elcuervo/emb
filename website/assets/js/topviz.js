/* ══════════════════════════════════════════════════════════════════════
   emb — the emb-top plate

   Both surfaces that show the recorded `emb-top` run load this file: the
   landing page, where the plate plays on its own, and the documentation
   surface, where the plate is a still frame until the reader asks for the
   recording. One implementation, two callers, and the difference between
   them is declared on the mount rather than branched on inside here.

   The mount -- `.topviz__live[data-topviz-cast]` -- is deliberately empty:
   the player builds its terminal into the element it is given and empties
   that element first, so nothing a page needs to keep can live inside it.

   The `<pre data-topviz-frame>` beside the mount is not a fallback to be
   tidied away. It is the still state: what a reader with scripting off, a
   reader whose motion preference is `reduce`, and a plate whose take cannot
   be read all keep. It paints before any script runs, so the plate is never
   empty, and it comes back when the take will not load.

   Attributes on the mount, all written by `website/tools/topviz/publish.py`:

     data-topviz-cast="assets/cast/emb-top-<sha8>.cast"  the take to play
     data-topviz-autoplay="1"    play without being asked (landing only)
     data-topviz-loop="1"        loop the take once it ends
     data-topviz-speed="1.8"     playback rate; 1 when unset
     data-topviz-poster="npt:0:20"  the player's own poster frame, optional

   Where `data-topviz-autoplay` is absent the player is created at once and
   held at its poster frame: the plate shows the recording in the dashboard's
   own colours, stopped, and the reader starts it from the player's own
   overlay. Nothing plays without being asked for.
   ══════════════════════════════════════════════════════════════════════ */
(function () {
  'use strict';

  var TERMINAL_FONT = '"JetBrains Mono", ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace';

  function ready(fn) {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fn, { once: true });
    } else {
      fn();
    }
  }

  function optionsFor(mount, autoplay) {
    var options = {
      autoplay: autoplay,
      loop: mount.getAttribute('data-topviz-loop') === '1',
      controls: true,
      speed: parseFloat(mount.getAttribute('data-topviz-speed')) || 1,
      fit: 'width',
      theme: 'plate',
      terminalFontFamily: TERMINAL_FONT,
      terminalLineHeight: 1.05
    };
    var poster = mount.getAttribute('data-topviz-poster');
    if (poster) { options.poster = poster; }
    return options;
  }

  function init(mount) {
    var cast = mount.getAttribute('data-topviz-cast');
    if (!cast) { return; }
    var plate = mount.closest('.topviz__play');
    var still = plate && plate.querySelector('[data-topviz-frame]');
    var player = null;

    /* Hand the plate back to its plain-text frame. The player has already
       emptied the mount by the time this runs, so hiding it is enough. */
    function restore() {
      mount.hidden = true;
      if (still) { still.hidden = false; }
    }

    /* Build the player once, and only where it can actually run. Below the
       narrow breakpoint the plate is hidden and the run's figures stand in for
       it, so there is nothing on screen to draw and the take is not fetched.
       The player is what draws the recording *in its own colours*: the
       `<pre>` beside the mount is `asciinema convert -f txt` output, which
       carries no ANSI codes at all and is there for readers who have no
       scripting, not as the surface's picture of the dashboard. */
    function ensure() {
      if (player) { return player; }
      if (!mount.offsetWidth) { return null; }
      if (!window.AsciinemaPlayer) { return null; }
      try {
        player = window.AsciinemaPlayer.create(cast, mount, optionsFor(mount, autoplay));
      } catch (err) {
        player = null;
        restore();
        return null;
      }
      mount.hidden = false;
      if (still) { still.hidden = true; }
      player.addEventListener('error', function () {
        player.dispose();
        player = null;
        restore();
      });
      return player;
    }

    /* The player is created either way, but once. Where the take is not meant
       to move on its own it is created at once and *held*: the plate is the
       recording's own coloured frame, stopped, and the reader starts it from
       the player's own start overlay -- which is the control a thing of this
       shape is expected to have, and the only one it needs. The landing page's
       plate is the section's argument and moves by itself; that is the
       difference between the two surfaces. */
    var autoplay = mount.getAttribute('data-topviz-autoplay') === '1'
      && !window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    if (!autoplay) {
      ensure();
      return;
    }

    /* The terminal's grid is measured from the type, so the face has to be in
       place before the player is created; until then, and if the font never
       settles, the frame is what the plate shows. */
    if (document.fonts && document.fonts.ready) {
      document.fonts.ready.then(ensure, ensure);
    } else {
      ensure();
    }
  }

  ready(function () {
    var mounts = document.querySelectorAll('[data-topviz-cast]');
    for (var i = 0; i < mounts.length; i++) { init(mounts[i]); }
  });
})();
