/*
 * This fork's settings (kohnorw/polyfin), drawn after upstream's in their sections: hide
 * specials and exact title matching in Playback, the web-safe video in Conversion, registration
 * in Security and the logo in Web player. Kept apart from upstream's files, so that following
 * upstream touches as few of its lines as can be.
 */
import type { Messages } from '@/i18n'
import { useI18n } from '@/i18n'
import enFork from '@/i18n/en/fork'
import { SettingRow } from '@/ui'
import type { SettingEntry } from './catalog'
import LogoField from './LogoField'
import { FieldRow, SettingsGroup } from './parts'
import type { SectionFormApi } from './SectionForm'

type ForkSettingsText = typeof enFork.fork.settings

/**
 * The fork's words for a setting entry: from the whole messages of the language shown, else in
 * English, for the settings area alone that the palette also searches.
 */
const f = (text: object): ForkSettingsText =>
  'fork' in text ? (text as Messages).fork.settings : enFork.fork.settings

/** The fork's settings, for the section search, the command palette and the field errors. */
export const forkSettingEntries: readonly SettingEntry[] = [
  {
    section: 'playback',
    anchor: 'hide-specials',
    label: (t) => f(t).hideSpecials,
    help: (t) => f(t).hideSpecialsHelp,
    keywords: ['specials', 'season 0'],
  },
  {
    section: 'playback',
    anchor: 'exact-title-match',
    label: (t) => f(t).exactTitleMatch,
    help: (t) => f(t).exactTitleMatchHelp,
    keywords: ['title', 'match', 'versions'],
  },
  {
    section: 'conversion',
    anchor: 'web-safe-video',
    label: (t) => f(t).webSafeVideo,
    help: (t) => f(t).webSafeVideoHelp,
    keywords: ['HEVC', 'AV1', 'Dolby Vision', 'browser'],
  },
  {
    section: 'security',
    anchor: 'registration',
    label: (t) => f(t).registration,
    help: (t) => f(t).registrationHelp,
    keywords: ['register', 'sign up'],
  },
  {
    section: 'web-player',
    anchor: 'brand-logo',
    label: (t) => f(t).brandLogo,
    help: (t) => f(t).brandLogoHelp,
    keywords: ['logo', 'branding'],
    codes: ['invalid_brand_logo'],
  },
]

/** Playback: hide specials and match titles exactly. */
export function ForkPlayback({ form, update, saved }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.fork.settings
  return (
    <SettingsGroup>
      <SettingRow
        kind="switch"
        anchor="hide-specials"
        label={s.hideSpecials}
        help={s.hideSpecialsHelp}
        checked={form.hideSpecials}
        onChange={(hideSpecials) => update({ hideSpecials })}
        saved={saved.hideSpecials}
      />
      <SettingRow
        kind="switch"
        anchor="exact-title-match"
        label={s.exactTitleMatch}
        help={s.exactTitleMatchHelp}
        checked={form.exactTitleMatch}
        onChange={(exactTitleMatch) => update({ exactTitleMatch })}
        saved={saved.exactTitleMatch}
      />
    </SettingsGroup>
  )
}

/** Conversion: the web player offered only the video browsers show. */
export function ForkConversion({ form, update, saved }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.fork.settings
  return (
    <SettingsGroup>
      <SettingRow
        kind="switch"
        anchor="web-safe-video"
        label={s.webSafeVideo}
        help={s.webSafeVideoHelp}
        checked={form.webSafeVideo}
        onChange={(webSafeVideo) => update({ webSafeVideo })}
        saved={saved.webSafeVideo}
      />
    </SettingsGroup>
  )
}

/** Security: the /register page open or closed. */
export function ForkSecurity({ form, update, saved }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.fork.settings
  return (
    <SettingsGroup>
      <SettingRow
        kind="switch"
        anchor="registration"
        label={s.registration}
        help={s.registrationHelp}
        checked={form.registration}
        onChange={(registration) => update({ registration })}
        saved={saved.registration}
      />
    </SettingsGroup>
  )
}

/** Web player: the server's logo in place of Jellyfin's. */
export function ForkWebPlayer({ form, update, error }: SectionFormApi) {
  const { t } = useI18n()
  const s = t.fork.settings
  return (
    <SettingsGroup>
      <FieldRow
        anchor="brand-logo"
        label={s.brandLogo}
        help={s.brandLogoHelp}
        error={error('brand-logo')}
      >
        <LogoField value={form.brandLogo} onValue={(brandLogo) => update({ brandLogo })} />
      </FieldRow>
    </SettingsGroup>
  )
}
