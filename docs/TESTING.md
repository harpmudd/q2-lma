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

## Round 2: v0.1 (2026-10-31)

Install: Q2 fully off, copy `dist/card/.lma/` over the card's `.lma/` (or unzip
`dist/q2-lma-v0.1-*.zip` at the root and copy `dist/card/.lma/my-artists.txt` beside it). The
probe's leftovers (`.lma/stream/`, `.lma/probe.txt`) are unused; delete them or not.

The three things only the Q2 can answer come first.

1. **Album art in Ogg files.** Streaming → Live Music Archive → My Artists → Grateful Dead → 1977
   → 05-08 → Play Show. On Now Playing: is there art (the llama; this show has none of its own),
   and do the title, artist and album show?
2. **Gaps between songs.** Same show, press song 16, Saint Stephen: it runs into 17, Not Fade
   Away, then into 18, Saint Stephen again. Is there a gap or click at each change? Then Settings →
   Prefer: MP3, press song 16 again (it downloads the MP3s), and compare the gaps and the art.
3. **Start time.** How long from pressing Play Show to sound, the first time, and again later
   (from the cache, it should be at once).

Then, as time allows:

4. Next, Previous, pause for a few minutes, screen off: does playback carry on through the show?
5. More Versions on 05-08: pick another recording, Back, Play Show: does it play that one?
6. Shuffle a Show from the menu, and from an artist's page.
7. Hold Play/Pause on an artist in All Artists or Top Artists: added to My Artists? Again:
   removed?
8. Search Artists for a band, and browse a letter in All Artists.
9. Watch for "didn't answer" or "isn't running" anywhere (seen intermittently on the PC test
   harness only, believed to be Windows-specific).

Afterwards: power fully off and send `.lma/log.txt`.
