/* emb — the animated flow graph.
 *
 * One SVG/DOM component for the pipeline diagrams the plates share: a row of
 * stage cards, curved wires between them, and small packets that travel the
 * wires to show direction. Values are always set by the caller from the
 * manifest or the run; the graph never invents one.
 *
 * Usage:
 *
 *   import { flowGraph } from '../assets/js/flowgraph.js';
 *   const flow = flowGraph(node, [
 *     { label: 'text', value: 'committed sample' },
 *     { label: 'EMB', value: '384 values' }
 *   ]);
 *   flow.focus(1);              // one stage
 *   flow.through(1);            // stages 0..1
 *   flow.select(2);             // persistent selected stage
 *   flow.setValues(['…', '…']); // same order as the stages
 *   flow.destroy();
 *
 * A stage may carry `note` (one plain line) and `usage` (the command or
 * expression it stands for). With `options.onSelect`, stages become
 * keyboard-operable buttons and the callback receives the index.
 *
 * With reduced motion the wire packets are drawn once at their midpoints and
 * no animation frame runs. With no layout (a hidden area) the paths are rebuilt
 * on the first ResizeObserver tick.
 */

const reduce = () => window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

function node(tag, className, text) {
  const element = document.createElement(tag);
  if (className) { element.className = className; }
  if (text !== undefined) { element.textContent = text; }
  return element;
}

