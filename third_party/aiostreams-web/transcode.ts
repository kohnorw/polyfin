/*
 * Polyfin's transcoding for AIOStreams' browser player (added by
 * third_party/aiostreams-web in the Polyfin fork; AGPL-3.0 like the app).
 *
 * The app asks every server to direct-play every version, as its desktop and
 * Android players (mpv, ExoPlayer) play anything. A browser's <video> does not:
 * an MKV, HEVC without the system's decoder, AC-3, E-AC-3, DTS or TrueHD audio
 * fail or play silent. For those, this asks the server for what Jellyfin web
 * clients get, an HLS stream it remuxes or converts (H.264 or the HEVC the
 * browser decodes, AAC stereo), and plays it with hls.js, or natively where
 * the browser plays HLS itself (Safari). Polyfin's playlists list every
 * segment (VOD), so seeking and resuming work as with the file.
 */
import Hls from 'hls.js';
import type { JellyfinClient } from '../client';
import type { MediaStream, SourceInfo } from '../types';

const probe = document.createElement('video');
const plays = (type: string) => {
  try {
    if (probe.canPlayType(type) === 'probably') return true;
    return typeof MediaSource !== 'undefined' && MediaSource.isTypeSupported(type);
  } catch {
    return false;
  }
};

const VIDEO_TYPES: Record<string, string[]> = {
  h264: ['video/mp4; codecs="avc1.640028"'],
  hevc: ['video/mp4; codecs="hvc1.1.6.L120.90"', 'video/mp4; codecs="hev1.1.6.L120.90"'],
  vp9: ['video/webm; codecs="vp9"', 'video/mp4; codecs="vp09.00.40.08"'],
  av1: ['video/mp4; codecs="av01.0.08M.08"', 'video/webm; codecs="av01.0.08M.08"'],
};
const AUDIO_TYPES: Record<string, string[]> = {
  aac: ['audio/mp4; codecs="mp4a.40.2"'],
  mp3: ['audio/mpeg'],
  opus: ['audio/webm; codecs="opus"', 'audio/mp4; codecs="opus"'],
  vorbis: ['audio/webm; codecs="vorbis"'],
  flac: ['audio/mp4; codecs="flac"', 'audio/flac'],
  ac3: ['audio/mp4; codecs="ac-3"'],
  eac3: ['audio/mp4; codecs="ec-3"'],
};
// Containers a <video> opens: MP4 and WebM (Matroska is not one, though
// Chromium sometimes copes; a file that fails falls back below anyway).
const CONTAINERS = ['mp4', 'm4v', 'mov', 'webm'];

const playsAny = (types?: string[]) => !!types?.some(plays);
const codecOf = (s?: MediaStream) => (s?.Codec ?? '').toLowerCase();

/** The browser's video codecs the server may keep or convert to. */
export function browserVideoCodecs(): string[] {
  return ['h264', 'hevc', 'vp9', 'av1'].filter((c) => playsAny(VIDEO_TYPES[c]));
}

/**
 * Whether this browser plays the version as it is: its container, its video
 * codec and its default audio codec. A version with no tracks known yet is
 * tried as it is.
 */
export function browserPlays(source: SourceInfo): boolean {
  const streams = source.MediaStreams ?? [];
  const video = streams.find((s) => s.Type === 'Video');
  const audios = streams.filter((s) => s.Type === 'Audio');
  if (!video && !audios.length) return true;
  const container = (source.Container ?? '').toLowerCase().split(',');
  if (!container.some((c) => CONTAINERS.includes(c))) return false;
  if (video && !playsAny(VIDEO_TYPES[codecOf(video)])) return false;
  const audio =
    audios.find((s) => s.Index === source.DefaultAudioStreamIndex) ?? audios[0];
  if (audio && !playsAny(AUDIO_TYPES[codecOf(audio)])) return false;
  return true;
}

/** The server's HLS stream of the version, converted for this browser; null when it gives none. */
export async function transcodeUrl(
  client: JellyfinClient,
  itemId: string,
  source: SourceInfo
): Promise<string | null> {
  const videoCodecs = browserVideoCodecs();
  const info = await client.post<{ MediaSources?: SourceInfo[] }>(
    `/Items/${itemId}/PlaybackInfo`,
    {
      MediaSourceId: source.Id,
      AudioStreamIndex: source.DefaultAudioStreamIndex ?? undefined,
      EnableDirectPlay: false,
      EnableDirectStream: false,
      EnableTranscoding: true,
      AutoOpenLiveStream: true,
      DeviceProfile: {
        MaxStreamingBitrate: 120_000_000,
        DirectPlayProfiles: [],
        TranscodingProfiles: [
          {
            Type: 'Video',
            Container: 'ts',
            Protocol: 'hls',
            Context: 'Streaming',
            VideoCodec: (videoCodecs.length ? videoCodecs : ['h264'])
              .filter((c) => c === 'h264' || c === 'hevc')
              .join(',') || 'h264',
            AudioCodec: 'aac,mp3',
            MaxAudioChannels: '2',
            BreakOnNonKeyFrames: true,
            MinSegments: 1,
          },
        ],
        SubtitleProfiles: [
          { Format: 'vtt', Method: 'External' },
          { Format: 'srt', Method: 'External' },
          { Format: 'ass', Method: 'External' },
          { Format: 'ssa', Method: 'External' },
          { Format: 'subrip', Method: 'External' },
          { Format: 'pgssub', Method: 'Encode' },
          { Format: 'dvdsub', Method: 'Encode' },
          { Format: 'dvbsub', Method: 'Encode' },
        ],
      },
    }
  );
  const sources = info.MediaSources ?? [];
  const found = sources.find((s) => s.Id === source.Id) ?? sources[0];
  const path = found?.TranscodingUrl;
  if (!path) return null;
  const url = new URL(client.url(path), window.location.href);
  if (client.token && !url.searchParams.has('ApiKey') && !url.searchParams.has('api_key'))
    url.searchParams.set('ApiKey', client.token);
  return url.toString();
}

/** Plays an HLS stream in the element; returns what stops it. */
export function playHls(
  el: HTMLVideoElement,
  url: string,
  onFatal: (message: string) => void
): () => void {
  if (!Hls.isSupported()) {
    // Safari, and iOS's browsers, play HLS themselves.
    el.src = url;
    return () => {};
  }
  const hls = new Hls({
    // Converted segments come as ffmpeg makes them: give the first time.
    manifestLoadingTimeOut: 30_000,
    fragLoadingTimeOut: 60_000,
    maxBufferLength: 60,
    backBufferLength: 30,
  });
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (!data.fatal) return;
    if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
      hls.recoverMediaError();
      return;
    }
    onFatal(data.details);
  });
  hls.loadSource(url);
  hls.attachMedia(el);
  return () => hls.destroy();
}
