/* Music fingerprinting in the browser — the query half of the plate.
 *
 * A Shazam-style fingerprint is a sparse constellation of spectrogram peaks,
 * paired into `(f1, f2, dt)` hashes. The library's hashes ship precomputed
 * (`assets/demo/fingerprints.json`); this module computes the *query*'s hashes
 * live from the shipped excerpt, then matches them by looking for the recording
 * that shares many hashes at one consistent time offset. Nothing is uploaded
 * and nothing is a stored ranking: the votes are counted here, from the audio.
 *
 * Every constant below mirrors `website/tools/build-fingerprints.py`, because
 * a hash computed here must be the same number the library stored. */

export const SR = 11025;
export const N_FFT = 1024;
export const HOP = 256;
const PEAK_DT = 2;
const PEAK_DF = 8;
const PEAKS_PER_SECOND = 40;
const FANOUT = 5;
const PAIR_WINDOW = 48;
const FREQ_BITS = 10;
const TIME_BITS = 10;

/* Iterative radix-2 FFT, in place. Sized once per call — the plate does ~100
 * frames, so the bit-reversal table is rebuilt per frame and that is fine. */
function fft(re, im) {
  const n = re.length;
  for (let i = 1, j = 0; i < n; i++) {
    let bit = n >> 1;
    for (; j & bit; bit >>= 1) { j ^= bit; }
    j ^= bit;
    if (i < j) { const tr = re[i]; re[i] = re[j]; re[j] = tr; const ti = im[i]; im[i] = im[j]; im[j] = ti; }
  }
  for (let len = 2; len <= n; len <<= 1) {
    const ang = -2 * Math.PI / len;
    const wr = Math.cos(ang), wi = Math.sin(ang);
    for (let i = 0; i < n; i += len) {
      let cr = 1, ci = 0;
      for (let k = 0; k < len / 2; k++) {
        const ur = re[i + k], ui = im[i + k];
        const vr = re[i + k + len / 2] * cr - im[i + k + len / 2] * ci;
        const vi = re[i + k + len / 2] * ci + im[i + k + len / 2] * cr;
        re[i + k] = ur + vr; im[i + k] = ui + vi;
        re[i + k + len / 2] = ur - vr; im[i + k + len / 2] = ui - vi;
        const ncr = cr * wr - ci * wi; ci = cr * wi + ci * wr; cr = ncr;
      }
    }
  }
}

function hann(n) {
  const w = new Float64Array(n);
  for (let i = 0; i < n; i++) { w[i] = 0.5 - 0.5 * Math.cos(2 * Math.PI * i / (n - 1)); }
  return w;
}

/* Decode any browser-playable audio to mono float at SR, which is what the
 * library was built at. `decodeAudioData` resamples to the context's rate. */
export async function decodeAudio(arrayBuffer) {
  const Ctx = window.OfflineAudioContext || window.webkitOfflineAudioContext;
  const ctx = new Ctx(1, SR, SR);
  const buffer = await ctx.decodeAudioData(arrayBuffer.slice(0));
  return buffer.getChannelData(0);
}

export function spectrogram(samples) {
  const frames = 1 + Math.floor((samples.length - N_FFT) / HOP);
  if (frames < 1) { throw new Error('that excerpt is too short to fingerprint'); }
  const bins = N_FFT / 2 + 1;
  const win = hann(N_FFT);
  const db = [];
  for (let f = 0; f < bins; f++) { db.push(new Float32Array(frames)); }
  const re = new Float64Array(N_FFT), im = new Float64Array(N_FFT);
  for (let i = 0; i < frames; i++) {
    for (let n = 0; n < N_FFT; n++) { re[n] = samples[i * HOP + n] * win[n]; im[n] = 0; }
    fft(re, im);
    for (let f = 0; f < bins; f++) { db[f][i] = 20 * Math.log10(Math.hypot(re[f], im[f]) + 1e-6); }
  }
  return { db, frames, bins };
}

function percentile(values, p) {
  const sorted = Float64Array.from(values).sort();
  return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))];
}

/* Local maxima in a time-frequency neighbourhood, density-capped — the
 * constellation a fingerprint is made of. */
export function pickPeaks(db, bins, frames) {
  const flat = [];
  for (let f = 1; f < bins - 1; f++) { flat.push(...db[f]); }
  const threshold = percentile(flat, 0.78);
  const found = [];
  for (let t = 0; t < frames; t++) {
    const loT = Math.max(0, t - PEAK_DT), hiT = Math.min(frames, t + PEAK_DT + 1);
    for (let f = 1; f < bins - 1; f++) {
      const v = db[f][t];
      if (v < threshold) { continue; }
      let isMax = true;
      for (let ff = Math.max(0, f - PEAK_DF); ff < Math.min(bins, f + PEAK_DF + 1) && isMax; ff++) {
        for (let tt = loT; tt < hiT; tt++) { if (db[ff][tt] > v) { isMax = false; break; } }
      }
      if (isMax) { found.push({ t: t, f: f, v: v }); }
    }
  }
  const cap = Math.floor(PEAKS_PER_SECOND * frames * HOP / SR) + 1;
  found.sort((a, b) => b.v - a.v);
  return found.slice(0, cap);
}

export function hashOf(f1, f2, dt) {
  return ((f1 & ((1 << FREQ_BITS) - 1)) << (FREQ_BITS + TIME_BITS)) |
    ((f2 & ((1 << FREQ_BITS) - 1)) << TIME_BITS) |
    (dt & ((1 << TIME_BITS) - 1));
}

/* `hash -> anchor frame`, the same map the builder produced for the library. */
export function fingerprint(peaks) {
  const byFrame = new Map();
  peaks.forEach((p) => {
    if (!byFrame.has(p.t)) { byFrame.set(p.t, []); }
    byFrame.get(p.t).push(p.f);
  });
  const frames = [...byFrame.keys()].sort((a, b) => a - b);
  const out = new Map();
  frames.forEach((t) => {
    byFrame.get(t).forEach((f1) => {
      let taken = 0;
      for (let i = 0; i < frames.length && taken < FANOUT; i++) {
        const tt = frames[i];
        if (tt <= t || tt > t + PAIR_WINDOW) { continue; }
        byFrame.get(tt).forEach((f2) => {
          if (taken < FANOUT) { const h = hashOf(f1, f2, tt - t); if (!out.has(h)) { out.set(h, t); } taken++; }
        });
      }
    });
  });
  return out;
}

/* Hash lookup, then the offset histogram: the recording that shares hashes at
 * one consistent offset wins, and the size of that vote is the evidence. */
export function match(query, hashes) {
  const votes = new Map();
  query.forEach((qt, h) => {
    const entries = hashes[h];
    if (!entries) { return; }
    entries.forEach(([track, lt]) => {
      const key = track + ':' + (lt - qt);
      votes.set(key, (votes.get(key) || 0) + 1);
    });
  });
  const ranked = [...votes.entries()].map(([key, n]) => {
    const [track, offset] = key.split(':').map(Number);
    return { track: track, offset: offset, votes: n };
  }).sort((a, b) => b.votes - a.votes);
  return { best: ranked[0] || null, ranked: ranked.slice(0, 8) };
}
