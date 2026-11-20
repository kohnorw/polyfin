import type en from '../en/fork'

const fork: typeof en = {
  fork: {
    settings: {
      hideSpecials: 'Specials ausblenden',
      hideSpecialsHelp:
        'Lässt die Specials von Serien (Staffel 0) aus ihren Staffeln, Folgenlisten und „Als Nächstes“ weg. Standardmäßig an.',
      exactTitleMatch: 'Titel genau abgleichen',
      exactTitleMatchHelp:
        'Lässt die Versionen weg, deren Datei zu einem anderen Titel gehört: ihr Name beginnt mit einem anderen Titel (Smiling Friends für Friends, No Way Home für Brand New Day), der eines Films nennt ein Jahr, das mehr als ein Jahr abweicht, der einer Folge eine andere Staffel oder Folge. Ein führendes „The“, Satzzeichen, Leerzeichen und Länderkennungen wie (US) zählen nicht. Versionen ohne Dateinamen, oder deren Titel sich nicht daraus lesen lässt, bleiben; in einer anderen Sprache benannte Releases werden als anderer Titel weggelassen. Standardmäßig an.',
      webSafeVideo: 'Jede Datei im Webplayer abspielen',
      webSafeVideoHelp:
        'Browser geben an, HEVC-, Dolby-Vision- und AV1-Video abzuspielen, und zeigen es dann oft schwarz, während Ton und Zeit laufen. Eingeschaltet erhält der Webplayer diese nie unverändert: solche Versionen werden nach H.264 umgewandelt abgespielt. Die Jellyfin-Apps behalten ihre eigenen Player.',
      registration: 'Registrierung',
      registrationHelp:
        'Lässt jeden, der /register auf diesem Server öffnet, ein Mitgliedskonto mit den Standardeinstellungen anlegen, das auf dem Anmeldebildschirm verborgen ist. Ausgeschaltet meldet die Seite, dass die Registrierung geschlossen ist, und führt zurück zur Anmeldung. Jede Adresse kann nur wenige Konten anlegen, bevor sie warten muss, wie bei fehlgeschlagenen Anmeldungen.',
      brandLogo: 'Logo',
      brandLogoHelp:
        'Wird anstelle des Jellyfin-Logos im Webplayer gezeigt: in Kopfzeile, Menü, Start- und Anmeldebildschirm und im Browser-Tab, sowie in den Jellyfin-Apps, die den Webplayer öffnen (Desktop, Android, iOS). Apps mit eigenen Bildschirmen, wie Infuse oder Swiftfin, behalten ihres. PNG, JPEG, WebP, GIF oder SVG, bis 1 MB; ein breites Logo passt am besten in die Kopfzeile.',
      brandLogoChoose: 'Bild wählen',
      brandLogoRemove: 'Jellyfin-Logo verwenden',
      brandLogoNone: 'Jellyfin-Logo',
      brandLogoType: 'Wähle ein PNG-, JPEG-, WebP-, GIF- oder SVG-Bild.',
      brandLogoSize: 'Wähle ein Bild von höchstens 1 MB.',
    },
    users: {
      registrationTitle: 'Registrierung',
      registration: 'Registrierung erlauben',
      registrationHelp:
        'Jeder, der /register auf diesem Server öffnet, kann ein Mitgliedskonto mit den Standardeinstellungen anlegen. Ausgeschaltet meldet die Seite, dass die Registrierung geschlossen ist.',
      registrationOpenPage: 'Seite öffnen',
      registrationFailed: 'Die Einstellung konnte nicht gespeichert werden. Versuche es erneut.',
      registrationCodes: 'Registrierungscode verlangen',
      registrationCodesHelp:
        'Besucher von /register geben vor dem Formular einen dieser Codes ein. Wähle neben jedem Code, wie viele Konten er anlegen darf.',
      registrationCodesCount: (n: number) => (n === 1 ? '1 nutzbarer Code' : `${n} nutzbare Codes`),
      registrationCodesNew: 'Neuer Code',
      registrationCodesFailed: 'Die Codes konnten nicht geändert werden. Versuche es erneut.',
      registrationCodeUsed: (name: string) => `Genutzt von ${name}`,
      registrationCodeUnused: 'Ungenutzt',
      registrationCodeUses: (uses: number, max: number) =>
        max === 0 ? `${uses}-mal genutzt` : `${uses} von ${max} genutzt`,
      registrationCodeLimit: (n: number) => (n === 1 ? 'Einmalig' : `${n} Nutzungen`),
      registrationCodeUnlimited: 'Unbegrenzt',
      registrationCodeLimitLabel: 'Wie oft der Code genutzt werden kann',
      registrationCodeCopy: 'Code kopieren',
      registrationCodeCopied: 'Kopiert',
      registrationCodeDelete: 'Code löschen',
      deleteSelected: (n: number) => `Auswahl löschen (${n})`,
      deleteSelectedHelp:
        'Diese Konten und ihr Verlauf, ihre Favoriten und Einstellungen werden endgültig gelöscht.',
      previousPage: 'Zurück',
      nextPage: 'Weiter',
      selectAll: 'Alle auswählen',
      liveTvOn: 'Live-TV einschalten',
      liveTvOff: 'Live-TV ausschalten',
      liveTvAccessTitle: 'Live-TV-Zugang',
      liveTvAccessHelp:
        'Die IPTV-Quellen und Kategorien, die dieser Benutzer mit der Live-TV-Berechtigung erhält. Später hinzugefügte Kategorien einer Quelle erscheinen, außer die ganze Quelle ist aus.',
      saveLiveTvAccess: 'Live-TV-Zugang speichern',
      noLiveTvSources: 'Keine IPTV-Quellen',
      noLiveTvSourcesHelp:
        'Füge unter Inhalte › Quellen eine IPTV-Quelle hinzu und weise sie hier zu.',
    },
    libraries: {
      liveTvImage: 'Apps zeigen es auf der Kachel der Live-TV-Bibliothek.',
    },
  },
}

export default fork
