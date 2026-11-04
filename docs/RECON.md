# Recon: Q2 Pod card apps (2026-10-31)

Sources read: DiamondBond/q2-pod at `1ddd96d` (patch/addon.c, patch/addon_library.c,
patch/netupdate.c, docs/internals.md, docs/librespot.md, docs/downloads.md, sources.list),
andsuor/q2-subsonic `q2-v0.4`, m-d-e-v/q2-podcasts v1.6, DiamondBond/q2-librespot `shanlingq2`,
q2-pod PRs #145-#150 and #163, and the maintainer's review m-d-e-v/q2-qobuz#1.

**[C]** = confirmed from source or docs. **[I]** = inferred.

## Three findings that shape the design

1. **Stream mode does not stream a URL.** [C] The app downloads each song whole into a card folder
   (renamed in from `.part`); the firmware plays it as a local file once it exists. A card cache is
   mandatory, and the first song starts only after its whole file has downloaded, so small MP3/Ogg
   derivatives are the right v0 choice.
2. **Build:** static `CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat`, **then set the
   `EF_MIPS_NAN2008` ELF flag** (the kernel requires it; Go doesn't set it). Both Go apps do this in
   an identical `contrib/shanlingq2/package.py`. [C]
3. **Firmware buffer limits:** op names < 16 bytes, a level's escaped args <= 400 bytes, at most 6
   levels deep. See Limits. [C]

## 1. On-card layout

```
/mnt/mmc/.<id>/
  q2app        manifest (required)
  run          launcher, run as: /bin/sh /mnt/mmc/.<id>/run
  q2-<id>      binary (required: without it the Streaming row is hidden)
  icon-40.png  optional, 40 px, drawn 1:1
  icon.png     optional, 52 px, scaled; else a shared icon
```

- At most 8 card apps show in Streaming (`APPS_MAX`, peq.h:240), sorted by name. [C]
- `q2app` is `key=value` lines: `name=` (row label, <= 31 bytes, default the id), `search=` (search
  box hint), `qr=1` (show the QR page at first answer), `goto=1` (app answers the `goto` level),
  `scrobble=0` (1.6.5: songs under `.<id>/` stay out of `.scrobbler.log` and still count as plays;
  read at listen time, older firmware ignores it). [C]
- `run` (Subsonic/Podcasts pattern): copy binary and itself to `/tmp`, `exec` from there (the card is
  never kept busy), background a restart loop (5 s doubling to 60 s on non-zero exit; exit 0 ends
  it), pid in `/tmp/q2-<id>`, second run exits while it's there, TERM passes to the app and KILLs
  it after 2 s (the firmware waits 3 s). Subsonic keeps a failed run's stderr in
  `/tmp/q2-<id>.crash` and folds it into `.<id>/log.txt` at the next start. [C]

Runtime files [C]:

| File | Writer | Purpose |
|---|---|---|
| `/tmp/q2-<id>.browse.sock` | app listens | Q2SPOT/1 |
| `/tmp/q2-<id>.state` | app | `hold=1` holds standby/auto power-off; `addr`, `pin`, `qr`, `qrsize`, `note` for the QR page |
| `/tmp/q2-<id>.play` | app | Stream mode session |
| `/tmp/q2-<id>.playing` | firmware | path now playing |
| `/tmp/q2-<id>.library` / `.library.done` | app / firmware | library update |
| `/tmp/q2-<id>.sleep` | app | sleep timer |
| `/tmp/q2-addon.caps` | firmware | feature words |

The firmware stops an app with TERM to the loop's pid: Stop Streaming Services, before USB storage
(restarted after), App Store install/remove, the Stop row. [C]

## 2. Q2SPOT/1

**Launch** [C]: the Streaming row runs `run` (unless running) and opens the page with first op `menu`,
no args. No socket: "Starting <name>…", asked again every 500 ms; after 10 s "<name> isn't
running" + Retry. Stop <name> appears on error pages and on loads of 3 s or more.

**Framing** [C]: one request per Unix stream connection; client writes, shuts its write side, reads
to EOF; 15 s deadline (Subsonic answers within 8 s). Request <= 8 KiB, reply <= 64 KiB. Lines end
`\n`, fields tab-separated; `%`, tab, LF, CR escaped as `%25 %09 %0A %0D`; NUL or a bad escape is
malformed.

```
Q2SPOT/1
<id>\t<op>\t<arg>…

Q2SPOT/1
<id>\t<status>\t<message>\t<cursor>
<kind>\t<uri>\t<title>\t<subtitle>\t<context>\t<position>     (0-50 rows)
```

Message <= 120 bytes, title/subtitle <= 255 (cut on a UTF-8 boundary), cursor <= 1 KiB. A reply not
echoing the id is dropped. A non-`ok` status shows only the message + Retry, never rows (so Subsonic
reports errors as `ok` with a message row). Reusable implementation: `internal/q2spot` in either app.

**Row kinds** [C] (addon.c `go()`):

| Kind | Press |
|---|---|
| `list` | new level: op = `uri`, arg = `context`, title = row title |
| `action` | send op `uri` + `context`, toast the reply message, reload level; `ok` to op `stop` closes the page |
| `search` | T9 box; OK sends op `search` + term (<= 63 bytes) |
| `remote` | QR page from the state file |
| `local` | play a card file; `uri` ending `/` opens a folder; `context=album` = track (name) order |
| `info` | nothing |
| `progress` | nothing; `position` = permille bar. Only when caps has `progress`, else send `info` (old firmware sends `position` as an op) |

- `position` on any other row is the op a **held Play/Pause** sends, with `context` as arg.
- Reply message `refresh`: re-asked every 2 s while on top, never pages. `pages` + cursor: firmware
  fetches the next pages itself and appends. Any other message + cursor: a More… row.
- Presses reach the app only as requests. No key channel: transport keys stay with the stock player.

**Stream mode, folder form** [C]: `.play` = `"<seq> /mnt/mmc/.<hidden-folder>"` (written whole, by
rename). Songs named in play order (`0001 - Title [id].mp3`), each renamed in. First song plays once
present; later names sorting after the last queued are appended. Firmware writes `.playing`; app
fetches ahead and deletes long-played files. Any other queue ends the session. Folder re-read only
on mtime change; FAT mtimes are 2 s, so leave >= 2.1 s between rewrites.

**Stream mode, `.m3u` form** [C] (caps `playlist`): `.play` = `"<seq> /mnt/mmc/…/queue-<seq>.m3u"`,
UTF-8, one absolute card path per line (`#` lines, BOM, CRLF tolerated). Queued in order; the
firmware waits at the first missing entry. The app may append, or rewrite from the waited entry on;
earlier changes need a new seq. Lines off `/mnt/mmc/`, with a `..` segment, or >= 512 bytes are
skipped; a `.play` path >= 160 bytes is refused. Reads <= 256 KB, 32 folder listings a poll. A user
queue at any time ends the session. Reference: q2-subsonic `cmd/q2-subsonic/stream.go` (2 ahead,
2 behind, skip after 3 failed tries, idle exit).

**Library update** [C]: not needed for v0 (streamed songs live in hidden folders the scan skips).

**Caps** [C]: `/tmp/q2-addon.caps`, one line of space-separated words, currently
`playlist progress pages library goto sleep`, rewritten at every Streaming scan. Read as a set
(`strings.Fields`); no file = older firmware, use none. Reading at startup works since the scan
precedes any launch [I]; re-reading per use (Podcasts) also survives firmware updates. These words
arrived in PRs #146-#150 (merged 2026-10-30, Q2 Pod 1.5.9+ [I]). Before #147, folder mode played
only FLAC/MP3, so **Ogg needs `playlist`**; MP3 is the safe fallback.

## 3. sources.list

[C] netupdate.c `shelf_sources`: `owner/repo [Display Name]`, `#` comments, at most 16 lines read.
**id = repo name less `q2-`**, 1-15 of `a-z0-9-`, unique, not `rockbox`/`themes`/`spotify`, not
reserved (`books coverflow downpic photos q2 sldp tidal`). `q2-lma` → id `lma`, folder `.lma`,
binary `.lma/q2-lma`. Install takes the latest release's first `.zip` asset, needs zip size + 64 MB
free, unpacks only `.<id>/*`, keeps nothing on update (never ship settings in the zip). Remove is
`rm -rf .<id>`. Listing needs a PR to q2-pod at release time.

## 4. Toolchain and releases

[C]
- `CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -trimpath -ldflags='-s -w -buildid='`
- `package.py`: verify static ELF, MIPS LE, soft-float ABI, no `PT_INTERP`; set nan2008; zip
  `.<id>/{q2app,icon.png,icon-40.png,run,q2-<id>}`, binary last, entries dated 2026-01-01.
- Zip `q2-<id>-vX.Y-<sha>.zip`. Tag `q2-vX.Y` must match `const version` in main.go; CI runs vet,
  tests, `test_launcher.py`, build, `gh release create`.
- Pin a supported Go (maintainer's #1 must-fix on qobuz was Go 1.22; Subsonic pins `1.27`).
- CPU: Ingenic X2000, MIPS32r2 FP64 NaN-2008, glibc 2.28. librespot (contrast) is Rust hard-float
  linked to the Q2's glibc; soft-float doesn't matter for us since the stock player decodes [I].

## 5. HTTPS, CA roots, clock, memory

- No CA bundle for apps [C]; `/etc/scrobble-ca.pem` is the firmware's own pinned set. Both Go apps
  embed curl's Mozilla `cacert.pem` (MPL-2.0) via `go:embed` (`internal/tlsroots`).
- Clock often unset (1970/2000) [C]. Podcasts takes time from plain-HTTP `Date` (maintainer flagged
  as forgeable, qobuz #6); Subsonic from an HTTPS HEAD verified except dates. Use Subsonic's, with a
  floor at a date built into the binary.
- No IPv6 in the kernel [C]: dial `tcp4` (Podcasts).
- ~67 MB free beside the firmware, no swap [C]: `debug.SetMemoryLimit(48 << 20)`.

## 6. Limits (addon.c) [C]

| Limit | Value | Effect |
|---|---|---|
| Rows per reply | 50, <= 64 KiB | page longer lists |
| `pages` auto-paging | 10 replies / 500 rows, then More… | a year of shows fits; all shows of an artist may not |
| Depth (`ADDON_DEPTH`) | 6, oldest past base dropped | menu → Artist → Year → Show → Songs = 5; More… also uses a level |
| Op name (`lv_t.op`, `pg.aop`) | < 16 bytes, silently cut | short op names |
| Level args (`lv_t.args`) | 400 bytes escaped, with tabs | context must fit or the press does nothing |
| Level title | 127 bytes | |
| Search term | 63 bytes | |
| Reply deadline | 15 s | cache; answer within ~8 s |
| `refresh` lists | never page | small live screens only |

## Maintainer expectations (from reviews)

- Supported, pinned Go. `hold=1` only while actually fetching, never while a paused session lives.
- Validate ids/folder names in one place before they reach file paths.
- No certificate check at a time below a built-in date. Logs in `/tmp` or a size-capped card file,
  no secrets. Don't count transport errors as permanent failures; back off.
- Don't evict what the user is playing; stop with a clear "space full" instead.
- Zip holds only `.<id>/`, no settings.

## Host-side harness

Feasible [C]: Subsonic's `q2spot_test.go` dials the socket; `device_test.go` drives the handler with
fakes; `test_launcher.py` runs `run` on the host. Plan: a Go Q2SPOT/1 test client enforcing the
limits above; a fake firmware driving `.play` / `.m3u` / `.playing` against a temp card; an
`httptest` archive.org with recorded JSON. Unix sockets work on Windows 10+ in Go [I, verify];
`test_launcher.py` needs `sh` (Git Bash).

## Device testing

1. Power the Q2 fully off (not asleep).
2. Unzip the release zip at the card root: `X:\.lma\{q2-lma,run,q2app,icons}`.
3. Eject, reinsert, power on, join Wi-Fi, open Streaming, press the row.

Failure signs [C]: long "Starting…" then "isn't running" (run failed / crashed at start); "didn't
answer" (no or malformed reply within 15 s); Retry and Stop rows on both. `/tmp` is lost at
power-off and no shell is documented [I], so the app writes `.lma/log.txt` (per-line open/close,
1 MB rotation, crash trace folded in at next start). Read it on a PC after a full power-off.

## Not yet done

Internet Archive endpoints (advancedsearch.php, /metadata, /download) are unverified against the
live API: first step of design.
