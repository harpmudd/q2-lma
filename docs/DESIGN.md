# q2-lma v0 design

A Q2 Pod card app (id `lma`, Streaming row "Live Music Archive") that browses the Internet
Archive's Live Music Archive (`etree`) and streams shows through Stream mode. Inputs:
[RECON.md](RECON.md), [PROBE-RESULTS.md](PROBE-RESULTS.md), [ARTISTS.md](ARTISTS.md), [ART.md](ART.md).

## Decisions (user, 2026-10-31)

- Artists are read live from the archive. **My Artists is each user's own:** it ships empty, and
  nothing in the app is specific to one person's taste.
- Played shows **stay on the card up to a cap** (default 2 GB, adjustable), oldest evicted first.
- No "Download show" in v0.
- **Shuffle** picks from My Artists (Top Artists while My Artists is empty).
- Album art: the show's own art if it has some, else the Live Music Archive llama logo.

## Following the existing apps

Q2 Pod's maintainer (DiamondBond) reviews every app added to the App Store, and has no written
style guide. These conventions come from the firmware's own Spotify browser, the two Go card apps
he accepted (andsuor/q2-subsonic, m-d-e-v/q2-podcasts), and his reviews (q2-pod #145-#150, #163,
m-d-e-v/q2-qobuz#1). Where they agree, q2-lma does the same.

**On screen**
- Rows are Title Case: "Search Artists", "More Versions", "Turn Off Live Music Archive".
- Subtitles carry counts and state, parts joined with " · ": "SBD · 4.8★ · 23 versions".
- Empty lists say so plainly in one row: "No shows yet", "Nothing found". Loading shows
  "Loading…" with the ellipsis character.
- A screen that can't load at all answers a non-ok status with one line of English (`offline`,
  `throttled`, `error`), so the firmware shows it with **Retry**, as its Spotify browser does. A
  problem that leaves the rest usable is an info row plus a Details row, last in the list, as
  Subsonic does.
- Rows that come and go (stream errors) go last, so the rows above keep their places under the
  cursor.
- The main menu asks to `refresh` only while something changes (a download, a background load),
  as Podcasts does.
