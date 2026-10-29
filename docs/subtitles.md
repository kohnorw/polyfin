# Subtitles

Polyfin's subtitles are a port of Jellyfin 12.4's: the same tracks, the same files, the same delivery to each app, and the same API. This page explains what that means for the titles of a Polyfin server, addon streams included.

## The tracks of a version

A version lists, as Jellyfin lists them:

- the subtitle files of the title (see [Subtitle files](#subtitle-files)), first;
- then the subtitle tracks inside the version's file, as FFmpeg reads them, for addon streams as for local files.

Apps can save the user's subtitle language and subtitle mode, which choose the default track as Jellyfin's `MediaStreamSelector` does.

## Subtitle files

Subtitle files are named as Jellyfin names them, `<name>.<language>[.forced][.sdh|.cc|.hi].<ext>`, in the formats Jellyfin lists: `.srt`, `.ass`, `.ssa`, `.vtt`, `.sub` (MicroDVD, or VobSub beside its `.idx`), `.smi`/`.sami`, `.sup` and `.mks`.

- **Local folders**: the files beside a local file are its files (see [Local folders](local-folders.md)).
- **Every title**: the files in the title's metadata folder, `<data>/metadata/library/<id[:2]>/<id>/`, named `subtitle.<language>…`. Files uploaded by users and files downloaded from a provider are saved there. Files users uploaded to earlier versions of Polyfin move there when Polyfin starts.

Users with subtitle management (and administrators) can upload and delete files from their apps, as on Jellyfin.

## Addon subtitles: search and download

The subtitles Stremio addons offer for a title are Jellyfin subtitle **providers**: an app's **Search subtitles** lists them by language, and the one picked is downloaded into the title's folder, where it is a subtitle file from then on. Nothing is downloaded on its own.

## How tracks reach the app

Each track goes out as Jellyfin's `StreamBuilder` decides, from the app's subtitle profiles: embedded in the stream, as an external file (converted on request to the format the app asks for: SubRip, WebVTT, ASS/SSA, TTML or Jellyfin's JSON), in the HLS master playlist, or burned into the video.

Tracks inside the file are kept in `<data>/subtitles/` once read. They are read 20 seconds after a play of the version starts, so that the reading never slows the start of playback, and before the user picks one; a player asking for one sooner has it read at once:

- **Through the index first.** A Matroska file's index tells where every subtitle block sits, so Polyfin fetches those blocks alone, in seconds, rather than the whole file. The files it makes are FFmpeg's to the byte: SubRip for text, the script for ASS, SUP for PGS.
- **Else with the whole file.** Tracks the index does not give (MP4 files, VobSub, hosts that would take too many requests) are extracted by FFmpeg, every track at once: when the player asks for one, never ahead, as hosts rate limit what they serve. For an addon stream, FFmpeg reads the addon's link itself when the addon allows it, else through Polyfin.

A player asking for a track gets it the moment it is read, while the others still are. An extraction goes on when the player stops waiting, so that asking again finds it under way or done. **Allow subtitle extraction on the fly** is Jellyfin's `EnableSubtitleExtraction`.

A track that could only be burned in is left out when the user may not have the video converted.

### NZB versions

An addon announces none of the subtitles inside an NZB release, so Polyfin finds them itself. Opening a title analyzes its NZB versions (8 at most, 3 at a time), waiting up to 8 seconds so that the version picker lists their subtitle tracks; a version still being read lists them the next time the title opens. Their subtitles are extracted when the player asks for one.

## ASS styles and fonts

ASS tracks keep their styles and positions. When one is burned in, the fonts the file carries are used, and fallback fonts come from the folder set by `POLYFIN_FONTS_DIR`. See [Configuration](configuration.md).

## Image subtitles

PGS and VobSub tracks are served as they are to apps that draw them, and burned into the video for the others. Burning in is a video conversion: it counts toward **Video conversions at once** (see [Playback](playback.md#video-conversions-at-once)) and is described in [Transcoding](transcoding.md).

Guests of a share link see no subtitles.
