/* emb — the mechanism strip.
 *
 * The demos' how-it-works figure, in stages: the steps between a reader's input
 * and the answer, as a run of glyphs joined by the page's rule. It lives apart
 * from the gallery's client so the client page and the documentation can draw
 * the same figure without loading the sandbox machinery.
 *
 * A caller names its steps -- `{ glyph, label }` -- and nothing else; the glyph
 * vocabulary is fixed here so ten plates cannot invent ten visual languages.
 * The accent mark inside a step is the sub-shape that carries its meaning (the
 * one vector that matters, the row a search lands on), and it is the only thing
 * that changes colour when the step is reached.
 */

const MECH_NS = 'http://www.w3.org/2000/svg';

/* The site's committed motion contract, read where a figure needs it: motion is
 * the reader's choice, and a figure that animates must ask. */
export function motionAllowed() {
  const query = window.matchMedia('(prefers-reduced-motion: reduce)');
  return !query.matches;
}

function node(tag, attrs, children) {
  const element = document.createElement(tag);
  Object.keys(attrs || {}).forEach((key) => {
    if (key === 'class') { element.className = attrs[key]; }
    else if (key === 'text') { element.textContent = attrs[key]; }
    else if (key === 'hidden') { element.hidden = !!attrs[key]; }
    else { element.setAttribute(key, attrs[key]); }
  });
  (children || []).forEach((child) => element.appendChild(child));
  return element;
}

function mechNode(tag, attrs, cls) {
  const element = document.createElementNS(MECH_NS, tag);
  Object.keys(attrs || {}).forEach((key) => element.setAttribute(key, attrs[key]));
  if (cls) { element.setAttribute('class', cls); }
  return element;
}

/* Every glyph is drawn in the same 56x44 box, stroke 2, square caps, so a step
 * reads the same wherever it stands. Shapes inherit currentColor, which the
 * stage sets; the accent names the sub-shape that carries the step's meaning. */
