# RTTY Transmit Spectrum Experiment

Date: 2 November 2026.

A numerical experiment compared abrupt tone changes with 5 ms and 10 ms
raised-cosine frequency transitions for the proposed 170-Hz RTTY transmitter.
The 5 ms candidate substantially reduced out-of-band energy on the measured
patterns and supports requiring transition shaping in version 0.1.

These are measurements of a temporary waveform model, not an implemented
go-rtty transmitter, RF measurements, or independent-receiver test results.
The full [spectral acceptance gate](RTTY_DESIGN.md#spectral-acceptance) remains
required before release.

## Waveform and measurement method

The experiment used Go 1.26.2 on Linux amd64 and Gonum v0.17.0 real FFTs.
It generated mono PCM16 at 12 kHz and 48 kHz, with mark 1585 Hz, space
1415 Hz, exact 500/11 baud, and one start, five least-significant-bit-first
data bits, and 1.5 stop bits. Oscillator phase started at zero and remained
continuous. There was no receiver or channel impairment.

Each frequency transition started at the logical symbol boundary. For a
transition duration `tau`, the model used:

```text
u = clamp((t - boundary) / tau, 0, 1)
f(t) = f_old + (f_new - f_old) * (1 - cos(pi*u)) / 2
x[n] = 0.5 * envelope[n] * sin(phase[n])
phase[n+1] = wrap(phase[n] + 2*pi*f(n/Fs)/Fs)
pcm[n] = roundToEven(32767*x[n])
```

The abrupt reference set `f(t)` directly to the next tone at the boundary.
Half-bit boundaries used `floor(H*Fs*11/1000)` without per-symbol rounding.
Every waveform had a 250 ms mark lead-in, a 150 ms mark tail, and 5 ms
raised-cosine amplitude ramps inside those intervals. Analysis converted PCM
back to floating point by dividing by 32768.

Long patterns contained 384 frames, or 63.36 seconds of payload: the cyclic
sequence of codes 0 through 31, and separate repetitions of codes 0, 31, 10,
and 21. These are explicit raw framing patterns; no text encoder or automatic
shift insertion was used. Code 21 supplies frequent mark/space transitions.

Steady-payload spectra excluded lead-in and tail and used a one-second
periodic Hann window, 50 percent overlap, 1 Hz FFT bins, and linear averaging
of one-sided power spectra. Hann equivalent noise bandwidth was 1.5 Hz.
Ratios used total positive-frequency power from the same spectrum as the
denominator. No peak normalization or display smoothing was used.

Finite-burst measurements included the complete lead-in, one raw character,
and tail, including both amplitude ramps. They used a rectangular analysis
window and zero-padding to the next power-of-two FFT length, giving bin
spacing of 1.46484375 Hz at both sample rates. Out-of-band energy was
integrated over bin centers. These preliminary burst measurements use coarser
resolution than the release gate, which requires bin spacing at most 1 Hz.

## Selected results

All values are dBc relative to total power or, for a finite burst, total
energy. The two outside-band columns sum both sides of the center frequency.

| Sample rate | Pattern | Transition | Outside +/-250 Hz | Outside +/-500 Hz |
| --- | --- | --- | ---: | ---: |
| 12 kHz | Cyclic codes 0 through 31 | Abrupt | -30.39 | -39.94 |
| 12 kHz | Cyclic codes 0 through 31 | 5 ms | -43.07 | -70.78 |
| 12 kHz | Repeated code 21 | Abrupt | -28.56 | -38.21 |
| 12 kHz | Repeated code 21 | 5 ms | -41.16 | -69.10 |
| 48 kHz | Repeated code 21 | Abrupt | -28.59 | -38.34 |
| 48 kHz | Repeated code 21 | 5 ms | -41.16 | -69.13 |
| 12 kHz | Single code 3 burst | Abrupt | -35.44 | -45.23 |
| 12 kHz | Single code 3 burst | 5 ms | -44.18 | -69.19 |

Across the measured long patterns at both sample rates, the 5 ms candidate's
worst outside-250-Hz result was -41.16 dBc and its worst outside-500-Hz result
was -69.08 dBc. The largest ten-bin, 10 Hz band wholly outside +/-250 Hz was
-47.43 dBc, on repeated code 10. These satisfy the proposed -40 dBc,
-55 dBc, and -45 dBc limits for the tested subset.

The 10 ms candidate further reduced steady-payload energy, but affects a
larger fraction of each bit. The 5 ms candidate is preferred for the first
interoperability experiment because it met the provisional mask on this
subset with less transition time. This does not establish its receiver
performance or justify omitting the remaining waveform cases.

## Remaining qualification

The narrowest measured margin to an integrated limit is about 1.2 dB.
Qualification must cover all repeated codes, seeded traffic, both polarities,
both stop lengths and baud presets, center-frequency and initial-phase
variation, amplitude variation, and queue interruptions. Complete generated
transmissions must also pass the whole-burst energy checks and independent
receiver tests. If another case fails, change the waveform and rerun the gate;
do not average away the failure or automatically loosen the limit.

The receiver work must account for the shaped transition's midpoint occurring
about 2.5 ms after the logical boundary. Neither the library's character
timing nor an RF occupied-bandwidth claim has been validated by this experiment.
