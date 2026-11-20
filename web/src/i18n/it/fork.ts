import type en from '../en/fork'

const fork: typeof en = {
  fork: {
    settings: {
      hideSpecials: 'Nascondi gli speciali',
      hideSpecialsHelp:
        'Esclude gli speciali delle serie (stagione 0) dalle loro stagioni, dagli elenchi degli episodi e da Prossimi. Attivo per impostazione predefinita.',
      exactTitleMatch: 'Titoli esatti',
      exactTitleMatchHelp:
        'Esclude le versioni il cui file è di un altro titolo: il nome inizia con un altro titolo (Smiling Friends per Friends, No Way Home per Brand New Day), quello di un film indica un anno distante più di un anno, quello di un episodio un’altra stagione o un altro episodio. Un «The» iniziale, la punteggiatura, gli spazi e i paesi come (US) non contano. Le versioni senza nome di file, o il cui titolo non si legge, sono mantenute; le uscite con nome in un’altra lingua sono escluse come altro titolo. Attivo per impostazione predefinita.',
      webSafeVideo: 'Riproduci ogni file nel lettore web',
      webSafeVideoHelp:
        'I browser dichiarano di riprodurre video HEVC, Dolby Vision e AV1, poi spesso li mostrano neri, con l’audio e l’orologio che scorrono. Attivo, il lettore web non li riceve mai così come sono: queste versioni vengono riprodotte convertite in H.264. Le app Jellyfin mantengono i propri lettori.',
      registration: 'Registrazione',
      registrationHelp:
        'Consente a chiunque apra /register su questo server di creare un account membro, con le impostazioni predefinite, nascosto nella schermata di accesso. Disattivata, la pagina indica che le registrazioni sono chiuse e riporta all’accesso. Ogni indirizzo può registrare solo pochi account prima di dover attendere, come per gli accessi falliti.',
      brandLogo: 'Logo',
      brandLogoHelp:
        'Mostrato al posto del logo di Jellyfin nel lettore web: intestazione, menu, schermate di avvio e di accesso e scheda del browser, e nelle app Jellyfin che aprono il lettore web (desktop, Android, iOS). Le app con schermate proprie, come Infuse o Swiftfin, mantengono il loro. PNG, JPEG, WebP, GIF o SVG, fino a 1 MB; un logo largo si adatta meglio all’intestazione.',
      brandLogoChoose: 'Scegli un’immagine',
      brandLogoRemove: 'Usa il logo di Jellyfin',
      brandLogoNone: 'Logo di Jellyfin',
      brandLogoType: 'Scegli un’immagine PNG, JPEG, WebP, GIF o SVG.',
      brandLogoSize: 'Scegli un’immagine di 1 MB al massimo.',
    },
    users: {
      registrationTitle: 'Registrazione',
      registration: 'Consenti la registrazione',
      registrationHelp:
        'Chiunque apra /register su questo server può creare un account membro con le impostazioni predefinite. Disattivata, la pagina indica che le registrazioni sono chiuse.',
      registrationOpenPage: 'Apri la pagina',
      registrationFailed: 'Impossibile salvare l’impostazione. Riprova.',
      registrationCodes: 'Richiedi un codice di registrazione',
      registrationCodesHelp:
        'I visitatori di /register digitano uno di questi codici prima del modulo. Scegli accanto a ogni codice quanti account può creare.',
      registrationCodesCount: (n: number) =>
        n === 1 ? '1 codice utilizzabile' : `${n} codici utilizzabili`,
      registrationCodesNew: 'Nuovo codice',
      registrationCodesFailed: 'Impossibile modificare i codici. Riprova.',
      registrationCodeUsed: (name: string) => `Usato da ${name}`,
      registrationCodeUnused: 'Non usato',
      registrationCodeUses: (uses: number, max: number) =>
        max === 0 ? `Usato ${uses} volte` : `Usato ${uses} su ${max}`,
      registrationCodeLimit: (n: number) => (n === 1 ? 'Uso singolo' : `${n} usi`),
      registrationCodeUnlimited: 'Illimitato',
      registrationCodeLimitLabel: 'Quante volte si può usare il codice',
      registrationCodeCopy: 'Copia il codice',
      registrationCodeCopied: 'Copiato',
      registrationCodeDelete: 'Elimina il codice',
      deleteSelected: (n: number) => `Elimina la selezione (${n})`,
      deleteSelectedHelp:
        'Questi account e la loro cronologia, i preferiti e le impostazioni vengono eliminati per sempre.',
      previousPage: 'Precedente',
      nextPage: 'Successiva',
      selectAll: 'Seleziona tutto',
      liveTvOn: 'Attiva la TV in diretta',
      liveTvOff: 'Disattiva la TV in diretta',
      liveTvAccessTitle: 'Accesso alla TV in diretta',
      liveTvAccessHelp:
        'Le sorgenti IPTV e le categorie che questo utente riceve, con il permesso TV in diretta. Le categorie aggiunte in seguito a una sorgente compaiono, a meno che l’intera sorgente sia disattivata.',
      saveLiveTvAccess: 'Salva l’accesso alla TV in diretta',
      noLiveTvSources: 'Nessuna sorgente IPTV',
      noLiveTvSourcesHelp: 'Aggiungi una sorgente IPTV in Contenuti › Sorgenti, poi assegnala qui.',
    },
    libraries: {
      liveTvImage: 'Le app la mostrano sul riquadro della libreria TV in diretta.',
    },
  },
}

export default fork
