/* emb — the retail field.
 *
 * One small three.js scene shared by the two retail plates: the committed
 * products (or the held-out reference cloud), the frozen centroids, and the
 * live query. It is deliberately a reading instrument, not a game: the cloud
 * turns slowly, drag turns it, and the only emphasised marks are the query,
 * its nearest region and its two nearest products.
 *
 * The projection is build-time PCA shipped in the index; this module only
 * draws coordinates the page already read. With no WebGL, or reduced motion,
 * it renders the same marks once and leaves the page's tables and bars to
 * carry the argument.
 */

const FALLBACK = { dark: '#111110', ink: '#F3F0E8', muted: '#8C8981', accent: '#FF5A1F' };

function css(name, fallback) {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return value || fallback;
}

function bounds(sets) {
  let min = [Infinity, Infinity, Infinity];
  let max = [-Infinity, -Infinity, -Infinity];
  sets.forEach((set) => (set || []).forEach((point) => {
    if (!point || !Number.isFinite(point.x)) { return; }
    min = [Math.min(min[0], point.x), Math.min(min[1], point.y), Math.min(min[2], point.z)];
    max = [Math.max(max[0], point.x), Math.max(max[1], point.y), Math.max(max[2], point.z)];
  }));
  if (!Number.isFinite(min[0])) { min = [-1, -1, -1]; max = [1, 1, 1]; }
  const centre = [(min[0] + max[0]) / 2, (min[1] + max[1]) / 2, (min[2] + max[2]) / 2];
  const span = Math.max(max[0] - min[0], max[1] - min[1], max[2] - min[2], 1e-6);
  return { centre, scale: 1.9 / span };
}

