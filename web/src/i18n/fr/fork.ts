import type en from '../en/fork'

const fork: typeof en = {
  fork: {
    settings: {
      hideSpecials: 'Masquer les épisodes spéciaux',
      hideSpecialsHelp:
        'Retire les épisodes spéciaux des séries (saison 0) de leurs saisons, listes d’épisodes et de « À suivre ». Activé par défaut.',
      exactTitleMatch: 'Titres exacts',
      exactTitleMatchHelp:
        'Retire les versions dont le fichier est d’un autre titre : son nom commence par un autre titre (Smiling Friends pour Friends, No Way Home pour Brand New Day), celui d’un film donne une année à plus d’un an près, celui d’un épisode une autre saison ou un autre épisode. « The », la ponctuation, les espaces et les pays comme (US) ne comptent pas. Les versions sans nom de fichier, ou dont le titre ne s’y lit pas, sont gardées ; les sorties nommées dans une autre langue sont retirées comme un autre titre. Activé par défaut.',
      webSafeVideo: 'Lire tous les fichiers dans le lecteur web',
      webSafeVideoHelp:
        'Les navigateurs disent lire la vidéo HEVC, Dolby Vision et AV1, puis l’affichent souvent en noir, le son et l’horloge tournant. Activé, le lecteur web ne les reçoit jamais telles quelles : ces versions sont lues converties en H.264. Les applications Jellyfin gardent leurs propres lecteurs.',
      registration: 'Inscription',
      registrationHelp:
        'Permet à quiconque ouvre /register sur ce serveur de créer un compte membre, avec les réglages par défaut, masqué de l’écran de connexion. Désactivée, la page indique que les inscriptions sont fermées et ramène à l’écran de connexion. Chaque adresse ne peut créer que quelques comptes avant de devoir attendre, comme pour les connexions échouées.',
      brandLogo: 'Logo',
      brandLogoHelp:
        'Affiché à la place du logo Jellyfin dans le lecteur web : son en-tête, son menu, ses écrans de démarrage et de connexion et l’onglet du navigateur, ainsi que dans les applications Jellyfin qui ouvrent le lecteur web (bureau, Android, iOS). Les applications qui ont leurs propres écrans, comme Infuse ou Swiftfin, gardent le leur. PNG, JPEG, WebP, GIF ou SVG, 1 Mo au plus ; un logo large convient le mieux à l’en-tête.',
      brandLogoChoose: 'Choisir une image',
      brandLogoRemove: 'Utiliser le logo Jellyfin',
      brandLogoNone: 'Logo Jellyfin',
      brandLogoType: 'Choisissez une image PNG, JPEG, WebP, GIF ou SVG.',
      brandLogoSize: 'Choisissez une image de 1 Mo au plus.',
    },
    users: {
      registrationTitle: 'Inscription',
      registration: 'Permettre l’inscription',
      registrationHelp:
        'Quiconque ouvre /register sur ce serveur peut créer un compte membre avec les réglages par défaut. Désactivée, la page indique que les inscriptions sont fermées.',
      registrationOpenPage: 'Ouvrir la page',
      registrationFailed: 'Le réglage n’a pas pu être enregistré. Réessayez.',
      registrationCodes: 'Exiger un code d’inscription',
      registrationCodesHelp:
        'Les visiteurs de /register saisissent un de ces codes avant le formulaire. Choisissez à côté de chaque code combien de comptes il peut créer.',
      registrationCodesCount: (n: number) =>
        n <= 1 ? `${n} code inutilisé` : `${n} codes inutilisés`,
      registrationCodesNew: 'Nouveau code',
      registrationCodesFailed: 'Les codes n’ont pas pu être modifiés. Réessayez.',
      registrationCodeUsed: (name: string) => `Utilisé par ${name}`,
      registrationCodeUnused: 'Inutilisé',
      registrationCodeUses: (uses: number, max: number) =>
        max === 0 ? `Utilisé ${uses} fois` : `Utilisé ${uses} sur ${max}`,
      registrationCodeLimit: (n: number) => (n === 1 ? 'Usage unique' : `${n} usages`),
      registrationCodeUnlimited: 'Illimité',
      registrationCodeLimitLabel: 'Combien de fois le code peut servir',
      registrationCodeCopy: 'Copier le code',
      registrationCodeCopied: 'Copié',
      registrationCodeDelete: 'Supprimer le code',
      deleteSelected: (n: number) => `Supprimer la sélection (${n})`,
      deleteSelectedHelp:
        'Ces comptes et leur historique, favoris et réglages sont supprimés définitivement.',
      previousPage: 'Précédent',
      nextPage: 'Suivant',
      selectAll: 'Tout sélectionner',
      liveTvOn: 'Activer la TV en direct',
      liveTvOff: 'Désactiver la TV en direct',
      liveTvAccessTitle: 'Accès à la TV en direct',
      liveTvAccessHelp:
        'Les sources IPTV et catégories que reçoit cet utilisateur, avec la permission TV en direct. Les catégories ajoutées ensuite à une source apparaissent, sauf si toute la source est désactivée.',
      saveLiveTvAccess: 'Enregistrer l’accès à la TV en direct',
      noLiveTvSources: 'Aucune source IPTV',
      noLiveTvSourcesHelp: 'Ajoutez une source IPTV dans Contenu › Sources, puis attribuez-la ici.',
    },
    libraries: {
      liveTvImage: 'Les applications l’affichent sur la vignette de la bibliothèque TV en direct.',
    },
  },
}

export default fork