export function flowGraph(host, stages, options) {
  if (!host || !stages || !stages.length) { return null; }
  host.replaceChildren();
  host.classList.add('flow-graph');
  const inner = node('div', 'flow-graph__inner');
  inner.style.setProperty('--flow-n', String(stages.length));
  const wires = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  wires.setAttribute('class', 'flow-graph__wires');
  wires.setAttribute('aria-hidden', 'true');
  const list = node('ol', 'flow-graph__stages');
  list.style.setProperty('--flow-n', String(stages.length));
  inner.append(wires, list);
  host.append(inner);

  const cards = stages.map((stage, index) => {
    const card = node('li', 'flow-graph__stage');
    card.dataset.stage = String(index);
    const head = node('span', 'flow-graph__head');
    head.append(node('span', 'flow-graph__n', String(index + 1).padStart(2, '0')));
    head.append(node('span', 'flow-graph__k', stage.label));
    const value = node('span', 'flow-graph__v', stage.value === undefined ? '—' : stage.value);
    card.append(head, value);
    if (stage.note) { card.append(node('span', 'flow-graph__note', stage.note)); }
    if (stage.usage) { card.append(node('code', 'flow-graph__usage', stage.usage)); }
    list.append(card);
    return { card, value };
  });

  const packetCount = options && options.packets ? options.packets : 2;
  const onSelect = options && typeof options.onSelect === 'function' ? options.onSelect : null;
  if (onSelect) {
    cards.forEach((item, index) => {
      item.card.tabIndex = 0;
      item.card.setAttribute('role', 'button');
      item.card.addEventListener('click', () => { api.select(index); onSelect(index); });
      item.card.addEventListener('keydown', (event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          api.select(index);
          onSelect(index);
        }
      });
    });
  }
  const paths = [];
  const packets = [];
  const reduced = reduce();
  let visible = true;
  let frame = 0;
  let width = 0;
  let height = 0;

  const buildWires = () => {
    const box = inner.getBoundingClientRect();
    width = Math.max(1, Math.round(box.width));
    height = Math.max(1, Math.round(box.height));
    wires.setAttribute('viewBox', '0 0 ' + width + ' ' + height);
    wires.setAttribute('width', width);
    wires.setAttribute('height', height);
    wires.replaceChildren();
    paths.length = 0;
    packets.length = 0;

    for (let i = 0; i < cards.length - 1; i++) {
      const a = cards[i].card.getBoundingClientRect();
      const b = cards[i + 1].card.getBoundingClientRect();
      const x1 = a.right - box.left;
      const x2 = b.left - box.left;
      const y1 = a.top - box.top + a.height / 2;
      const y2 = b.top - box.top + b.height / 2;
      const bend = Math.max(18, (x2 - x1) * 0.42);
      const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      path.setAttribute('d', 'M ' + x1 + ' ' + y1 + ' C ' + (x1 + bend) + ' ' + y1 + ', ' +
        (x2 - bend) + ' ' + y2 + ', ' + x2 + ' ' + y2);
      path.setAttribute('class', 'flow-graph__wire');
      path.setAttribute('data-wire', String(i));
      wires.append(path);
      paths.push(path);

      if (!reduced) {
        for (let p = 0; p < packetCount; p++) {
          const dot = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
          dot.setAttribute('r', '2.6');
          dot.setAttribute('class', 'flow-graph__packet');
          wires.append(dot);
          packets.push({ dot, path, offset: (p + 0.35) / packetCount + i * 0.17, length: 0 });
        }
      }
    }
    packets.forEach((packet) => { packet.length = packet.path.getTotalLength(); });
    if (reduced) { paintPackets(0); }
  };

  const paintPackets = (now) => {
    packets.forEach((packet) => {
      const t = reduced ? 0.5 : ((now * 0.00012 + packet.offset) % 1);
      const point = packet.path.getPointAtLength(t * packet.length);
      packet.dot.setAttribute('cx', point.x);
      packet.dot.setAttribute('cy', point.y);
      packet.dot.setAttribute('opacity', reduced ? '0.45' : String(0.25 + 0.7 * Math.sin(Math.PI * t)));
    });
  };

  const tick = (now) => {
    frame = 0;
    if (!visible || reduced) { return; }
    paintPackets(now);
    frame = requestAnimationFrame(tick);
  };
  const start = () => { if (!frame && visible && !reduced) { frame = requestAnimationFrame(tick); } };

  const resize = new ResizeObserver(() => { buildWires(); start(); });
  resize.observe(inner);
  const intersection = new IntersectionObserver((entries) => {
    visible = entries[0].isIntersecting;
    if (visible) { start(); } else { frame && cancelAnimationFrame(frame); frame = 0; }
  });
  intersection.observe(inner);

  // First layout may be hidden; build as soon as the observer has real size.
  requestAnimationFrame(() => { buildWires(); start(); });

  const api = {
    focus(index) {
      cards.forEach((card, i) => {
        if (i === index) { card.card.setAttribute('data-on', ''); } else { card.card.removeAttribute('data-on'); }
      });
      paths.forEach((path, i) => {
        if (i === index) { path.setAttribute('data-on', ''); } else { path.removeAttribute('data-on'); }
      });
      const dot = cards[index];
      if (dot) {
        dot.card.animate && dot.card.animate(
          [{ transform: 'translateY(2px)' }, { transform: 'translateY(0)' }],
          { duration: 220, easing: 'cubic-bezier(.2,.7,.2,1)' }
        );
      }
      return api;
    },
    through(index) {
      cards.forEach((card, i) => {
        if (i <= index) { card.card.setAttribute('data-on', ''); } else { card.card.removeAttribute('data-on'); }
      });
      paths.forEach((path, i) => {
        if (i < index) { path.setAttribute('data-on', ''); } else { path.removeAttribute('data-on'); }
      });
      return api;
    },
    setValues(values) {
      values.forEach((value, i) => {
        if (cards[i]) { cards[i].value.textContent = value; }
      });
      return api;
    },
    select(index) {
      cards.forEach((item, i) => {
        if (i === index) { item.card.setAttribute('data-selected', ''); }
        else { item.card.removeAttribute('data-selected'); }
      });
      return api;
    },
    reset() {
      cards.forEach((item) => {
        item.card.removeAttribute('data-on');
        item.card.removeAttribute('data-selected');
      });
      paths.forEach((path) => path.removeAttribute('data-on'));
      return api;
    },
    destroy() {
      resize.disconnect();
      intersection.disconnect();
      if (frame) { cancelAnimationFrame(frame); }
      host.replaceChildren();
      host.classList.remove('flow-graph');
    }
  };

  return api;
}
