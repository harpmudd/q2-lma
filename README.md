# q2-lma

The Internet Archive's [Live Music Archive](https://archive.org/details/etree) on the Shanling Q2: a
[Q2 Pod](https://github.com/DiamondBond/q2-pod) card app that browses thousands of artists'
concert recordings and streams them over Wi-Fi, with nothing to set up.

## Install

Needs Q2 Pod 1.5.9 or later. Q2 Pod 1.5.9 shows a streamed song by its file name and "Unknown
artist"; 1.6.0 reads the title, artist and album the app writes into each song.

With the Q2 **powered off** (not asleep), unzip the release's `q2-lma-*.zip` at the root of the
microSD card, so that `.lma/` sits at the top of the card. Put the card back, power on, join Wi-Fi,
and open **Streaming → Live Music Archive**.

## Use

- **My Artists:** your own list. Hold Play/Pause on any artist, anywhere, to add or remove it. It
  starts empty.
- **All Artists:** A to Z; each letter lists its 500 most played artists. **Search Artists** finds
  any of them.
- **Top Artists:** the 500 most played on the archive.
- An artist lists its years; a year lists its shows, one row per date, with the recording the app
  will play: soundboard first, then matrix, then the rest, and among those the best rated.
- A show: **Play Show**, or any song to start there. **More Versions** lists the other recordings
  of that date; pick one and it plays from then on.
- **Shuffle a Show:** a random show by one of My Artists (by a top artist while My Artists is
  empty). On an artist's page, a random show of theirs.
- **Settings:** the cache's size (512 MB to 8 GB, 2 GB at first), Ogg or MP3 first, Clear Cache.
- **Turn Off Live Music Archive** ends the app; it also ends by itself after 15 minutes unused,
  but never while a show streams.

Songs download whole, a couple ahead of the one playing, and the Q2 plays them from the card, with
their titles and the show's cover (or the Live Music Archive's llama). Played shows stay on the
card until the cache is full, oldest first, so playing one again starts at once.

## Files

All on the card under `.lma/`:

| File | What it is |
|---|---|
| `my-artists.txt` | My Artists: one archive.org collection a line, its name after a tab. Edit it on a computer if you like: the collection is the end of the artist's address, `https://archive.org/details/GratefulDead` → `GratefulDead`. |
| `settings.json` | Cache size, Ogg or MP3, and the recordings picked in More Versions. |
| `cache/` | Played shows, one folder each. |
| `meta/` | archive.org's answers, kept a day to a month, 20 MB at most. |
| `log.txt` | What the app did, and why anything failed. |

## Limits

- MP3 and Ogg Vorbis only; recordings with only FLAC or other formats can't play.
- A letter with more than 500 artists shows its 500 most played; search finds the rest.
- A song starts once it has downloaded whole: about 1.5 MB a second on a typical connection, so a
  long jam can take a while to start. Ogg files are half the size of MP3s.
- Some artists aren't on the archive at all, at their own request.

## Errors and logs

A screen that can't load says why, with Retry. A song that can't be fetched is skipped, and the
menu's last rows say so, with Details. `.lma/log.txt` has the rest; a crash's trace is added at the
next start. It never holds anything private.

## Update and remove

Install a new release over the old one; My Artists, settings and the cache stay. To remove the app,
delete `.lma/` with the Q2 powered off.

## Build

Go 1.27 and Python 3, no other dependencies:

    contrib/shanlingq2/build.sh

writes `target/shanlingq2/q2-lma-vX.Y-<commit>.zip`. `go test ./...` runs everything on a computer,
the firmware's side included (a fake archive.org and a fake Stream mode). See
[contrib/shanlingq2/README](contrib/shanlingq2/README) and [docs/](docs/).

## License

Not chosen yet. `internal/tlsroots/cacert.pem` is Mozilla's CA list as curl publishes it
(MPL-2.0). The llama logo belongs to the Internet Archive.
