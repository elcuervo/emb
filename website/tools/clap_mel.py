#!/usr/bin/env python3
"""The CLAP log-mel, exactly as the model's own feature extractor computes it.

CLAP's audio tower takes a pre-computed mel, so something has to compute it, and
it has to be *the same function* in the build tool and (later) the browser, or a
hash match stops working. This module mirrors
`transformers.models.clap.feature_extraction_clap.ClapFeatureExtractor` for the
model's configured mode (`truncation="rand_trunc"`, `padding="repeatpad"`),
which uses the **slaney** mel filter bank:

  1. repeat-pad the waveform to 10 s (480,000 samples), then zero-pad;
  2. STFT: Hann(1024, periodic), hop 480, centre + reflect pad, power 2.0,
     one-sided (513 bins);
  3. mel: 64 slaney filters over 50–14,000 Hz at 48 kHz;
  4. `10 * log10(max(mel, 1e-10))` — the dB conversion, no top-db clipping.

Output is `[1001, 64]` float32: 1 + 480000 // 480 frames. The tower wants
`[1, 1, 1001, 64]`, which is the same bytes with a leading batch and channel.

`verify()` checks the filter bank against librosa when it is installed, so the
one part that is easy to get subtly wrong is measured rather than assumed.
"""

from __future__ import annotations

import numpy as np

SR = 48000
N_FFT = 1024
HOP = 480
N_MELS = 64
FMIN = 50.0
FMAX = 14000.0
MAX_SAMPLES = 480000  # 10 s
FRAMES = 1 + MAX_SAMPLES // HOP
MEL_FLOOR = 1e-10


def hertz_to_mel(freq: np.ndarray) -> np.ndarray:
    freq = np.asarray(freq, dtype=np.float64)
    min_log_hertz, min_log_mel = 1000.0, 15.0
    logstep = 27.0 / np.log(6.4)
    mels = 3.0 * freq / 200.0
    return np.where(freq >= min_log_hertz, min_log_mel + np.log(np.maximum(freq, 1e-12) / min_log_hertz) * logstep, mels)


def mel_to_hertz(mels: np.ndarray) -> np.ndarray:
    mels = np.asarray(mels, dtype=np.float64)
    min_log_hertz, min_log_mel = 1000.0, 15.0
    logstep = np.log(6.4) / 27.0
    freq = 200.0 * mels / 3.0
    return np.where(mels >= min_log_mel, min_log_hertz * np.exp(logstep * (mels - min_log_mel)), freq)


def mel_filter_bank(nbins: int = N_FFT // 2 + 1, nmels: int = N_MELS, fmin: float = FMIN, fmax: float = FMAX, sr: int = SR) -> np.ndarray:
    """The slaney-normalised triangular bank, shape `[nbins, nmels]`."""
    fft_freqs = np.linspace(0, sr // 2, nbins)
    filter_freqs = mel_to_hertz(np.linspace(hertz_to_mel(fmin), hertz_to_mel(fmax), nmels + 2))
    diff = np.diff(filter_freqs)
    slopes = filter_freqs[None, :] - fft_freqs[:, None]
    down = -slopes[:, :-2] / diff[:-1]
    up = slopes[:, 2:] / diff[1:]
    bank = np.maximum(0.0, np.minimum(down, up))
    enorm = 2.0 / (filter_freqs[2:] - filter_freqs[:-2])
    return bank * enorm[None, :]


def hann_periodic(n: int = N_FFT) -> np.ndarray:
    return np.hanning(n + 1)[:-1]


def log_mel(waveform: np.ndarray) -> np.ndarray:
    """`[1001, 64]` float32 log-mel for a 48 kHz mono waveform."""
    w = np.asarray(waveform, dtype=np.float64).reshape(-1)
    if len(w) == 0:
        raise ValueError("empty waveform")
    if len(w) < MAX_SAMPLES:
        w = np.tile(w, int(MAX_SAMPLES / len(w)))
    w = np.pad(w[:MAX_SAMPLES], (0, MAX_SAMPLES - min(len(w), MAX_SAMPLES)))
    w = np.pad(w, (N_FFT // 2, N_FFT // 2), mode="reflect")
    frames = 1 + (len(w) - N_FFT) // HOP
    starts = (np.arange(frames) * HOP)[:, None] + np.arange(N_FFT)[None, :]
    seg = w[starts] * hann_periodic()[None, :]
    power = np.abs(np.fft.rfft(seg, axis=1)) ** 2.0
    mel = np.maximum(MEL_FLOOR, power @ mel_filter_bank())
    return (10.0 * np.log10(np.maximum(mel, MEL_FLOOR))).astype("<f4")


def packed(waveform: np.ndarray) -> bytes:
    """The bytes a `clap_audio.lua` call expects for `[1, 1, 1001, 64]`."""
    return log_mel(waveform).tobytes()


def verify() -> int:
    """Compare the filter bank to librosa's, when librosa is installed."""
    try:
        import librosa
    except ImportError:
        print("clap_mel: librosa not installed; skipping the cross-check")
        return 0
    theirs = librosa.filters.mel(sr=SR, n_fft=N_FFT, n_mels=N_MELS, fmin=FMIN, fmax=FMAX, htk=False, norm="slaney").T
    ours = mel_filter_bank()
    diff = float(np.max(np.abs(ours - theirs)))
    ok = diff < 1e-8 and ours.shape == theirs.shape
    print(f"clap_mel: filter bank vs librosa max|diff|={diff:.2e} shape={ours.shape} -> {'ok' if ok else 'FAIL'}")
    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(verify())
