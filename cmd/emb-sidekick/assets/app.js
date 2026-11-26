(() => {
  'use strict';
  const main = document.getElementById('main');
  const navigation = document.getElementById('navigation');
  const connection = document.getElementById('connection');
  const rangeButtons = document.querySelectorAll('[data-range]');
  const svgNS = 'http://www.w3.org/2000/svg';
  const shortTime = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  let snapshot = null;
  let view = null;
  let userNavigated = false;
  let nodeID = null;
  let modelName = null;
  let windowMinutes = 5;
  let lastReceived = 0;
  let streamError = false;
  let fetchError = false;
  let lastStatus = '';

  function element(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = text;
    return node;
  }
  function append(parent, ...children) {
    parent.append(...children);
    return parent;
  }
  function valid(value) { return typeof value === 'number' && Number.isFinite(value); }
  function number(value) {
    if (!valid(value)) return 'Unavailable';
    // Integers render without layout surprises; a real fraction of one still reads as non-zero.
    if (value > 0 && value < 1) return '<1';
    return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value);
  }
  function measure(value, suffix) { return valid(value) ? `${number(value)} ${suffix}` : 'Unavailable'; }
  function rate(value, suffix) { return measure(value, `${suffix}/s`); }
  function latency(value) {
    if (!valid(value)) return 'Unavailable';
    return value >= 1000 ? measure(value / 1000, 'ms') : measure(value, 'µs');
  }
  function memory(value) { return measure(value, 'MB'); }
  function percent(value) { return measure(valid(value) ? value * 100 : null, '%'); }
  function percentage(value) { return measure(value, '%'); }
  function stamp(value) {
    const time = typeof value === 'number' ? value : Date.parse(value);
    return Number.isFinite(time) ? shortTime.format(time) : 'Unavailable';
  }
  function timeAgo(value) {
    const time = Date.parse(value);
    if (!Number.isFinite(time)) return 'No successful sample';
    const seconds = Math.max(0, Math.floor((Date.now() - time) / 1000));
    return seconds < 60 ? `${seconds}s ago` : `${Math.floor(seconds / 60)}m ${seconds % 60}s ago`;
  }
  function severity(value) { return ['healthy', 'degraded', 'critical', 'no_data'].includes(value) ? value : 'no_data'; }
  function badge(value) {
    const label = severity(value);
    const item = element('span', 'health', label === 'no_data' ? 'No data' : label);
    item.dataset.health = label;
    return item;
  }
  function stateTone(state) {
    if (state === 'connected' || state === 'idle') return 'good';
    if (state === 'stale' || state === 'starting') return 'warn';
    if (state === 'disconnected' || state === 'auth_failed') return 'hot';
    return 'muted';
  }
  // Tone is presentation pressure toward the track's end, not a new threshold claim.
  function pressureTone(fraction) {
    if (!valid(fraction)) return 'muted';
    if (fraction >= 0.9) return 'hot';
    if (fraction >= 0.7) return 'warn';
    return 'good';
  }
  function pair(label, value) {
    const item = element('div');
    append(item, element('dt', '', label), element('dd', '', value));
    return item;
  }
  function facts(items) {
    const list = element('dl', 'fact-list');
    items.forEach(([label, value]) => list.append(pair(label, value)));
    return list;
  }
  function section(title, meta) {
    const wrapper = element('section', 'section');
    const header = element('div', 'section-head');
    append(header, element('h2', '', title), element('p', 'meta', meta));
    wrapper.append(header);
    return wrapper;
  }
  function metric(label, value, note) {
    const cell = element('div', 'metric');
    append(cell, element('span', 'label', label), element('strong', '', value));
    if (note) cell.append(element('small', '', note));
    return cell;
  }
  function stat(label, value, note) {
    const cell = element('div', 'stat');
    append(cell, element('span', 'label', label), element('strong', '', value));
    if (note) cell.append(element('small', '', note));
    return cell;
  }
  function warning(message) {
    return element('p', 'inline-status', message);
  }
  function hero(cells, history) {
    const block = element('section', 'hero');
    const head = element('div', 'hero-head');
    head.append(element('span', 'label', `Right now · ${windowMinutes} min window`));
    const numbers = element('div', 'hero-numbers');
    cells.forEach(cell => numbers.append(cell));
    append(block, head, numbers, spark(history));
    return block;
  }
  function windowPoints(history) {
    const reference = Date.parse(snapshot?.at);
    return (Array.isArray(history) ? history : []).filter(point => {
      const time = Date.parse(point?.at);
      return Number.isFinite(time) && (!Number.isFinite(reference) || (time >= reference - windowMinutes * 60000 && time <= reference));
    });
  }
  function spark(history) {
    const points = windowPoints(history);
    const values = points.map(point => valid(point.req_rate) ? point.req_rate : null);
    const readings = values.filter(valid);
    const latest = readings.length ? readings[readings.length - 1] : null;
    const svg = document.createElementNS(svgNS, 'svg');
    svg.setAttribute('class', 'hero-spark');
    svg.setAttribute('viewBox', '0 0 100 28');
    svg.setAttribute('preserveAspectRatio', 'none');
    svg.setAttribute('role', 'img');
    svg.setAttribute('aria-label', readings.length
      ? `Requests per second over the last ${windowMinutes} min. Latest ${rate(latest, 'req')}. Peak ${rate(Math.max(...readings), 'req')}.`
      : `No measured requests per second in the last ${windowMinutes} min.`);
    if (!readings.length) return svg;
    const times = points.map(point => Date.parse(point.at));
    const max = Math.max(...readings) || 1;
    const x = index => points.length > 1 ? index / (points.length - 1) * 100 : 50;
    const y = value => 26 - Math.max(0, value) / max * 22;
    function item(tag, attrs) {
      const node = document.createElementNS(svgNS, tag);
      Object.entries(attrs).forEach(([key, value]) => node.setAttribute(key, value));
      svg.append(node);
      return node;
    }
    item('line', { class: 'baseline', x1: 0, x2: 100, y1: 27.5, y2: 27.5 });
    const runs = [];
    let run = [];
    values.forEach((value, index) => {
      const gap = index > 0 && (!valid(values[index - 1]) || times[index] - times[index - 1] > 3500);
      if (!valid(value) || gap) {
        if (run.length) runs.push(run);
        run = [];
        if (!valid(value)) return;
      }
      run.push(index);
    });
    if (run.length) runs.push(run);
    runs.forEach(segment => {
      const line = segment.map((index, i) => `${i ? 'L' : 'M'}${x(index).toFixed(2)},${y(values[index]).toFixed(2)}`).join(' ');
      if (segment.length < 2) {
        item('circle', { class: 'end-dot', cx: x(segment[0]).toFixed(2), cy: y(values[segment[0]]).toFixed(2), r: 1.6 });
        return;
      }
      const head = `${x(segment[0]).toFixed(2)},27.5`, tail = `${x(segment[segment.length - 1]).toFixed(2)},27.5`;
      item('path', { class: 'area', d: `M${head} ${line.replace(/^M/, 'L')} L${tail} Z` });
      item('path', { class: 'trace', d: line });
    });
    let last = -1;
    values.forEach((value, index) => { if (valid(value)) last = index; });
    if (last >= 0) item('circle', { class: 'end-dot', cx: x(last).toFixed(2), cy: y(values[last]).toFixed(2), r: 1.6 });
    return svg;
  }
  function meterRow(label, value, fraction, tone) {
    const group = document.createDocumentFragment();
    const row = element('div', 'meter-row');
    append(row, element('span', 'label', label), element('span', 'value', value));
    const track = element('div', 'meter');
    track.dataset.tone = tone;
    track.setAttribute('role', 'img');
    track.setAttribute('aria-label', `${label}: ${value}`);
    const fill = element('span', 'fill');
    fill.style.width = `${Math.max(0, Math.min(100, fraction * 100)).toFixed(1)}%`;
    track.append(fill);
    append(group, row, track);
    return group;
  }
  function titleBand(title, description, health, reason, details) {
    const band = element('header', 'titleband');
    const left = element('div');
    append(left, element('h1', '', title), element('p', 'desc', description));
    const aside = element('div', 'aside');
    aside.append(badge(health));
    if (reason) aside.append(element('p', 'status-copy', reason));
    details.forEach(([label, value]) => {
      const line = element('p', 'meta');
      append(line, document.createTextNode(`${label} `), element('span', 'status-copy', value));
      aside.append(line);
    });
    append(band, left, aside);
    return band;
  }
  function chart(title, history, fields, unit, display) {
    const figure = element('div', 'chart');
    figure.append(element('span', 'chart-label', title));
    const points = windowPoints(history);
    const series = fields.map(field => points.map(point => valid(point[field.key]) ? point[field.key] : null));
    const readings = series.flat().filter(valid);
    const count = points.filter((_, index) => series.some(values => valid(values[index]))).length;
    const first = points.length ? stamp(points[0].at) : 'No sample';
    const last = points.length ? stamp(points[points.length - 1].at) : 'No sample';
    const latest = fields.map((field, index) => {
      const values = series[index];
      const value = values.length ? values[values.length - 1] : null;
      return `${field.label} ${display(value)}`;
    }).join(' · ');
    const summary = count ? `${count} measured ${count === 1 ? 'sample' : 'samples'} · ${first} to ${last} local · ${latest}` : 'No measured samples in this window.';
    figure.append(element('p', 'chart-summary', summary));
    if (!readings.length) return figure;
    const svg = document.createElementNS(svgNS, 'svg');
    const width = window.matchMedia('(max-width: 640px)').matches ? 340 : 600;
    svg.setAttribute('viewBox', `0 0 ${width} 205`);
    svg.setAttribute('role', 'img');
    svg.setAttribute('aria-label', `${title}. ${summary}`);
    const left = 65, right = width - 15, top = 12, bottom = 170;
    const ceiling = Math.max(...readings, 0);
    const max = ceiling || 1;
    const times = points.map(point => Date.parse(point.at));
    const span = Math.max(1, times[times.length - 1] - times[0]);
    const x = index => left + (times[index] - times[0]) / span * (right - left);
    const y = value => bottom - Math.max(0, value) / max * (bottom - top);
    function svgItem(tag, attrs, text) {
      const item = document.createElementNS(svgNS, tag);
      Object.entries(attrs).forEach(([key, value]) => item.setAttribute(key, value));
      if (text !== undefined) item.textContent = text;
      svg.append(item);
      return item;
    }
    [top, (top + bottom) / 2, bottom].forEach((row, index) => {
      svgItem('line', { class: 'gridline', x1: left, x2: right, y1: row, y2: row });
      svgItem('text', { class: 'axis', x: left - 9, y: row + 4, 'text-anchor': 'end' }, number(max * (1 - index / 2)));
    });
    svgItem('text', { class: 'axis', x: left, y: 199 }, first);
    svgItem('text', { class: 'axis', x: right, y: 199, 'text-anchor': 'end' }, last);
    svgItem('text', { class: 'axis', x: right, y: 12, 'text-anchor': 'end' }, unit);
    series.forEach(values => {
      let segment = '';
      values.forEach((value, index) => {
        if (!valid(value)) { segment = ''; return; }
        const gap = index > 0 && times[index] - times[index - 1] > 3500;
        segment += `${!segment || gap ? 'M' : 'L'}${x(index).toFixed(1)},${y(value).toFixed(1)} `;
        if (gap) segment = `M${x(index).toFixed(1)},${y(value).toFixed(1)} `;
        // Separate paths at missing samples and polling gaps; a single-point run remains a dot.
        const nextGap = index === values.length - 1 || !valid(values[index + 1]) || times[index + 1] - times[index] > 3500;
        if (nextGap) {
          if (segment.includes('L')) svgItem('path', { class: 'trace', d: segment });
          else svgItem('circle', { class: 'end-dot', cx: x(index), cy: y(value), r: 2.6 });
          segment = '';
        }
      });
    });
    figure.append(svg);
    return figure;
  }
  function navButton(label, key, selected, action) {
    const button = element('button', '', label);
    button.type = 'button';
    button.dataset.focusKey = key;
    if (selected) button.setAttribute('aria-current', 'page');
    button.addEventListener('click', action);
    return button;
  }
  function renderNavigation(nodes) {
    const links = [];
    if (nodes.length > 1) links.push(navButton('Fleet overview', 'fleet', view === 'fleet', () => { userNavigated = true; view = 'fleet'; modelName = null; render(true); }));
    if (view === 'node') {
      const selected = nodes.find(node => node.id === nodeID);
      if (selected) links.push(navButton(selected.name || selected.address || 'Node', 'node-current', true, () => main.focus()));
    }
    navigation.replaceChildren(...links);
  }
  function rowCell(label, value, className = '') {
    const cell = element('span', className);
    append(cell, element('span', 'cell-label', label), typeof value === 'string' ? document.createTextNode(value) : value);
    return cell;
  }
  function recentEvents(events) {
    const reference = Date.parse(snapshot.at);
    return (Array.isArray(events) ? events : [])
      .filter(event => valid(event.at_us) && (!Number.isFinite(reference) || event.at_us / 1000 >= reference - windowMinutes * 60000))
      .sort((left, right) => right.at_us - left.at_us);
  }
  function eventP95(recent) {
    const sizes = recent.map(event => event.latency_us).filter(valid).sort((left, right) => left - right);
    return { count: sizes.length, value: sizes.length >= 10 ? sizes[Math.ceil(sizes.length * 0.95) - 1] : null };
  }
  function tailSection(node, recent) {
    const rolling = eventP95(recent);
    const tail = section('Tail latency', 'Server percentiles and recent events');
    tail.append(element('p', 'note', 'p50, p95 and p99 come from the node. The event figure is computed from measured request events.'));
    const grid = element('div', 'stat-grid');
    append(grid,
      stat('p50 latency', latency(node.p50_us), 'Server percentile'),
      stat('p95 latency', latency(node.p95_us), 'Server percentile'),
      stat('p99 latency', latency(node.p99_us), 'Server percentile'),
      stat('p95 of recent events', rolling.count >= 10 ? latency(rolling.value) : 'Unavailable',
        rolling.count >= 10 ? `p95 of the last ${number(rolling.count)} measured requests in ${windowMinutes} min` : `${number(rolling.count)} measured requests in ${windowMinutes} min; 10 needed`));
    tail.append(grid);
    return tail;
  }
  function pressureSection(node) {
    const point = node.point || {};
    const pressure = section('Cache, sessions and compute', 'Latest measured point');
    const rows = [];
    if (valid(point.cache_hit_rate)) {
      const hit = point.cache_hit_rate;
      rows.push(meterRow('Cache hit ratio', percentage(hit), hit / 100, pressureTone(1 - hit / 100)));
    }
    const models = Array.isArray(node.models) ? node.models : [];
    let busy = 0, total = 0;
    models.forEach(model => {
      const sample = model.point || {};
      if (valid(sample.sessions_busy) && valid(sample.sessions_total)) {
        busy += sample.sessions_busy;
        total += sample.sessions_total;
      }
    });
    if (total > 0) rows.push(meterRow('Sessions busy', `${number(busy)} / ${number(total)}`, busy / total, pressureTone(busy / total)));
    if (valid(point.err_rate) && valid(point.req_rate) && point.req_rate > 0) {
      const share = point.err_rate / point.req_rate;
      const tone = share === 0 ? 'good' : share >= 0.05 ? 'hot' : 'warn';
      rows.push(meterRow('Error share of requests', percent(share), share, tone));
    }
    if (valid(point.cpu_pct)) {
      const above = point.cpu_pct > 100;
      rows.push(meterRow(above ? 'CPU usage above one core average' : 'CPU usage', `${percentage(point.cpu_pct)} per core`, point.cpu_pct / 100,
        above ? 'hot' : pressureTone(point.cpu_pct / 100)));
    }
    if (!rows.length) pressure.append(warning('No cache, session, error or CPU measurement in the latest point.'));
    rows.forEach(row => pressure.append(row));
    return pressure;
  }
  function fleetView(fleet, nodes) {
    const content = document.createDocumentFragment();
    content.append(hero([
      metric('Requests/s', rate(fleet.req_rate, 'req'), 'Fleet total'),
      metric('Tokens/s', rate(fleet.tok_rate, 'tok'), 'Fleet total'),
      metric('p95 latency', latency(fleet.p95_us), 'Recent completed requests'),
      metric('Nodes', number(fleet.discovered), 'Discovered members')
    ], fleet.history));
    content.append(titleBand('Fleet overview', 'Every discovered node. Select a row to inspect its measured history.', fleet.health, fleet.reason || snapshot.error, [
      ['Membership', `${number(fleet.discovered)} discovered · ${number(fleet.active)} active · ${number(fleet.idle)} idle`],
      ['Orphaned', number(fleet.orphaned)],
      ['Snapshot', `${stamp(snapshot.at)} local`]
    ]));
    const trends = section('Fleet history', `${windowMinutes} min window · measured points only`);
    const graph = element('div', 'chart-grid');
    append(graph,
      chart('Request throughput', fleet.history, [{ key: 'req_rate', label: 'Requests/s' }], 'req/s', value => rate(value, 'req')),
      chart('Token throughput', fleet.history, [{ key: 'tok_rate', label: 'Tokens/s' }], 'tok/s', value => rate(value, 'tok')),
      chart('Tail latency', fleet.history, [{ key: 'p95_us', label: 'Fleet p95' }], 'µs', latency));
    trends.append(graph);
    content.append(trends);
    const membership = section('Fleet membership', `${nodes.length} ${nodes.length === 1 ? 'node' : 'nodes'} · discovery order`);
    if (!nodes.length) membership.append(warning('No nodes discovered yet. Check configured targets and DNS; discovery continues automatically.'));
    else {
      const table = element('div', 'table');
      const header = element('div', 'table-head');
      ['NODE / ADDRESS', 'REACHABILITY', 'REQUESTS', 'SHARE', 'LAST SAMPLE'].forEach(text => header.append(element('span', '', text)));
      table.append(header);
      const total = fleet.req_rate;
      nodes.forEach(node => {
        const state = node.state || 'starting';
        const name = node.name || node.address || 'Unnamed node';
        const row = element('button', 'node-row');
        row.type = 'button';
        row.dataset.focusKey = `node-${node.id}`;
        row.setAttribute('aria-label', `Open ${name}. State ${state.replaceAll('_', ' ')}. Requests ${rate(node.point?.req_rate, 'req')}.`);
        const identity = element('span');
        append(identity, element('span', 'row-name', name), element('span', 'row-sub', `${node.address || 'Address unavailable'}${node.orphaned ? ' · orphaned' : ''}`));
        const reach = element('span', 'row-value');
        const pill = element('span', 'pill', state.replaceAll('_', ' '));
        pill.dataset.tone = stateTone(state);
        append(reach, element('span', 'cell-label', 'Reachability'), badge(node.health), pill);
        append(row, identity, reach, rowCell('Requests', rate(node.point?.req_rate, 'req'), 'row-value'), rowCell('Share of measured requests', valid(total) && total > 0 && ['connected', 'idle'].includes(node.state) && valid(node.point?.req_rate) ? percent(node.point.req_rate / total) : 'Unavailable', 'row-value'), rowCell('Last sample', node.last_ok ? `${stamp(node.last_ok)} · ${timeAgo(node.last_ok)}` : 'No successful sample', 'row-value'));
        row.addEventListener('click', () => { userNavigated = true; view = 'node'; nodeID = node.id; modelName = null; render(true); });
        table.append(row);
      });
      membership.append(table);
    }
    content.append(membership);
    return content;
  }
  function modelView(model) {
    const detail = element('section', 'model-detail');
    const head = element('div', 'section-head');
    append(head, element('h2', '', model.name || 'Unnamed model'), element('p', 'meta', [model.status, valid(model.dim) ? `${number(model.dim)} dimensions` : null, model.pooling, model.quantization].filter(Boolean).join(' · ') || 'Identity unavailable'));
    detail.append(head);
    const metrics = element('div', 'metric-line');
    append(metrics, metric('Requests/s', rate(model.point?.req_rate, 'req'), 'Model throughput'), metric('Tokens/s', rate(model.point?.tok_rate, 'tok'), 'Model throughput'), metric('Lifetime average latency', latency(model.point?.avg_latency_us), 'All completed requests'), metric('Error rate', rate(model.point?.err_rate, 'err'), 'Measured model errors'));
    detail.append(metrics);
    const plots = element('div', 'chart-grid');
    append(plots,
      chart('Model requests', model.history, [{ key: 'req_rate', label: 'Requests/s' }], 'req/s', value => rate(value, 'req')),
      chart('Model tokens', model.history, [{ key: 'tok_rate', label: 'Tokens/s' }], 'tok/s', value => rate(value, 'tok')),
      chart('Lifetime average request latency', model.history, [{ key: 'avg_latency_us', label: 'Lifetime mean latency' }], 'µs', latency),
      chart('Cache hit ratio', model.history, [{ key: 'cache_hit_rate', label: 'Cache hit' }], '%', percentage));
    detail.append(plots);
    const blocks = element('div', 'detail-grid');
    const latencyBlock = element('div', 'detail-block');
    append(latencyBlock, element('h2', '', 'Latency and scheduling'), facts([
      ['p50 / p95 / p99', `${latency(model.p50_us)} / ${latency(model.p95_us)} / ${latency(model.p99_us)}`],
      ['Cumulative dispatch wait', latency(model.point?.dispatch_wait_us)], ['Cumulative admission wait', latency(model.point?.admission_wait_us)], ['Cumulative inference run', latency(model.point?.run_us)], ['Lifetime runs', number(model.point?.runs)],
      ['Busy / total sessions', `${number(model.point?.sessions_busy)} / ${number(model.point?.sessions_total)}`]
    ]));
    const cacheBlock = element('div', 'detail-block');
    append(cacheBlock, element('h2', '', 'Cache and freshness'), facts([
      ['Cache hit ratio', percentage(model.point?.cache_hit_rate)], ['Attribution', model.point?.stale === true ? 'Stale measurement' : model.point?.stale === false ? 'Current measurement' : 'Unavailable'],
      ['Latest point', model.point?.at ? `${stamp(model.point.at)} local` : 'Unavailable']
    ]));
    if (model.advice) cacheBlock.append(advice(model.advice));
    append(blocks, latencyBlock, cacheBlock);
    detail.append(blocks);
    return detail;
  }
  function advice(text) {
    const line = element('p', 'advice');
    append(line, element('strong', '', 'Suggested check'), document.createTextNode(` ${String(text)}`));
    return line;
  }
  const EVENTS_SHOWN = 8;
  function eventView(recent) {
    const shown = recent.slice(0, EVENTS_SHOWN);
    const meta = `${shown.length < recent.length ? `Newest ${shown.length} of ${number(recent.length)}` : `Newest ${shown.length}`} · ${windowMinutes} min window · no payloads`;
    const list = section('Completed requests', meta);
    list.classList.add('events');
    if (!shown.length) { list.append(warning('No completed requests in this window.')); return list; }
    const table = element('div', 'table');
    const header = element('div', 'table-head');
    ['TIME', 'LATENCY', 'MODEL', 'RESULT'].forEach(label => header.append(element('span', '', label)));
    table.append(header);
    shown.forEach(event => {
      const details = element('details');
      details.dataset.eventKey = String(event.at_us);
      const summary = element('summary');
      summary.dataset.focusKey = `event-${event.at_us}`;
      summary.setAttribute('aria-label', `${stamp(event.at_us / 1000)}. ${latency(event.latency_us)}. ${event.model || 'Unattributed model'}. ${event.error ? 'Error' : 'Completed'}. Expand for phase details.`);
      const result = element('span', event.error ? 'pill' : '', event.error ? 'Error' : 'Completed');
      if (event.error) result.dataset.tone = 'hot';
      append(summary, element('span', 'event-time', stamp(event.at_us / 1000)), element('span', '', latency(event.latency_us)), element('span', 'event-model', event.model || 'Unattributed model'), result);
      const body = element('div', 'event-facts');
      body.append(facts([
        ['Cache path', valid(event.cached_texts) ? event.cached_texts > 0 ? `${number(event.cached_texts)} cached texts` : 'No cached texts' : 'Unavailable'],
        ['Attribution', event.attributed === true ? 'Measured' : 'Unavailable'],
        ['Dispatch', event.attributed === true ? latency(event.dispatch_us) : 'Unavailable'],
        ['Admission', event.attributed === true ? latency(event.admit_us) : 'Unavailable'],
        ['Run', event.attributed === true ? latency(event.run_us) : 'Unavailable']
      ]));
      append(details, summary, body);
      table.append(details);
    });
    list.append(table);
    return list;
  }
  function nodeView(node) {
    const content = document.createDocumentFragment();
    const point = node.point || {};
    const recent = recentEvents(node.events);
    content.append(hero([
      metric('Requests/s', rate(point.req_rate, 'req'), 'Node total'),
      metric('Tokens/s', rate(point.tok_rate, 'tok'), 'Node total'),
      metric('Error rate', rate(point.err_rate, 'err'), 'Measured failures'),
      metric('Active requests', number(point.active), 'In flight now')
    ], node.history));
    content.append(titleBand(node.name || node.address || 'Unnamed node', `${node.address || 'Address unavailable'} · ${node.orphaned ? 'orphaned peer · ' : ''}${(node.state || 'starting').replaceAll('_', ' ')}`, node.health, node.reason, [
      ['Last success', node.last_ok ? `${stamp(node.last_ok)} local · ${timeAgo(node.last_ok)}` : 'No successful sample'],
      ['Sample', point.at ? `${stamp(point.at)} local` : 'No measured point']
    ]));
    const secondary = element('div', 'metric-line');
    append(secondary,
      metric('CPU usage', percentage(point.cpu_pct), 'Average per core'),
      metric('Memory', memory(point.mem_mb), 'Resident memory'),
      metric('Connections', number(point.conns), 'Open client connections'),
      metric('Uptime', valid(point.uptime_secs) ? `${number(point.uptime_secs / 60)} min` : 'Unavailable', 'Since process start'));
    content.append(secondary);
    content.append(pressureSection(node));
    content.append(tailSection(node, recent));
    const traffic = section('Traffic history', `${windowMinutes} min window · measured points only`);
    const charts = element('div', 'chart-grid');
    append(charts,
      chart('Requests', node.history, [{ key: 'req_rate', label: 'Requests/s' }], 'req/s', value => rate(value, 'req')),
      chart('Tokens', node.history, [{ key: 'tok_rate', label: 'Tokens/s' }], 'tok/s', value => rate(value, 'tok')),
      chart('Memory', node.history, [{ key: 'mem_mb', label: 'Memory MB' }], 'MB', memory),
      chart('CPU usage', node.history, [{ key: 'cpu_pct', label: 'CPU %' }], '%', percentage),
      chart('Cache hit ratio', node.history, [{ key: 'cache_hit_rate', label: 'Cache hit' }], '%', percentage),
      chart('Connections', node.history, [{ key: 'conns', label: 'Connections' }], 'count', value => number(value)),
      chart('Active requests', node.history, [{ key: 'active', label: 'In flight' }], 'requests', value => number(value)));
    traffic.append(charts);
    content.append(traffic);
    const diagnostics = element('section', 'detail-grid');
    diagnostics.setAttribute('aria-label', 'Node signals');
    const service = element('div', 'detail-block');
    append(service, element('h2', '', 'Cache traffic'), facts([
      ['Cache hits', rate(point.cache_hit_ps, 'hits')],
      ['Cache misses', rate(point.cache_miss_ps, 'misses')],
      ['Cache evictions', rate(point.cache_evict_ps, 'evictions')]
    ]));
    const compute = element('div', 'detail-block');
    append(compute, element('h2', '', 'Compute budget'), facts([
      ['CPU token budget', point.cpu_plan_known === true ? number(point.cpu_budget) : 'Unavailable'],
      ['Active CPU tokens', point.cpu_plan_known === true ? number(point.cpu_active_tokens) : 'Unavailable']
    ]));
    if (node.advice) compute.append(advice(node.advice));
    append(diagnostics, service, compute);
    content.append(diagnostics);
    const models = Array.isArray(node.models) ? node.models : [];
    const modelSection = section('Loaded models', `${models.length} ${models.length === 1 ? 'model' : 'models'}`);
    if (!models.length) modelSection.append(warning('No models reported by this node.'));
    else {
      const table = element('div', 'table');
      const header = element('div', 'table-head');
      ['MODEL / STATUS', 'REQUESTS', 'LIFETIME AVG', 'ERRORS', 'SESSIONS'].forEach(label => header.append(element('span', '', label)));
      table.append(header);
      models.forEach(model => {
        const row = element('button', 'model-row');
        row.type = 'button';
        row.dataset.focusKey = `model-${model.name}`;
        row.setAttribute('aria-pressed', String(model.name === modelName));
        row.setAttribute('aria-label', `Open model ${model.name || 'unnamed'}. ${rate(model.point?.req_rate, 'req')}. Lifetime average latency ${latency(model.point?.avg_latency_us)}.`);
        const identity = element('span');
        append(identity, element('span', 'row-name', model.name || 'Unnamed model'), element('span', 'row-sub', model.status || 'Status unavailable'));
        append(row, identity, rowCell('Requests', rate(model.point?.req_rate, 'req'), 'row-value'), rowCell('Lifetime average latency', latency(model.point?.avg_latency_us), 'row-value'), rowCell('Errors', rate(model.point?.err_rate, 'err'), 'row-value'), rowCell('Sessions busy / total', `${number(model.point?.sessions_busy)} / ${number(model.point?.sessions_total)}`, 'row-value'));
        row.addEventListener('click', () => { modelName = model.name === modelName ? null : model.name; render(false); });
        table.append(row);
      });
      modelSection.append(table);
      const selected = models.find(model => model.name === modelName);
      if (selected) modelSection.append(modelView(selected));
    }
    content.append(modelSection, eventView(recent));
    return content;
  }
  function render(moveFocus = false) {
    if (!snapshot) return;
    const focused = document.activeElement?.dataset?.focusKey;
    const expanded = new Set([...main.querySelectorAll('details[open]')].map(item => item.dataset.eventKey));
    const nodes = Array.isArray(snapshot.nodes) ? snapshot.nodes : [];
    if (!userNavigated && nodes.length > 1) view = 'fleet';
    else if (nodes.length === 1 && view !== 'node') view = 'node';
    if (view === 'node' && !nodes.some(node => node.id === nodeID)) {
      nodeID = nodes[0]?.id ?? null;
      modelName = null;
    }
    renderNavigation(nodes);
    let content;
    if (!nodes.length) {
      content = element('section', 'waiting');
      append(content, element('h1', '', 'Waiting for nodes.'), element('p', '', snapshot.error || 'No nodes discovered yet. The monitor keeps resolving configured targets.'));
    } else if (view === 'fleet') content = fleetView(snapshot.fleet || {}, nodes);
    else content = nodeView(nodes.find(node => node.id === nodeID) || nodes[0]);
    const banner = element('p', 'inline-status');
    banner.id = 'freshness';
    main.replaceChildren(banner, content);
    main.querySelectorAll('details').forEach(item => { if (expanded.has(item.dataset.eventKey)) item.open = true; });
    updateConnection();
    if (moveFocus) { main.focus(); window.scrollTo(0, 0); }
    else if (focused) {
      const target = [...navigation.querySelectorAll('[data-focus-key]'), ...main.querySelectorAll('[data-focus-key]')].find(item => item.dataset.focusKey === focused);
      if (target) target.focus({ preventScroll: true });
    }
  }
  function updateConnection() {
    const sampleAt = Date.parse(snapshot?.at);
    const old = lastReceived && Date.now() - lastReceived > 5000;
    const sourceOld = Number.isFinite(sampleAt) && Date.now() - sampleAt > 5000;
    let state = !snapshot ? fetchError ? 'error' : 'connecting' : streamError || old || sourceOld ? 'stale' : lastReceived ? 'live' : 'connecting';
    const text = state === 'live' ? 'Live measurements' : state === 'stale' ? streamError ? 'Stream interrupted · reconnecting' : 'Sampling paused · waiting for data' : state === 'error' ? 'Snapshot unavailable · reconnecting' : 'Connecting live stream';
    if (text !== lastStatus) {
      const ageText = element('span', 'age');
      ageText.setAttribute('aria-hidden', 'true');
      connection.replaceChildren(document.createTextNode(text), ageText);
      connection.dataset.state = state;
      lastStatus = text;
    }
    const age = connection.querySelector('.age');
    if (age) age.textContent = snapshot?.at ? `· last snapshot ${timeAgo(snapshot.at)}` : '';
    const banner = document.getElementById('freshness');
    if (banner) {
      banner.textContent = state === 'live' ? `Latest complete snapshot · ${stamp(snapshot.at)} local · updates continue automatically.` :
        state === 'stale' ? `Previous measurements only · last snapshot ${stamp(snapshot.at)} local. Reconnecting.` :
          `Last available snapshot · ${stamp(snapshot.at)} local. Waiting for the live stream.`;
      const nodes = Array.isArray(snapshot.nodes) ? snapshot.nodes : [];
      const selected = view === 'node' ? nodes.find(node => node.id === nodeID) : null;
      const impaired = selected ? !['connected', 'idle'].includes(selected.state) : nodes.some(node => !['connected', 'idle'].includes(node.state));
      if (impaired) banner.textContent += selected ? ` Node state: ${selected.state || 'starting'}; last-known values may be historical.` : ' Some nodes are not currently connected; inspect their rows for last successful samples.';
      if (snapshot.error) banner.textContent += ` Discovery: ${snapshot.error}`;
      banner.hidden = state === 'live' && !snapshot.error && !impaired;
    }
  }
  let scrollAt = 0;
  let pendingRender = false;
  // The whole view is rebuilt per frame; a rebuild landing mid-gesture drops
  // frames on a PiP surface, so hold it until scrolling goes quiet.
  window.addEventListener('scroll', () => { scrollAt = performance.now(); }, { passive: true });
  function receive(data, live) {
    if (!data || typeof data !== 'object' || !Array.isArray(data.nodes)) return;
    snapshot = data;
    if (live) { lastReceived = Date.now(); streamError = false; }
    fetchError = false;
    if (performance.now() - scrollAt < 250) { pendingRender = true; return; }
    render();
  }
  rangeButtons.forEach(button => button.addEventListener('click', () => {
    windowMinutes = Number(button.dataset.range);
    rangeButtons.forEach(option => option.setAttribute('aria-pressed', String(option === button)));
    render();
  }));
  fetch('api/snapshot', { headers: { Accept: 'application/json' } })
    .then(response => { if (!response.ok) throw Error('Snapshot unavailable'); return response.json(); })
    .then(data => { if (!snapshot || Date.parse(data.at) >= Date.parse(snapshot.at)) receive(data, false); })
    .catch(() => { fetchError = true; if (!snapshot) main.replaceChildren(append(element('section', 'waiting'), element('h1', '', 'Cannot reach the monitor.'), element('p', '', 'The first snapshot is unavailable. Keep this page open; the live stream will reconnect automatically.'))); updateConnection(); });
  if (typeof EventSource === 'undefined') {
    streamError = true;
    updateConnection();
  } else {
    const source = new EventSource('api/stream');
    source.onmessage = event => {
      try { receive(JSON.parse(event.data), true); }
      catch (_) { streamError = true; updateConnection(); }
    };
    source.onerror = () => { streamError = true; updateConnection(); };
  }
  setInterval(() => {
    updateConnection();
    if (pendingRender && performance.now() - scrollAt > 250) { pendingRender = false; render(); }
  }, 1000);
})();
