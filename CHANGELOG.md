# Changelog

## [Unreleased]

## [0.4]

First release: the Internet Archive's Live Music Archive on the Shanling Q2, as a Q2 Pod card app.
Needs Q2 Pod 1.5.9 or later; 1.6.0 shows each song's title, artist, album and cover in Now
Playing.

- **Browse:** My Artists (your own list: hold Play/Pause on any artist to add or remove it), All
  Artists A to Z, Top Artists and Search Artists; an artist's years; a year's shows, each row the
  date and venue over the city, source and rating; a show's songs.
- **The best recording plays:** soundboard first, then matrix, then the rest, best rated first.
  More Versions picks another recording of the date.
- **Streams through Stream mode:** opening a show starts downloading its first song, so Play Show
  is usually instant; songs download a couple ahead of the one playing, Ogg first and MP3 where a
  show has no Ogg, each tagged with its title, artist, album and the show's cover (or the Live
  Music Archive's llama).
- **Shuffle a Show** from My Artists, or from one artist.
- **Played shows stay on the card** up to a cache size you choose (512 MB to 8 GB), oldest first.
- **Rides out archive.org's hiccups:** passing server errors are waited out, and a song that won't
  come is tried in its other format before it is skipped.
- **Fits the Q2:** lists never scroll, and the round Streaming icon follows Q2 Pod's theme: a black
  llama on grey in Minimal, a white one on black in Classic.
