# Album art

**Requirement (user, 2026-10-31):** when a show has no album art, streamed songs show the Live Music
Archive llama logo, `assets/lma-logo.png` (300x300 PNG, grey + alpha, not interlaced).

**How the Q2 finds art** (q2-pod docs/internals.md, patch/coverflow.c):
- Now Playing's art comes from stock `toolsGetAlbumCover` → `taglib_album_picture`: art **embedded
  in the file's tags**. [C]
- `cover.jpg` / `folder.jpg` beside the file are read by Coverflow, which lists Library albums only;
  Stream mode's hidden folders are never in the Library. So a folder image alone won't show. [I]
- q2-podcasts embeds covers into MP3s (ID3v2 APIC, baseline JPEG <= 600 px) for exactly this. [C]

**Plan:**
- MP3: write an ID3v2 APIC frame (the show's art, else the logo) before the file is renamed into
  place.
- Ogg: art goes in a Vorbis comment `METADATA_BLOCK_PICTURE`, which means rewriting the comment
  packet and its Ogg pages. Whether the Q2's TagLib build shows Ogg art at all is **unverified**:
  test it on the device before building it. If it doesn't, Ogg loses its size advantage for
  art-bearing songs, and the choice of MP3 vs Ogg needs revisiting.
- The logo is converted once to a baseline JPEG (flattened on black) and embedded in the binary.
- "Show has art": the item's own JPEG/PNG files, not `__ia_thumb.jpg` (archive.org's generated
  tile). To be confirmed against real items during design.

**Open point:** the logo is the Internet Archive's. Fine for personal use; check their terms before
any public release.
