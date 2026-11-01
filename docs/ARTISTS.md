# Artists (checked against archive.org, 2026-10-31)

The app browses by each band's own Live Music Archive collection (`collection:<id>`), not by the
`creator` field: creator search also matches tribute bands (Grateful Dead's creator count includes
them).

## The user's list: on the archive

| # | Artist | Collection | Shows |
|---|---|---|---|
| 1 | Grateful Dead | `GratefulDead` | 18,411 |
| 3 | Widespread Panic | `WidespreadPanic` | 4,504 |
| 6 | String Cheese Incident | `StringCheeseIncident` | 2,105 |
| 7 | moe. | `moe` | 4,664 |
| 8 | The Disco Biscuits | `DiscoBiscuits` | 1,988 |
| 11 | Ratdog | `Ratdog` | 1,317 |
| 12 | Leftover Salmon | `LeftoverSalmon` | 1,327 |
| 13 | Yonder Mountain String Band | `YonderMountainStringBand` | 2,199 |
| 14 | Keller Williams | `KellerWilliams` | 1,413 |
| 16 | Tedeschi Trucks Band / Derek Trucks Band | `TedeschiTrucksBand`, `DerekTrucksBand` | 1,352 + 1,063 |
| 9 | Umphrey's McGee | `UmphreysMcGee` (archive spells it "Umphreys McGee") | 3,392 |
| 18 | Perpetual Groove | `PerpetualGroove` | 920 |
| 19 | Zen Tricksters | `ZenTricksters` | 108 |
| 20 | O.A.R. | `OfARevolution` | 682 |

## The user's list: not on the archive

No band collection, and the matching items are tribute bands only: Phish, The Allman Brothers
Band, Dave Matthews Band, Gov't Mule, Hot Tuna, Medeski Martin & Wood (rechecked by title and
identifier with alternate spellings). Umphrey's McGee was first listed here by mistake: the search
missed the archive's spelling without the apostrophe.
[I] These bands asked archive.org to remove or never host their shows; the cause isn't checked
here, only the absence.

## Verified alternatives (all 14 added, 2026-10-31)

| Artist | Collection | Shows |
|---|---|---|
| Max Creek | `MaxCreek` | 3,104 |
| Phil Lesh & Friends | `PhilLeshandFriends` | 2,896 |
| Railroad Earth | `RailroadEarth` | 2,225 |
| Donna the Buffalo | `DonnatheBuffalo` | 1,254 |
| Furthur | `Furthur` | 1,224 |
| Strangefolk | `Strangefolk` | 1,140 |
| Tea Leaf Green | `TeaLeafGreen` | 1,119 |
| Dark Star Orchestra | `DarkStarOrchestra` | 1,074 |
| Hot Buttered Rum | `HotButteredRum` | 852 |
| Lotus | `Lotus` | 625 |
| Grace Potter & the Nocturnals | `GracePotterandtheNocturnals` | 595 |
| Bob Weir | `BobWeir` | 517 |
| Assembly of Dust | `AssemblyofDust` | 435 |
| Particle | `Particle` | 271 |

## Formats

Older items carry VBR MP3 and Ogg Vorbis derivatives; newer ones may carry only VBR MP3 (for
example `wsp2026-06-26`: Flac and VBR MP3). The app prefers Ogg when present (half the size, so
faster to start), else VBR MP3.

## The developer's own list: 28 artists

The 14 above that are on the archive, plus all 14 alternatives (28). Tedeschi Trucks Band and Derek
Trucks Band are one entry, as the user listed them, browsing both collections (open question, the
default until the user says otherwise).

The archive has 9,333 artist collections under `etree` (2026-10-31), so the app reads artists
live rather than from a fixed list; these 28 are the developer's personal My Artists, put on their card as
`.lma/my-artists.txt` and used as test data. The app ships with My Artists empty.
Match on collection identifiers, never on titles: titles vary in spelling and punctuation.

## Finding an artist's identifier

Open the artist's page on archive.org: the identifier is the last part of its address,
`https://archive.org/details/UmphreysMcGee` → `UmphreysMcGee`. On the Q2 there's no need: hold
Play/Pause on the artist in All Artists, Top Artists or search.
