# Changelog

## [Unreleased]

## [0.2]

- Pressing Play Show again while it starts no longer starts the download over.
- The show's page shows the song downloading, with a bar, and what plays; the toast says how long
  the first song should take.
- archive.org's passing errors (500s, timeouts) are waited out instead of skipping the song; a song
  that still won't come is tried in its other format (MP3 for Ogg) before it is skipped.

## [0.1]

First version, for testing on a Q2.

- Browse the Live Music Archive: My Artists, All Artists A to Z, Top Artists, Search Artists; an
  artist's years; a year's shows; a show's songs.
- The best recording of each date plays: soundboard, then matrix, then the rest, best rated first.
  More Versions picks another.
- Streams through Q2 Pod's Stream mode, with titles and the show's cover (or the llama logo) in each
  song, Ogg first and MP3 when a show has no Ogg.
- Shuffle a Show, from My Artists or one artist.
- Played shows stay on the card up to a cache size you choose.
