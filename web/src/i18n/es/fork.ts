import type en from '../en/fork'

const fork: typeof en = {
  fork: {
    settings: {
      hideSpecials: 'Ocultar especiales',
      hideSpecialsHelp:
        'Deja los especiales de las series (temporada 0) fuera de sus temporadas, listas de episodios y A continuación. Activado por defecto.',
      exactTitleMatch: 'Títulos exactos',
      exactTitleMatchHelp:
        'Deja fuera las versiones cuyo archivo es de otro título: su nombre empieza por otro título (Smiling Friends para Friends, No Way Home para Brand New Day), el de una película da un año a más de un año de distancia, el de un episodio otra temporada u otro episodio. Un «The» inicial, la puntuación, los espacios y los países como (US) no cuentan. Se conservan las versiones sin nombre de archivo, o cuyo título no se puede leer en él; las publicaciones con nombre en otro idioma se dejan fuera como otro título. Activado por defecto.',
      webSafeVideo: 'Reproducir todos los archivos en el reproductor web',
      webSafeVideoHelp:
        'Los navegadores dicen reproducir vídeo HEVC, Dolby Vision y AV1 y luego a menudo lo muestran en negro, con el sonido y el reloj en marcha. Activado, el reproductor web nunca los recibe tal cual: esas versiones se reproducen convertidas a H.264. Las aplicaciones de Jellyfin conservan sus propios reproductores.',
      registration: 'Registro',
      registrationHelp:
        'Permite que cualquiera que abra /register en este servidor cree una cuenta de miembro, con los ajustes por defecto, oculta en la pantalla de inicio de sesión. Desactivado, la página indica que el registro está cerrado y vuelve al inicio de sesión. Cada dirección solo puede registrar unas pocas cuentas antes de esperar, como con los inicios de sesión fallidos.',
      brandLogo: 'Logotipo',
      brandLogoHelp:
        'Se muestra en lugar del logotipo de Jellyfin en el reproductor web: su cabecera, menú, pantallas de inicio y de sesión y la pestaña del navegador, y en las aplicaciones de Jellyfin que abren el reproductor web (escritorio, Android, iOS). Las aplicaciones con pantallas propias, como Infuse o Swiftfin, conservan las suyas. PNG, JPEG, WebP, GIF o SVG, hasta 1 MB; un logotipo ancho queda mejor en la cabecera.',
      brandLogoChoose: 'Elegir una imagen',
      brandLogoRemove: 'Usar el logotipo de Jellyfin',
      brandLogoNone: 'Logotipo de Jellyfin',
      brandLogoType: 'Elija una imagen PNG, JPEG, WebP, GIF o SVG.',
      brandLogoSize: 'Elija una imagen de 1 MB como máximo.',
    },
    users: {
      registrationTitle: 'Registro',
      registration: 'Permitir el registro',
      registrationHelp:
        'Cualquiera que abra /register en este servidor puede crear una cuenta de miembro con los ajustes por defecto. Desactivado, la página indica que el registro está cerrado.',
      registrationOpenPage: 'Abrir la página',
      registrationFailed: 'No se pudo guardar el ajuste. Inténtelo de nuevo.',
      registrationCodes: 'Exigir un código de registro',
      registrationCodesHelp:
        'Los visitantes de /register escriben uno de estos códigos antes del formulario. Elija junto a cada código cuántas cuentas puede crear.',
      registrationCodesCount: (n: number) =>
        n === 1 ? '1 código utilizable' : `${n} códigos utilizables`,
      registrationCodesNew: 'Nuevo código',
      registrationCodesFailed: 'No se pudieron cambiar los códigos. Inténtelo de nuevo.',
      registrationCodeUsed: (name: string) => `Usado por ${name}`,
      registrationCodeUnused: 'Sin usar',
      registrationCodeUses: (uses: number, max: number) =>
        max === 0 ? `Usado ${uses} veces` : `Usado ${uses} de ${max}`,
      registrationCodeLimit: (n: number) => (n === 1 ? 'Un solo uso' : `${n} usos`),
      registrationCodeUnlimited: 'Ilimitado',
      registrationCodeLimitLabel: 'Cuántas veces se puede usar el código',
      registrationCodeCopy: 'Copiar el código',
      registrationCodeCopied: 'Copiado',
      registrationCodeDelete: 'Borrar el código',
      deleteSelected: (n: number) => `Borrar la selección (${n})`,
      deleteSelectedHelp:
        'Estas cuentas y su historial, favoritos y ajustes se borran para siempre.',
      previousPage: 'Anterior',
      nextPage: 'Siguiente',
      selectAll: 'Seleccionar todo',
      liveTvOn: 'Activar la TV en directo',
      liveTvOff: 'Desactivar la TV en directo',
      liveTvAccessTitle: 'Acceso a la TV en directo',
      liveTvAccessHelp:
        'Las fuentes IPTV y categorías que recibe este usuario, con el permiso de TV en directo. Las categorías que una fuente añada después se muestran, salvo que toda la fuente esté desactivada.',
      saveLiveTvAccess: 'Guardar el acceso a la TV en directo',
      noLiveTvSources: 'No hay fuentes IPTV',
      noLiveTvSourcesHelp: 'Añada una fuente IPTV en Contenido › Fuentes y asígnela aquí.',
    },
    libraries: {
      liveTvImage: 'Las aplicaciones la muestran en el mosaico de la biblioteca de TV en directo.',
    },
  },
}

export default fork
