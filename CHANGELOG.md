# Changelog

## [Unreleased]

## [0.3]

- Opening a show starts downloading its first song, so Play Show is often instant.
- A show's own art no longer holds up its first song; it is fetched alongside.
- Lists don't scroll: every row is fitted to the screen. A year's shows read "May 8 · Barton Hall"
  over "Ithaca, NY · SBD 4.8★ · 23 versions".
- The Streaming icon is round: grey in Minimal, as its neighbours, and black in Classic. The app
  writes the theme's icon when it starts; Q2 Pod shows it once it reloads its images (after a theme
  change or a restart).
- Settings → Test Download Speed compares one connection with three at once.

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
