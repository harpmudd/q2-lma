# Device testing plan

## Round 0: checks with no new code (about 10 minutes)

Write down the answers to each step.

1. **Q2 Pod version** (System settings → About). It must be **1.5.9 or later** (released
   2026-10-30), the first release that writes `/tmp/q2-addon.caps`, and so the first with `.m3u`
   Stream mode (needed for Ogg), progress rows and full lists. If it's older, update first: System
   settings → System Update with Wi-Fi.
2. **Card apps already on the card.** Count the rows in Streaming besides TIDAL, Spotify, Tidal
   Downloads, Internet Radio, AirPlay and Configuration. Streaming shows at most 8 card apps.
3. **Card free space and format** (on a PC: properties of the card; FAT32 or exFAT).
4. **Network baseline.** Streaming → Internet Radio → play any station. This proves Wi-Fi and the
   audio path work outside any card app.
5. **A Go card app runs on your unit.** System settings → App Store → Podcasts → Install, then
   Streaming → Podcasts, search any show, download one episode and play it. This proves the
   App Store, the Go build style (static mipsle soft-float, nan2008), HTTPS from a card app and
   playback of a downloaded file. Optional; the probe in round 1 also proves it.

## Round 1: the probe (one sideload)

A throwaway card app, `.lma/`, that answers what the design depends on. It shows each result as a
row on the Q2, and writes them to `.lma/probe.txt` on the card:

- the binary runs (toolchain and nan2008 on this unit), and the launcher's pid, crash and log files;
- the contents of `/tmp/q2-addon.caps`;
- the system clock at start (unset or not), and the time from an HTTPS Date header;
- DNS and HTTPS to archive.org with the embedded CA bundle over tcp4, plus the time to answer an
  advancedsearch query and a metadata query;
- download throughput for one etree MP3 and one Ogg (sets first-song latency and the cache window);
- free card space, free memory (`/proc/meminfo`), CPU (`/proc/cpuinfo`), kernel;
- **playback:** a "Play test" row that streams 3 short tracks through `.m3u` Stream mode (MP3,
  Ogg, MP3), so you can confirm both formats play in order and the next track follows on.

Steps:

1. Power the Q2 **fully off** (not asleep). Take out the card.
2. Unzip `q2-lma-probe-*.zip` at the card root, so `X:\.lma\` holds `q2-lma`, `run`, `q2app` and the
   icons.
3. Eject the card in Windows, put it back, power on, join Wi-Fi.
4. Streaming → **LMA Probe**. Wait for the rows (up to about 1 minute while it measures).
5. Press **Play test**. Note whether all 3 tracks play, in order, and how long the first one took to
   start.
6. Note anything odd on screen ("isn't running", "didn't answer", toasts).
7. Power **fully off**, take out the card, and send me `X:\.lma\probe.txt` and `X:\.lma\log.txt`.

## Round 2 onward

Once the app exists: same sideload steps, one feature per round, with `log.txt` coming back each
time.
