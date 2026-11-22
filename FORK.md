# kohnorw/polyfin

This is a fork of [moodiness/polyfin](https://github.com/moodiness/polyfin). It follows upstream automatically and keeps the changes below on top.

## What this fork adds

**Look**

- **The AIOStreams look** is the web player's default theme, `aiostreams`, served from `internal/accounts/aiostreams_theme.css`. It has near-black pages, white pill buttons, Inter type, rounded cards, Apple-style subtitles, a slim player bar and a small skip button, plus mobile and desktop-app fixes. Upstream's themes (LumaaGlaass, jellyfin-web's own look, Custom) are still offered under **Settings › Web player** and on each user's page.
- jellyfin-web offers its dark display theme only, so the look never sits on top of another theme.
- **Logo** (**Settings › Web player › Logo**): the server's logo replaces Jellyfin's in the web player and on `/register`.
- In the web player, a **subtitle panel** controls size, height and timing.
- The admin app uses the AIOStreams palette and Inter (`web/src/fork-theme.css`).

**Users and registration**

- **`/register`**: visitors create their own member account. Turn it on or off under **Settings › Security › Registration**. Registration codes can have a usage limit (**Users › Registration**). Xtream sign-up gives a new account its own IPTV login, which a daily task checks.
- **Users list**: select users, then turn Live TV on or off for all of them or delete them at once. The list shows 25 users per page.
- New users play **one stream at a time** by default.

**Live TV**

- Live TV is **off for new users and groups** until it is turned on.
- **Live TV access** (a user's or group's **Visible libraries**) gives or takes IPTV sources and their categories. It works alongside upstream's channel rules.
- A **TV Categories** view with a **Favorites** folder, and a programme schedule on a channel's page, including in Streamyfin.
- No Replay or record buttons. Live TV libraries can be renamed and given an image.
- **Channel logos** show whatever type their server gives them, as IPTV providers often serve them as `application/octet-stream`, and SVG logos are drawn into PNG pictures with FFmpeg. Programmes give their channel's logo with `ChannelInfo`, as Jellyfin before 12 did, so jellyfin-web's programme cards show it.

**Playback and library**

- **Hide specials** (season 0) is on by default (**Settings › Playback**).
- **Match titles exactly** leaves out other titles' files (**Settings › Playback**).
- Pre-release rips (cam, telesync, telecine, screener) are hidden by default, using upstream's **Hidden releases**.
- **Play every file in the web player**: browsers get H.264 in place of HEVC, Dolby Vision or AV1, which they often show black (**Settings › Conversion**).
- Usenet streams have their own size record, so AIOStreams NZBs play.
- Administrators can stop any playback from **Now Playing**. Playbacks paused for over an hour end.
- Cinemeta is searched when the user's own search catalogs find nothing.
- TMDB answers are kept for two weeks, across restarts. Settled titles' descriptions are kept for six weeks.
- `adjacentTo` is answered, so apps find the next episode. NostalgiaTV-style episode listings and genres are supported (`episodePoolListing` in episodepool.go answers a library's episodes before upstream's `episodeListing`, whose bound test is skipped). Jellyfin for Roku plays converted versions.
- A chosen audio language wins over a file's default audio flag.

**NVIDIA**

Upstream ships FFmpeg 9.0, built for NVENC API 13.1, which needs NVIDIA driver 610 or newer. On older drivers, such as the R580 drivers for Pascal (GTX 10) cards, Polyfin downloads the same FFmpeg built for API 13.0 into the data folder on its own. It converts on the processor until the download is done. **Settings › Conversion** and **System › Health** show which FFmpeg is in use. Pass the GPU to the container as before, with the NVIDIA container runtime.

## How the fork is kept apart from upstream

To keep upstream's updates merging cleanly, the fork's code lives in its own files wherever it can. Their names contain `fork`: `internal/accounts/fork_settings.go`, `internal/admin/users_fork.go`, `internal/jellyfin/branding_fork.go`, `web/src/features/settings/forkSettings.tsx`, `web/src/features/users/forkUsers.tsx`, `web/src/i18n/*/fork.ts`, `web/src/fork-theme.css`, `cmd/polyfin/fork.go` and `internal/database/migrate_fork.go`.

**Database migrations.** The fork's migrations live in `internal/database/migrations_fork/` and run after upstream's, as versions 100001 and up. That way upstream can keep numbering its own without ever colliding.

A database migrated by the fork before November 2026 used the numbers 0141–0166, which upstream has since taken. It is moved over automatically the first time the new build starts (`adoptForkMigrations`). Your settings, registration codes, Xtream logins and Live TV access are kept, and upstream's migrations then run. Backups made before that move restore only into an older build: take a new backup after updating.

## Automatic updates

`.github/workflows/sync-upstream.yml` runs every 15 minutes and does the following:

1. Merges upstream's `main` (`scripts/sync-upstream.sh`), keeping the fork's own workflows only.
2. Builds the admin app and the server and runs the tests of the packages the fork changes.
3. Pushes the result to `main`. That builds and publishes `ghcr.io/kohnorw/polyfin:latest` (`docker.yml`).

If the merge conflicts or a test fails, `main` is left alone. A pull request from `upstream-sync`, titled **Sync with upstream needs a hand**, says what went wrong. The sync resumes by itself once that pull request is merged.

**One-time setup.** GitHub won't let a workflow's own token push commits that change workflow files, and upstream's commits often do. To allow it:

1. Create a [fine-grained personal access token](https://github.com/settings/personal-access-tokens/new) for `kohnorw/polyfin` with **Contents**, **Pull requests** and **Workflows** set to *Read and write*.
2. Save it as the repository secret `SYNC_TOKEN` (**Settings › Secrets and variables › Actions**).

Without it, the first run that can't push fails (GitHub emails you) and the sync pauses, marked by the branch `sync-paused`. The runs after that stop at once instead of retrying. Adding `SYNC_TOKEN` resumes it on its own; deleting `sync-paused` makes it try again without the token.
