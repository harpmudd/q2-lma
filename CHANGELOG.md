# Changelog

## [Unreleased]

## [0.5]

- **Now playing comes first:** while a show streams, the main menu opens with one row saying what
  plays ("Grateful Dead", then "May 8, 1977 · Barton Hall · 1 of 20") and the show's page with the
  song. It stays through the show's last song.
- **Nothing scrolls after Play Show:** messages are short, and long song names and errors run over
  two lines.
- **Replays:** pressing Play on a show you've already heard starts it again straight away.
- **Scrobbling:** on Q2 Pod 1.6.5 or later, shows stay out of your scrobbles, as live recordings
  don't match Last.fm's. They still count as plays. To scrobble them, delete the `scrobble=0` line
  from `.lma/q2app`.
- **Removed:** Test Download Speed.

## [0.4]

The first release: browse and stream thousands of concerts from the Internet Archive's Live Music
Archive on your Shanling Q2.

**Requires** Q2 Pod 1.6.0 or later.

### Features

- **Browse** My Artists, All Artists (A to Z), Top Artists, or search.
- **Best recording first:** soundboard, then matrix, then audience, best rated first. More Versions
  picks another.
- **Instant playback:** a show's first song starts downloading as soon as you open it.
- **Tags and art** in Now Playing, with the Live Music Archive llama when a show has no art.
- **Shuffle a Show** from your artists, or from one artist.
- **Replays from the card:** played shows stay until the cache size you choose is full.
- **Made for the Q2:** lists never scroll, and the icon follows your theme.