- Settings rows show their value and step it on a press, toasting the new one ("Cache Size:
  4 GB"); the last row is "Version 0.1", as Subsonic's.
- The last menu row is **Turn Off Live Music Archive** (`stop`), with when it turns off by itself
  as its subtitle, as Podcasts' "Turn Off Podcasts". The app exits after 15 minutes unused.
- Only features the firmware lists in `/tmp/q2-addon.caps` are used: `progress` rows only with
  `progress`, otherwise `info` text.
- `q2app`: `name=Live Music Archive`, `search=Search artists`, `qr=0`, `scrobble=0` (Q2 Pod 1.6.5+ keeps shows out of the scrobble log; older firmware ignores it). Icons `icon.png` (52 px) and
  `icon-40.png` (40 px), made from the llama logo; white on black, so the Minimal theme's greying
  suits it.

**Repository** (same layout as Subsonic and Podcasts)
- `cmd/q2-lma`, `internal/…`, `contrib/shanlingq2/` with `README`, `build.sh`, `package.py`,
  `launcher.sh`, `q2app`, the icons and `test_launcher.py`.
- `const version = "q2-vX.Y"` in main.go; tag `q2-vX.Y`; release zip `q2-lma-vX.Y-<sha>.zip`, the
  release's only `.zip` asset, holding only `.lma/` and no settings.
- `.github/workflows/ci.yml` (vet, gofmt, tests, `test_launcher.py`, the Q2 build) and
  `release.yml` (on a tag: the same checks, then publish), with Go pinned to a supported release.
- `CHANGELOG.md` with an Unreleased section; README sections Install, Use, Files, Limits, Errors
  and logs, Update and remove, Build, License.
- Code comments explain why and name the firmware code they rely on (`addon.c`'s limits, Stream
  mode rules), as both apps' do.

**Behaviour the maintainer asked for in reviews**
- `hold=1` only while a download runs; no standby hold for a paused session.
- Ids and file names from the network are checked in one place before they reach a path.
- Certificates are never checked at a time before a date built into the binary.
- Network errors don't use up a song's tries; they wait and back off.
- Nothing the user is playing is evicted from the cache.
- Logs carry no secrets (this app has none) and never keep the card busy.
- Removing the app leaves nothing behind outside `.lma/`.

## Screens

Each screen is one Q2SPOT/1 level. Op names stay under 16 bytes; contexts are identifiers, well
under the 400-byte limit.

```
Live Music Archive                       op menu
├─ My Artists                            op mine        the user's own; hold Play/Pause: remove
├─ All Artists                           op letters     A-Z, 0-9
│   └─ <letter>                          op letter X    paged, "pages" (500 rows, then More…)
├─ Top Artists                           op top         500 most downloaded collections
├─ Search Artists                        search row     op search TERM
├─ Shuffle a Show                        action shuffle
├─ Settings                              op settings    Cache Size: 2 GB, Clear Cache, Version 0.1
├─ Turn Off Live Music Archive           action stop    "It also turns off after 15 minutes unused"
└─ (while streaming) Couldn't stream "Scarlet Begonias"   info, then a Details row; last, so the
                                                          rows above keep their places

<artist>                                 op artist ID   hold on any artist row: add/remove My Artists
├─ Shuffle <artist>                      action shuffle ID
└─ 1977 · 64 shows                       op year ID 1977       newest year first
    └─ 05-08 Barton Hall, Cornell U.     op show ID 1977-05-08
         subtitle "SBD · 4.8★ · 23 versions"
         ├─ Play Show                    action play IDENT 0
         ├─ More Versions (22)           op versions ID 1977-05-08
         │    └─ <recording>             action pick DATEKEY IDENT  (toast, Back shows its songs)
         └─ 01 Minglewood Blues  6:21    action play IDENT 0
```

- Depth: menu → artists → artist → year → show → versions is 6, the firmware's limit. Through
  All Artists (one more level, the letter) the firmware drops the oldest level past the menu, so
  Back from deep in skips the letter list. Accepted.
- Picking a version is an action, not a new level: it stores the choice for that date, and Back
  shows that recording's songs.
- Song rows are `action play IDENT N`: the queue is the whole recording from song N.
- Long lists (a letter's artists, Top Artists) send `pages`; refreshing screens send `refresh`.
  The firmware never pages a `refresh` list, so a screen is one or the other.

## Data from archive.org

All requests carry `User-Agent: q2-lma/<version> (+<repo URL>)`, dial IPv4, one metadata request
at a time, honour `Retry-After` and back off on 429/5xx.

| Need | Request | Cached for |
|---|---|---|
| Letter / Top / Search | `advancedsearch.php` `mediatype:collection AND collection:etree …`, `fl`: identifier, title, downloads | 1 day |
| An artist's years | `services/search/v1/scrape?q=collection:ID&fields=year` (cursor) | 7 days |
| A year's shows | `advancedsearch.php` `collection:ID AND year:Y`, `fl`: identifier, date, source, avg_rating, num_reviews, downloads, venue, coverage, title; rows 1000 | 1 day for the current year, else 30 days |
| A recording's files | `/metadata/IDENT/files` (30 KB; the full `/metadata/IDENT` was 2 MB) | 30 days |
| A song | `/download/IDENT/FILE` (redirects to a storage node; Range works) | the audio cache |

- **Slow lists answer at once.** The Grateful Dead's years need about 9 s (18,411 items, two
  scrape calls). The Q2 gives up at 15 s, so a level whose data isn't cached answers a `refresh`
  page with a progress row ("Loading years…") and fills it in the background; its next refresh shows
  the list.
- Metadata cache: `.lma/meta/`, one JSON file per request, written by rename, total capped
  (20 MB), oldest dropped first.
- Collections are matched by identifier, never by title.

## Choosing the best recording for a date

Recordings of one date are ranked by, in order:

1. **Source class**, from the identifier and `source` text (case-insensitive tokens):
   soundboard (`sbd`, `soundboard`, `board`, `pre-fm`, `fm`) > matrix (`mtx`, `matrix`) >
   unknown > audience (`aud`, `audience`). An identifier saying `sbd` wins over a `source` saying
   matrix (the famous Cornell '77, `gd77-05-08.sbd.hicks…`, has source "Matrix (see notes)").
2. **Rating, damped for few reviews:** `(avg × n + 4.0 × 5) / (n + 5)`. Five reviews at 5.0 rank
   below 300 reviews at 4.8. No rating counts as 4.0 with n = 0.
3. **Downloads.**
4. A recording without VBR MP3 or Ogg files is skipped at play time, and the next one plays.

On the sample date this picks `gd77-05-08.sbd.hicks.4982.sbeok.shnf` (soundboard, 4.78 from
300 reviews). The show row's subtitle names the class, rating and version count.

## Streaming

As q2-subsonic does, through Stream mode's `.m3u` form (the user's firmware lists `playlist`):

- The cache holds one folder per recording: `.lma/cache/<IDENT>/NN - Title.ogg`. Pressing
  play writes `.lma/queue/queue-<seq>.m3u` listing the recording's songs from N on, then
  `/tmp/q2-lma.play` = `<seq> <m3u path>`.
- **Format:** Ogg Vorbis when the recording has it (about half the size of VBR MP3, so it starts
  sooner), else VBR MP3, else 64 kbps MP3. Subject to the Ogg art test below.
- **Fetch-ahead:** the song playing and the next 2, one download at a time, each to `.part`, synced,
  then renamed. `hold=1` only while a download runs, never while paused (the maintainer's qobuz
  review).
- **Stale `.playing`:** cached paths repeat across sessions, so `.playing` from an earlier session
  can name a song of the new one. It counts only once it changes; until then the current song is the
  one pressed.
- **Failures:** 3 tries per song (2, 8, 30 s); network errors keep waiting rather than failing. A
  song that can't be had is dropped from the queue file from the waited entry on, as the firmware
  allows.
- A session ends when the firmware plays anything else, or after the last song is fetched.

**Tags and art**, written before the rename:

- MP3: an ID3v2.4 tag with title, artist, album (`1977-05-08 Barton Hall`), track number and the
  art (APIC, baseline JPEG <= 600 px).
- Ogg: Vorbis comments with the same fields and `METADATA_BLOCK_PICTURE`. Built only if the device
  test below shows the Q2 displays Ogg art.
- Art: the recording's own JPEG/PNG files (not `__ia_thumb.jpg`, archive.org's generated tile),
  scaled; else the llama logo, embedded in the binary as a JPEG.

