# q2-lma

Stream concerts from the Internet Archive's [Live Music Archive](https://archive.org/details/etree)
on a Shanling Q2 running [Q2 Pod](https://github.com/DiamondBond/q2-pod).

## Install

Requires Q2 Pod 1.6.0 or later.

1. Download `q2-lma-*.zip` from the [latest release](https://github.com/harpmudd/q2-lma/releases/latest).
2. Unzip it at the root of the Q2's card, so `.lma` sits at the top of the card.
3. Restart the Q2.

## Use

Open **Streaming → Live Music Archive** (it needs Wi-Fi).

- **My Artists:** your own list. Hold Play/Pause on any artist to add or remove it.
- **All Artists**, **Top Artists** and **Search Artists** cover the whole archive.
- Choose an artist, a year, then a show. The best recording plays: soundboard first, then the best
  rated. **More Versions** lists the others.
- **Shuffle a Show** plays a random show by one of your artists.
- **Settings:** cache size, Ogg or MP3, Clear Cache, Test Download Speed.

Played shows stay on the card until the cache is full (2 GB to start), so replays start at once.

## Files

In `.lma` on the card:

- `my-artists.txt`: My Artists, one artist a line. You can edit it on a computer: an artist is the
  end of its archive.org address, such as `GratefulDead` from archive.org/details/GratefulDead.
- `settings.json`: your settings.
- `cache`, `meta`: played shows and saved archive.org data.
- `log.txt`: what the app did, for troubleshooting.

## Limits

- MP3 and Ogg only: recordings with only FLAC don't play.
- A song plays once it has downloaded whole, so a long first song takes a moment.
- All Artists shows each letter's 500 most played artists; search finds the rest.
- Some well-known bands aren't on the archive.

## Update and remove

To update, unzip a new release over the old one: My Artists, settings and the cache stay. To
remove the app, delete `.lma` from the card.

## Build

With Go 1.27 and Python 3, `contrib/shanlingq2/build.sh` writes `target/shanlingq2/q2-lma-*.zip`.
`go test ./...` runs the tests.

## Credits

- **Q2 Pod firmware, card-app host, Q2SPOT/1 and Stream mode:** Diamond Bond
  ([DiamondBond](https://github.com/DiamondBond)), [q2-pod](https://github.com/DiamondBond/q2-pod)
- **Reference card app ([q2-subsonic](https://github.com/andsuor/q2-subsonic)), whose streaming,
  launcher and packaging this app follows:** [andsuor](https://github.com/andsuor)
- **Reference card app ([q2-podcasts](https://github.com/m-d-e-v/q2-podcasts)):**
  [m-d-e-v](https://github.com/m-d-e-v)
- **The Live Music Archive:** the [Internet Archive](https://archive.org), and the tapers who
  record and share these shows
- **Llama logo:** the [Internet Archive](https://archive.org/details/etree)
- **CA certificates:** [Mozilla](https://www.mozilla.org), as [curl publishes them](https://curl.se/docs/caextract.html)
- **Live Music Archive app:** [HarpMudd](https://github.com/harpmudd)

## License

MIT (see [LICENSE](LICENSE)). `internal/tlsroots/cacert.pem` is Mozilla's CA list as curl
publishes it (MPL-2.0). The Live Music Archive's llama logo belongs to the Internet Archive.
