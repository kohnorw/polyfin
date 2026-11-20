import type en from '../en/fork'

const fork: typeof en = {
  fork: {
    settings: {
      hideSpecials: 'Ocultar especiais',
      hideSpecialsHelp:
        'Deixa os especiais das séries (temporada 0) fora das temporadas, das listas de episódios e de A seguir. Ativado por padrão.',
      exactTitleMatch: 'Títulos exatos',
      exactTitleMatchHelp:
        'Deixa de fora as versões cujo arquivo é de outro título: o nome começa com outro título (Smiling Friends para Friends, No Way Home para Brand New Day), o de um filme indica um ano com mais de um ano de diferença, o de um episódio outra temporada ou outro episódio. Um «The» inicial, a pontuação, os espaços e países como (US) não contam. Versões sem nome de arquivo, ou cujo título não se consegue ler nele, são mantidas; lançamentos com nome em outro idioma ficam de fora, como outro título. Ativado por padrão.',
      webSafeVideo: 'Reproduzir todos os arquivos no player web',
      webSafeVideoHelp:
        'Os navegadores dizem reproduzir vídeo HEVC, Dolby Vision e AV1 e depois muitas vezes o mostram preto, com o som e o relógio correndo. Ativado, o player web nunca os recebe como estão: essas versões são reproduzidas convertidas para H.264. Os aplicativos Jellyfin mantêm seus próprios players.',
      registration: 'Cadastro',
      registrationHelp:
        'Permite que qualquer pessoa que abra /register neste servidor crie uma conta de membro, com as configurações padrão, oculta da tela de login. Desativado, a página informa que o cadastro está fechado e volta para o login. Cada endereço pode cadastrar apenas algumas contas antes de esperar, como nos logins com falha.',
      brandLogo: 'Logotipo',
      brandLogoHelp:
        'Exibido no lugar do logotipo do Jellyfin no player web: cabeçalho, menu, telas de abertura e de login e a aba do navegador, e nos aplicativos Jellyfin que abrem o player web (desktop, Android, iOS). Aplicativos com telas próprias, como Infuse ou Swiftfin, mantêm as suas. PNG, JPEG, WebP, GIF ou SVG, até 1 MB; um logotipo largo fica melhor no cabeçalho.',
      brandLogoChoose: 'Escolher uma imagem',
      brandLogoRemove: 'Usar o logotipo do Jellyfin',
      brandLogoNone: 'Logotipo do Jellyfin',
      brandLogoType: 'Escolha uma imagem PNG, JPEG, WebP, GIF ou SVG.',
      brandLogoSize: 'Escolha uma imagem de no máximo 1 MB.',
    },
    users: {
      registrationTitle: 'Cadastro',
      registration: 'Permitir cadastro',
      registrationHelp:
        'Qualquer pessoa que abra /register neste servidor pode criar uma conta de membro com as configurações padrão. Desativado, a página informa que o cadastro está fechado.',
      registrationOpenPage: 'Abrir a página',
      registrationFailed: 'Não foi possível salvar a configuração. Tente novamente.',
      registrationCodes: 'Exigir um código de cadastro',
      registrationCodesHelp:
        'Os visitantes de /register digitam um destes códigos antes do formulário. Escolha ao lado de cada código quantas contas ele pode criar.',
      registrationCodesCount: (n: number) =>
        n === 1 ? '1 código utilizável' : `${n} códigos utilizáveis`,
      registrationCodesNew: 'Novo código',
      registrationCodesFailed: 'Não foi possível alterar os códigos. Tente novamente.',
      registrationCodeUsed: (name: string) => `Usado por ${name}`,
      registrationCodeUnused: 'Não usado',
      registrationCodeUses: (uses: number, max: number) =>
        max === 0 ? `Usado ${uses} vezes` : `Usado ${uses} de ${max}`,
      registrationCodeLimit: (n: number) => (n === 1 ? 'Uso único' : `${n} usos`),
      registrationCodeUnlimited: 'Ilimitado',
      registrationCodeLimitLabel: 'Quantas vezes o código pode ser usado',
      registrationCodeCopy: 'Copiar código',
      registrationCodeCopied: 'Copiado',
      registrationCodeDelete: 'Excluir código',
      deleteSelected: (n: number) => `Excluir selecionados (${n})`,
      deleteSelectedHelp:
        'Estas contas e seu histórico, favoritos e configurações são excluídos definitivamente.',
      previousPage: 'Anterior',
      nextPage: 'Próxima',
      selectAll: 'Selecionar tudo',
      liveTvOn: 'Ativar a TV ao vivo',
      liveTvOff: 'Desativar a TV ao vivo',
      liveTvAccessTitle: 'Acesso à TV ao vivo',
      liveTvAccessHelp:
        'As fontes IPTV e categorias que este usuário recebe, com a permissão de TV ao vivo. Categorias adicionadas depois a uma fonte aparecem, a menos que a fonte inteira esteja desativada.',
      saveLiveTvAccess: 'Salvar o acesso à TV ao vivo',
      noLiveTvSources: 'Nenhuma fonte IPTV',
      noLiveTvSourcesHelp: 'Adicione uma fonte IPTV em Conteúdo › Fontes e atribua-a aqui.',
    },
    libraries: {
      liveTvImage: 'Os aplicativos a mostram no bloco da biblioteca de TV ao vivo.',
    },
  },
}

export default fork