function mechGlyph(kind) {
  const svg = mechNode('svg', { class: 'mech__glyph', viewBox: '0 0 56 44', 'aria-hidden': 'true', focusable: 'false' });
  const line = (x1, y1, x2, y2, cls) => svg.appendChild(mechNode('line', { x1, y1, x2, y2, stroke: 'currentColor', 'stroke-width': 2 }, cls));
  const box = (x, y, w, h, cls) => svg.appendChild(mechNode('rect', { x, y, width: w, height: h, fill: 'none', stroke: 'currentColor', 'stroke-width': 2 }, cls));
  const bar = (x, y, w, h, cls) => svg.appendChild(mechNode('rect', { x, y, width: w, height: h, fill: 'currentColor' }, cls));
  const dot = (cx, cy, r, cls) => svg.appendChild(mechNode('circle', { cx, cy, r, fill: 'currentColor' }, cls));
  switch (kind) {
    case 'text': /* a passage */
      line(9, 15, 47, 15); line(9, 22, 37, 22); line(9, 29, 43, 29); break;
    case 'tokens': /* the text cut into subwords */
      for (let i = 0; i < 9; i++) { const x = 9.5 + i * 4.6; line(x, 16, x, 28); } break;
    case 'model': /* the model's own call */
      box(11, 12, 34, 20); dot(20, 22, 2.4); dot(28, 22, 2.4); dot(36, 22, 2.4, 'g-accent-fill'); break;
    case 'vector': /* the numbers it returns */
      line(8, 22, 48, 22);
      [[8, 7], [14, 13], [20, 19], [26, 9], [32, 24, 'g-accent-fill'], [38, 15], [44, 11]].forEach(([x, h, cls]) => bar(x, 22 - h, 3, h * 2, cls)); break;
    case 'search': /* a lookup over the index */
      for (let r = 0; r < 3; r++) { for (let c = 0; c < 4; c++) { dot(12 + c * 10, 13 + r * 9, 2.4, r === 1 && c === 2 ? 'g-accent-fill' : null); } } break;
    case 'rank': /* the rows, in their new order */
      [[9, 38], [9, 30], [9, 44], [9, 24]].forEach(([x, w], i) => bar(x, 11 + i * 7, w, 3, i === 0 ? 'g-accent-fill' : null)); break;
    case 'score': /* one number between two texts */
      line(8, 30, 48, 30); bar(30, 12, 4, 18, 'g-accent-fill'); line(30, 12, 30, 30); break;
    case 'labels': /* a distribution over candidates */
      [[10, 9], [19, 20], [28, 13], [37, 6]].forEach(([x, h], i) => bar(x, 32 - h, 6, h, i === 1 ? 'g-accent-fill' : null));
      line(8, 32, 48, 32); break;
    case 'passes': /* many inputs, one pass */
      for (let i = 0; i < 6; i++) { bar(9 + i * 7, 11, 4, 8); }
      bar(9, 27, 38, 4, 'g-accent-fill'); break;
    case 'cache': /* a remembered answer */
      box(11, 12, 34, 20); dot(28, 22, 5, 'g-accent-fill'); break;
    case 'photo': /* an image beside its labels */
      box(9, 11, 38, 24); dot(19, 19, 3, 'g-accent-fill'); line(11, 33, 24, 21); line(24, 21, 35, 31); line(35, 31, 45, 24); break;
    case 'fanout': /* one call, several reply shapes */
      line(8, 22, 22, 22); [12, 22, 32].forEach((y) => { line(22, 22, 40, y); dot(44, y, 3, y === 22 ? 'g-accent-fill' : null); }); break;
    case 'graph': /* nodes and the edges between them */
      line(16, 14, 40, 18); line(16, 14, 22, 34); line(40, 18, 34, 33); line(22, 34, 34, 33);
      dot(16, 14, 4); dot(40, 18, 4, 'g-accent-fill'); dot(22, 34, 4); dot(34, 33, 4); break;
    case 'wire': /* one call site, the command it puts on the wire */
      box(9, 12, 16, 20); line(28, 22, 44, 22); line(40, 17, 44, 22); line(40, 27, 44, 22);
      dot(17, 22, 2.4, 'g-accent-fill'); break;
    case 'lanes': /* commands drawn to scale on their own lanes */
      bar(9, 11, 30, 4, 'g-accent-fill'); bar(9, 21, 38, 4); bar(9, 31, 22, 4); break;
    case 'pool': /* a pool of connections, one command each */
      dot(14, 12, 3); dot(28, 12, 3); dot(42, 12, 3);
      dot(14, 30, 3, 'g-accent-fill'); dot(28, 30, 3); dot(42, 30, 3); break;
    case 'reply': /* a reply as bytes, or as fields */
      box(9, 12, 16, 20); box(31, 12, 16, 20);
      line(13, 18, 21, 18); line(13, 24, 21, 24);
      line(35, 18, 43, 18); line(35, 21, 43, 21); line(35, 24, 39, 24); break;
    case 'script': /* a function run in the server's own process */
      line(9, 22, 20, 22); line(16, 17, 20, 22); line(16, 27, 20, 22);
      box(20, 10, 16, 24); dot(28, 22, 3, 'g-accent-fill');
      line(36, 22, 47, 22); line(43, 17, 47, 22); line(43, 27, 47, 22); break;
    default: break;
  }
  return svg;
}

/* mechanism(node, stages) builds the strip and returns a small controller: a
 * caller calls `reset()` at the start of a run and `all()` when the answer
 * lands. `label` is the plain name of the step; `glyph` names its drawing. */
export function mechanism(target, stages) {
  const list = node('ol', { class: 'mech__list' });
  const items = [];
  stages.forEach((stage, i) => {
    if (i) { list.appendChild(node('li', { class: 'mech__link', 'aria-hidden': 'true' })); }
    const item = node('li', { class: 'mech__s' }, [
      mechGlyph(stage.glyph),
      node('span', { class: 'mech__name', 'text': stage.label })
    ]);
    items.push(item);
    list.appendChild(item);
  });
  target.replaceChildren(list);
  const links = list.querySelectorAll('.mech__link');
  function on(index) {
    items.forEach((item, i) => item.classList.toggle('is-on', i <= index));
    links.forEach((link, i) => link.classList.toggle('is-on', i < index));
  }
  return {
    count: stages.length,
    reset() { on(-1); },
    on: on,
    all() { on(items.length - 1); }
  };
}

/* playStages walks a strip a step at a time. It is timing, not truth: the
 * caller draws the final state when the real answer arrives, so a slow server
 * never leaves the strip mid-stride and a fast one never outruns its result.
 * With reduced motion asked for, every step is already on. */
export function playStages(rail, per, gap) {
  if (!motionAllowed()) { rail.all(); return Promise.resolve(); }
  return new Promise((resolve) => {
    let i = 0;
    function tick() {
      rail.on(i);
      i += 1;
      if (i < rail.count) { window.setTimeout(tick, per + gap); } else { resolve(); }
    }
    tick();
  });
}
