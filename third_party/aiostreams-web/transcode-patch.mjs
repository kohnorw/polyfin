// Connects transcode.ts to AIOStreams' browser player (lib/playback/video.ts)
// and its player page. Run by patch.sh from the AIOStreams checkout; each
// change fails when the text it replaces is gone.
import { copyFileSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const web = 'packages/web/src';

function change(file, pairs) {
  let text = readFileSync(file, 'utf8');
  for (const [from, to] of pairs) {
    if (!text.includes(from)) {
      console.error(`transcode-patch: not in ${file}:\n${from}`);
      process.exit(1);
    }
    text = text.split(from).join(to);
  }
  writeFileSync(file, text);
}

copyFileSync(join(here, 'transcode.ts'), `${web}/lib/playback/transcode.ts`);

change(`${web}/lib/playback/video.ts`, [
  [
    "import { t } from '@lingui/core/macro';",
    "import { t } from '@lingui/core/macro';\nimport type { JellyfinClient } from '../client';\nimport { browserPlays, playHls, transcodeUrl } from './transcode';",
  ],
  [
    'export function videoEngine(\n  video: React.RefObject<HTMLVideoElement | null>\n): EngineFactory {',
    'export function videoEngine(\n  video: React.RefObject<HTMLVideoElement | null>,\n  client?: JellyfinClient\n): EngineFactory {',
  ],
  [
    '    let detach = () => {};\n',
    '    let detach = () => {};\n    let stopHls = () => {};\n    let converted = false;\n',
  ],
  [
    '        const handlers: Record<string, () => void> = {',
    `        stopHls();
        stopHls = () => {};
        converted = false;
        const fail = () =>
          emit({
            type: 'failed',
            error: t({
              comment: 'Follows "Playback failed:"',
              message:
                'this browser cannot play this version, even converted by the server. Try another version, an external player or the desktop app.',
            }),
          });
        // The server's conversion, asked once: false when it gives none.
        const convert = async () => {
          converted = true;
          const url = client
            ? await transcodeUrl(client, next.item.Id!, next.source).catch(() => null)
            : null;
          if (file !== next || !url) return false;
          el.removeAttribute('src');
          stopHls = playHls(el, url, fail);
          return true;
        };
        const handlers: Record<string, () => void> = {`,
  ],
  [
    `          error: () =>
            emit({
              type: 'failed',
              error: t({
                comment: 'Follows "Playback failed:"',
                message:
                  'this browser cannot play this version. Nothing is converted on the server, so try another version, an external player or the desktop app.',
              }),
            }),
`,
    `          error: () => {
            if (converted || !client) fail();
            else void convert().then((ok) => ok || fail());
          },
`,
  ],
  [
    '        el.src = next.url;\n      },',
    `        if (client && !browserPlays(next.source))
          void convert().then((ok) => {
            if (!ok && file === next) el.src = next.url;
          });
        else el.src = next.url;
      },`,
  ],
  [
    '      close() {\n        detach();',
    '      close() {\n        stopHls();\n        stopHls = () => {};\n        detach();',
  ],
]);

change(`${web}/pages/player.tsx`, [
  [
    'usePlayer(hostPlayer ?? videoEngine(video), {',
    'usePlayer(hostPlayer ?? videoEngine(video, client), {',
  ],
]);