export async function retailField(node, options) {
  if (!node) { return null; }
  let THREE;
  try {
    THREE = await import('../vendor/three-0.170.0/three.module.min.mjs');
  } catch (err) {
    node.replaceChildren(Object.assign(document.createElement('p'), {
      className: 'retail-note',
      textContent: 'The 3D field module did not load; the distances, bars and tables below carry the same measurements.'
    }));
    return null;
  }

  const colours = {
    dark: css('--dark', FALLBACK.dark),
    ink: css('--fg', FALLBACK.ink),
    muted: css('--muted', FALLBACK.muted),
    accent: css('--accent', FALLBACK.accent)
  };
  const reduce = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  let renderer;
  try {
    renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, powerPreference: 'low-power' });
  } catch (err) {
    node.replaceChildren(Object.assign(document.createElement('p'), {
      className: 'retail-note',
      textContent: 'This browser has no WebGL, so the field is not drawn; the distance bars and tables carry the same measurements.'
    }));
    return null;
  }

  node.classList.add('field');
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  const initial = { w: node.clientWidth || 640, h: node.clientHeight || 420 };
  renderer.setSize(initial.w, initial.h, false);
  node.appendChild(renderer.domElement);
  renderer.domElement.setAttribute('aria-hidden', 'true');

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(colours.dark);
  const camera = new THREE.PerspectiveCamera(34, initial.w / (initial.h || 1), 0.05, 100);
  camera.position.set(0, 0, 4.0);
  const pivot = new THREE.Group();
  scene.add(pivot);

  const { centre, scale } = bounds([
    options.products, options.heldOut, options.centroids, options.extent,
    options.query ? [options.query] : []
  ]);
  const place = (point, array, index) => {
    array[index * 3] = (point.x - centre[0]) * scale;
    array[index * 3 + 1] = (point.y - centre[1]) * scale;
    array[index * 3 + 2] = (point.z - centre[2]) * scale;
  };

  const makePoints = (points, size, colour, opacity) => {
    if (!points || !points.length) { return null; }
    const positions = new Float32Array(points.length * 3);
    points.forEach((point, index) => place(point, positions, index));
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
    const material = new THREE.PointsMaterial({
      color: new THREE.Color(colour), size: size, sizeAttenuation: true,
      transparent: true, opacity: opacity, depthWrite: false
    });
    const cloud = new THREE.Points(geometry, material);
    pivot.add(cloud);
    return cloud;
  };

  makePoints(options.heldOut, 0.011, colours.muted, 0.38);
  const productCloud = makePoints(options.products, 0.018, colours.ink, 0.6);
  const centroidCloud = makePoints(options.centroids, 0.03, colours.muted, 0.85);

  // Three faint axes give the cloud an orientation without adding a grid.
  const axis = new THREE.BufferGeometry();
  axis.setAttribute('position', new THREE.BufferAttribute(new Float32Array([
    -1.05, 0, 0, 1.05, 0, 0, 0, -1.05, 0, 0, 1.05, 0, 0, 0, -1.05, 0, 0, 1.05
  ]), 3));
  pivot.add(new THREE.LineSegments(axis, new THREE.LineBasicMaterial({
    color: new THREE.Color(colours.muted), transparent: true, opacity: 0.22
  })));

  const marker = (colour, radius, opacity) => {
    const material = new THREE.MeshBasicMaterial({ color: new THREE.Color(colour), transparent: true, opacity: opacity });
    const mesh = new THREE.Mesh(new THREE.SphereGeometry(radius, 18, 14), material);
    pivot.add(mesh);
    return mesh;
  };

  const query = marker(colours.accent, 0.032, 0.95);
  const halo = marker(colours.accent, 0.062, 0.16);
  let nearestCentroid = marker(colours.accent, 0.05, 0.9);
  let runnerCentroid = marker(colours.ink, 0.04, 0.55);

  const connector = (colour, opacity) => {
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute('position', new THREE.BufferAttribute(new Float32Array(6), 3));
    const line = new THREE.Line(geometry, new THREE.LineBasicMaterial({
      color: new THREE.Color(colour), transparent: true, opacity: opacity
    }));
    pivot.add(line);
    return line;
  };
  const toNearest = connector(colours.accent, 0.7);
  const toRunner = connector(colours.ink, 0.32);

  const setLine = (line, point) => {
    if (!point || !options.query) { line.visible = false; return; }
    line.visible = true;
    const position = line.geometry.attributes.position;
    const q = [options.query.x, options.query.y, options.query.z];
    const t = [point.x, point.y, point.z];
    const a = [0, 1, 2].map((axis) => (q[axis] - centre[axis]) * scale);
    const b = [0, 1, 2].map((axis) => (t[axis] - centre[axis]) * scale);
    position.setXYZ(0, a[0], a[1], a[2]);
    position.setXYZ(1, b[0], b[1], b[2]);
    position.needsUpdate = true;
  };

  const apply = (state) => {
    const next = state || {};
    if (next.query) {
      const p = [0, 1, 2].map((axis) => ([next.query.x, next.query.y, next.query.z][axis] - centre[axis]) * scale);
      query.position.set(p[0], p[1], p[2]);
      halo.position.set(p[0], p[1], p[2]);
      pulse = 0.25;
    }
    query.visible = !!next.query;
    halo.visible = !!next.query && !reduce;
    const nearest = next.nearest;
    const runner = next.runner;
    if (nearest) {
      const p = [0, 1, 2].map((axis) => ([nearest.x, nearest.y, nearest.z][axis] - centre[axis]) * scale);
      nearestCentroid.position.set(p[0], p[1], p[2]);
      nearestCentroid.visible = true;
    } else { nearestCentroid.visible = false; }
    if (runner) {
      const p = [0, 1, 2].map((axis) => ([runner.x, runner.y, runner.z][axis] - centre[axis]) * scale);
      runnerCentroid.position.set(p[0], p[1], p[2]);
      runnerCentroid.visible = true;
    } else { runnerCentroid.visible = false; }
    setLine(toNearest, nearest);
    setLine(toRunner, runner);
    render();
  };

  let theta = 0.45, phi = 0.18, aimTheta = 0.45, aimPhi = 0.18;
  let pulse = 0.25;
  let dragging = false, lastX = 0, lastY = 0, onScreen = true, frame = 0;

  const draw = () => {
    pivot.rotation.y = theta;
    pivot.rotation.x = phi;
    pulse += (1 - pulse) * 0.14;
    query.scale.setScalar(0.35 + pulse * 0.65);
    if (nearestCentroid.visible) { nearestCentroid.scale.setScalar(0.45 + pulse * 0.55); }
    if (halo.visible) {
      const beat = 1 + Math.sin(performance.now() / 520) * 0.16;
      halo.scale.setScalar(pulse * beat);
    }
    renderer.render(scene, camera);
  };
  const render = () => { if (renderer) { draw(); } };

  const loop = () => {
    frame = 0;
    if (!renderer) { return; }
    theta += (aimTheta - theta) * 0.12;
    phi += (aimPhi - phi) * 0.12;
    if (!dragging && !reduce && onScreen) { aimTheta += 0.0016; }
    draw();
    if (!reduce && onScreen) { frame = requestAnimationFrame(loop); }
  };
  const start = () => { if (!frame) { frame = requestAnimationFrame(loop); } };

  const onPointerDown = (event) => {
    dragging = true; lastX = event.clientX; lastY = event.clientY;
    node.setPointerCapture && node.setPointerCapture(event.pointerId);
  };
  const onPointerMove = (event) => {
    if (!dragging) { return; }
    aimTheta += (event.clientX - lastX) * 0.006;
    aimPhi = Math.max(-1.1, Math.min(1.1, aimPhi + (event.clientY - lastY) * 0.006));
    lastX = event.clientX; lastY = event.clientY;
    start();
  };
  const onPointerUp = () => { dragging = false; };
  renderer.domElement.addEventListener('pointerdown', onPointerDown);
  renderer.domElement.addEventListener('pointermove', onPointerMove);
  renderer.domElement.addEventListener('pointerup', onPointerUp);
  renderer.domElement.addEventListener('pointercancel', onPointerUp);
  renderer.domElement.style.touchAction = 'none';
  renderer.domElement.style.cursor = 'grab';

  const resize = () => {
    const box = { w: node.clientWidth || 640, h: node.clientHeight || 420 };
    renderer.setSize(box.w, box.h, false);
    camera.aspect = box.w / (box.h || 1);
    camera.updateProjectionMatrix();
    render();
  };
  const observer = new ResizeObserver(resize);
  observer.observe(node);
  const visibility = new IntersectionObserver((entries) => {
    onScreen = entries[0].isIntersecting;
    if (onScreen) { start(); } else { render(); }
  });
  visibility.observe(node);

  apply(options);
  start();

  return {
    update(state) { apply(state); start(); },
    destroy() {
      observer.disconnect();
      visibility.disconnect();
      if (frame) { cancelAnimationFrame(frame); }
      if (productCloud) { productCloud.geometry.dispose(); productCloud.material.dispose(); }
      if (centroidCloud) { centroidCloud.geometry.dispose(); centroidCloud.material.dispose(); }
      pivot.traverse((child) => {
        if (child.geometry) { child.geometry.dispose(); }
        if (child.material) { child.material.dispose(); }
      });
      renderer.dispose();
      if (renderer.domElement.parentNode === node) { node.removeChild(renderer.domElement); }
    }
  };
}
