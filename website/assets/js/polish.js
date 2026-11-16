/* emb — small page polish, shared by the visual plates.
 *
 * Two effects, both optional and both switched off by reduced motion:
 *
 *   revealOnScroll(selector)  fades marked blocks in as they enter the viewport
 *                             (`[data-reveal]` only; nothing is hidden by default)
 *   countUp(node, value, fmt)  runs a displayed integer from 0 to its measured value
 *
 * The motion gate is set on <html> by revealOnScroll, so a page without
 * JavaScript keeps every block visible and every number at its final value.
 */

const reduce = () => window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

export function revealOnScroll(root, selector) {
  const host = root || document;
  const marks = [...host.querySelectorAll(selector || '[data-reveal]')];
  if (!marks.length || reduce()) { return () => {}; }
  document.documentElement.dataset.motion = 'on';
  marks.forEach((node) => node.setAttribute('data-reveal', ''));
  const observer = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) { return; }
      entry.target.classList.add('is-in');
      observer.unobserve(entry.target);
    });
  }, { rootMargin: '0px 0px -6% 0px', threshold: 0.06 });
  marks.forEach((node) => observer.observe(node));
  return () => observer.disconnect();
}

export function countUp(node, value, format, duration) {
  if (!node || !Number.isFinite(value)) { return; }
  const write = format || ((n) => Math.round(n).toLocaleString('en-US'));
  if (reduce()) { node.textContent = write(value); return; }
  const ms = duration || 620;
  const began = performance.now();
  const step = (now) => {
    const t = Math.min(1, (now - began) / ms);
    const eased = 1 - Math.pow(1 - t, 3);
    node.textContent = write(value * eased);
    if (t < 1) { requestAnimationFrame(step); }
  };
  requestAnimationFrame(step);
}
