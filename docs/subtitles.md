# Subtitles

This page explains which subtitles Polyfin offers and how they reach each Jellyfin app: the tracks inside each version's file, and nothing else.

## Only the tracks inside the files

Polyfin offers only the subtitle tracks a version's file carries. This is built in, not a setting:

- the addons' subtitles (SubRip and the like) are neither asked for nor listed, and an app's subtitle search finds none;
- subtitle files beside a local file (`.srt`, `.ass`, `.ssa`, `.vtt`, `.sup`, `.idx`/`.sub`) are not offered;
- subtitle uploads are refused (403), and no file a user added is listed;
- picture subtitles are never read into text (no "(OCR)" copies), and no version's speech is analyzed to sync subtitles.

Apps can save the user's subtitle language and subtitle mode, which choose the default track among the file's own (see [Playback](playback.md#versions-and-analysis)).

### In the web player

jellyfin-web, like other apps, draws text tracks itself: they come as files or in HLS, in the format it asks for, and switching needs no conversion. Image tracks it cannot draw are burned in (see [Image subtitles](#image-subtitles)).

## Text tracks inside files

Some apps take subtitles only as files, jellyfin-web first. These apps get the tracks inside a file from the start of playback.

For a Matroska file, Polyfin reads the track whole through the file's index, which tells where every subtitle line sits:

- Polyfin fetches little more than those lines, many in one request when the host allows it.
- It keeps the track, so it takes seconds the first time and nothing after.
- With [Prepare playback in advance](playback.md#preparing-playback-in-advance) on, Polyfin finds where the subtitle tracks sit before playback starts.

### Hosts that answer one range at a time

A host that answers one byte range at a time would take a request for nearly every line, and providers refuse requests past a rate.

- A track that would take such a host more than 64 requests (the dialogue of a movie, for one) is not read this way, nor offered as a file.
- Before it offers such a track, Polyfin learns whether the host serves several ranges at once, from what it already knows of the host or with one request for two ranges.
- Polyfin logs the tracks it leaves out.

### Tracks read with the whole file

A text track that the index cannot give cheaply is still offered as a file. When it is shown, Polyfin reads the version's file once, from start to end, in the background, in one request at the host's full speed (FFmpeg reads the link itself when the addon allows it, else through Polyfin at a playback's priority), and keeps every text track it finds, so that later playbacks of the version find them at once. The app waits up to 90 seconds for the track; picked again later, it is there. One file is read at a time. Torrents, replays and local folders' files are not read this way.

### Tracks offered after a remux

Tracks left out for that reason, and tracks the index does not locate (as in MP4 files), are offered as files once a remux has read them whole. See [Transcoding](transcoding.md).

## ASS styles and fonts

ASS tracks keep their styles and positions. The fonts the file carries are listed for apps that render ASS.

Fallback fonts, which apps load when a subtitle's own fonts are missing, come from the folder set by `POLYFIN_FONTS_DIR`. See [Configuration](configuration.md).

## Image subtitles

When an app cannot show image subtitles, Polyfin burns the image track inside the file into the video. This is a video conversion: it counts toward **Video conversions at once** (see [Playback](playback.md#video-conversions-at-once)) and is described in [Transcoding](transcoding.md).
