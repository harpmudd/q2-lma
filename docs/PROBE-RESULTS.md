# Probe results (2026-10-31)

`q2-probe-0.1` on the user's Q2, Q2 Pod 1.5.9, home Wi-Fi. Two runs; files read from the card's
`.lma/probe.txt` and `.lma/log.txt`.

## Device

| Check | Result |
|---|---|
| Binary | runs: static mipsle soft-float with nan2008 set is right |
| Kernel | Linux 5.10.21+, SMP PREEMPT |
| CPU | Ingenic XBurst II, 2 cores |
| Memory | 117.9 MB total, ~78.6 MB available with the firmware running |
| App memory | RSS 10.8 MB, Go heap 2.5 MB after all checks |
| Card | 512 GB, 415 GB free; sequential write + fsync 17-18 MB/s |
| Caps | `playlist progress pages library` (no `goto` or `sleep` in 1.5.9) |
| Clock | already set (within 1 s of archive.org); Wi-Fi time sync works on this unit |

## Network to archive.org

| Check | Result |
|---|---|
| DNS | 5-11 ms, one IPv4 address |
| HTTPS | verifies with the embedded roots, tcp4 |
| advancedsearch (50 rows) | 0.3-0.5 s |
| `/metadata/<id>/files` (30 KB) | 0.1-0.2 s |
| Download | 1.1-1.8 MB/s, from `dn720303.ca.archive.org` or `ia600106.us.archive.org` |

## Stream mode

- `.m3u` mode works: MP3 → Ogg → MP3 played in order; Next moved through all three.
- **Ogg plays.**
- Time to first sound (run 1): press → file landed 2.1 s → playing about 3 s. The firmware starts
  a song within about 1 s of its file landing (its 500 ms poll).
- Throughput sets first-song latency: about 1.5 MB/s means ~0.7 s per MB.

## Design consequences

1. **Prefer Ogg over VBR MP3 when both exist.** Ogg derivatives are about half the size (this show:
   Mama Tried 4.2 MB MP3 vs 2.0 MB Ogg; a 26-minute jam 38 MB vs 18 MB). For a long opening track
   that is ~25 s vs ~12 s before sound. MP3 when there is no Ogg.
2. **Make stream file names unique per session.** Probe bug found in run 2: the new session's first
   file had the same path as the last session's, and `.playing` still named it, so its start went
   unseen (the report says "playing never" and "first started at 0.0 s"; it did play). The app should
   put each session in its own folder or name, and treat `.playing` as stale until it changes.
3. Memory is generous for this app (~11 MB RSS against ~78 MB free); keep the 48 MB soft limit.
4. Search and metadata are fast enough to fetch live and cache, not to prefetch whole catalogs.
5. The clock fallback is still needed for units without Wi-Fi time, but this unit doesn't need it.