## Cache and card space

- Cap: Settings → Cache Size steps 512 MB, 1 GB, 2 GB (default), 4 GB, 8 GB, saved in
  `.lma/settings.json`.
- After each download, whole recordings are evicted least recently played first until under the
  cap, never the session's own.
- Below 1 GB free on the card, nothing downloads and the screen says "Card full".
- Settings → Clear Cache asks twice, then removes `.lma/cache/` (not the session playing).

## My Artists and Shuffle

My Artists belongs to whoever uses the app. It starts empty for everyone.

- **On the Q2:** hold Play/Pause on any artist row, in All Artists, Top Artists, search results or
  My Artists, to add or remove it (the row's `position` is `fav`; the toast says which). Empty, My
  Artists shows one row: "Hold Play/Pause on any artist to add it here".
- **From a computer:** `.lma/my-artists.txt`, one archive collection identifier per line, with an
  optional name after a tab or a space and `#` for comments:

  ```
  # My Artists for q2-lma: one archive.org collection per line
  GratefulDead    Grateful Dead
  UmphreysMcGee   Umphreys McGee
  ```

  The app reads it at each start and rewrites it (by rename) when the user adds or removes on the
  Q2, keeping comments and order. A line it can't match to a collection shows as "Not found on the
  archive: <id>", so a typo is visible. ARTISTS.md documents how to find an identifier.
- The file isn't in the release zip, so App Store updates keep it. Removing the app removes it.
- A favourite whose collection is gone shows "No longer on the archive".
- **Shuffle** picks a random artist from My Artists, a random year weighted by its show count, a
  random date, then its best recording, played from the start. With My Artists empty, it picks from
  Top Artists and says so. "Shuffle <artist>" does the same within one artist.
- The developer's own 28 artists (ARTISTS.md) are not shipped. They go on the developer's card as
  a `my-artists.txt`, and serve as test data.

## Code layout

```
cmd/q2-lma           main, Q2SPOT/1 handler, screens
cmd/q2ctl            drive the app from a computer
internal/q2spot      protocol (done)
internal/tlsroots    CA roots, clock floor (done)
internal/q2          card files, caps, log (done)
internal/ia          archive.org client, rate limiting, metadata cache, ranking
internal/stream      queue file, fetch-ahead, session tracking, eviction
internal/tag         ID3v2 writer, Ogg comment rewriter, cover scaling
```

**Tests on the computer:** ranking and parsing against recorded archive.org responses
(`testdata/`, fetched once); the client against `httptest`; the stream engine against a fake
firmware that reads `.play`, waits for files and advances `.playing`; screen limits (rows, op
length, context size, depth) checked on every reply in tests.

## Device tests for the first v0 build

1. **Ogg art:** does Now Playing show art embedded in an Ogg file? Decides Ogg vs MP3 as the
   default.
2. **Gapless:** do segues play without a gap, for Ogg and for MP3? Jam bands segue between tracks
   constantly. Q2 Pod has MP3 gapless support; Ogg is unknown.
3. Start-up time for a long first track, then Next, Back and pause, and the cache across replays.

## Not in v0

FLAC; Download show; song search; set lists across versions; a phone page.
