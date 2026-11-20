/** This fork's own messages (kohnorw/polyfin), apart from upstream's areas. */
const fork = {
  fork: {
    settings: {
      hideSpecials: 'Hide specials',
      hideSpecialsHelp:
        'Leaves series’ specials (season 0) out of their seasons, episode lists and Next Up. On by default.',
      exactTitleMatch: 'Match titles exactly',
      exactTitleMatchHelp:
        'Leaves out the versions whose file is of another title: its name starts with another title (Smiling Friends for Friends, No Way Home for Brand New Day), a movie’s names a year more than one year off, an episode’s names another season or episode. A leading “The”, punctuation, spaces and country tags such as (US) do not count. Versions without a file name, or whose title cannot be read from it, are kept; releases named in another language are left out, as another title. On by default.',
      webSafeVideo: 'Play every file in the web player',
      webSafeVideoHelp:
        'Browsers say they play HEVC, Dolby Vision and AV1 video, then often show it black, the sound and the clock running. On, the web player is never sent those as they are: such versions play converted to H.264. The Jellyfin apps keep their own players.',
      registration: 'Registration',
      registrationHelp:
        'Lets anyone who opens /register on this server create a member account, with the default settings, hidden from the sign-in screen. Off, the page says registration is closed and leads back to the sign-in screen. Each address may register only a few accounts before it waits, as for failed sign-ins.',
      brandLogo: 'Logo',
      brandLogoHelp:
        'Shown in place of the Jellyfin logo in the web player: its header, menu, splash and sign-in screens and the browser tab, and in the Jellyfin apps that open the web player (desktop, Android, iOS). Apps with screens of their own, such as Infuse or Swiftfin, keep theirs. PNG, JPEG, WebP, GIF or SVG, up to 1 MB; a wide logo suits the header best.',
      brandLogoChoose: 'Choose an image',
      brandLogoRemove: 'Use the Jellyfin logo',
      brandLogoNone: 'Jellyfin logo',
      brandLogoType: 'Choose a PNG, JPEG, WebP, GIF or SVG image.',
      brandLogoSize: 'Choose an image of 1 MB at most.',
    },
    users: {
      registrationTitle: 'Registration',
      registration: 'Let people register',
      registrationHelp:
        'Anyone who opens /register on this server can create a member account with the default settings. Off, the page says registration is closed.',
      registrationOpenPage: 'Open the page',
      registrationFailed: 'The setting could not be saved. Try again.',
      registrationCodes: 'Require a registration code',
      registrationCodesHelp:
        'Visitors of /register type one of these codes before the form. Choose beside each code how many accounts it may create.',
      registrationCodesCount: (n: number) => (n === 1 ? '1 usable code' : `${n} usable codes`),
      registrationCodesNew: 'New code',
      registrationCodesFailed: 'The codes could not be changed. Try again.',
      registrationCodeUsed: (name: string) => `Used by ${name}`,
      registrationCodeUnused: 'Unused',
      registrationCodeUses: (uses: number, max: number) =>
        max === 0 ? `Used ${uses} times` : `Used ${uses} of ${max}`,
      registrationCodeLimit: (n: number) => (n === 1 ? 'Single use' : `${n} uses`),
      registrationCodeUnlimited: 'Unlimited',
      registrationCodeLimitLabel: 'How many times the code can be used',
      registrationCodeCopy: 'Copy code',
      registrationCodeCopied: 'Copied',
      registrationCodeDelete: 'Delete code',
      deleteSelected: (n: number) => `Delete selected (${n})`,
      deleteSelectedHelp:
        'These accounts and their watch history, favorites and settings are deleted for good.',
      previousPage: 'Previous',
      nextPage: 'Next',
      selectAll: 'Select all',
      liveTvOn: 'Turn Live TV on',
      liveTvOff: 'Turn Live TV off',
      liveTvAccessTitle: 'Live TV access',
      liveTvAccessHelp:
        'The IPTV sources and categories this user gets, with the Live TV permission. Categories a source adds later show, unless the whole source is off.',
      saveLiveTvAccess: 'Save Live TV access',
      noLiveTvSources: 'No IPTV sources',
      noLiveTvSourcesHelp: 'Add an IPTV source in Content › Sources, then assign it here.',
    },
    libraries: {
      liveTvImage: 'Apps show it on the Live TV library tile.',
    },
  },
}

export default fork
