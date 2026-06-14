// Changelog da landing. Edite SÓ este arquivo pra adicionar novidades.
// Coloque a entrada mais nova NO TOPO da lista. Depois: git add/commit/push -> o CI publica.
//   date:    "AAAA-MM-DD"  (o site formata pra "11 jun 2026" sozinho)
//   version: build(s) do dia, ex. "v1.0.27" ou "v1.0.21–26" (o nº é o run do GitHub Actions)
//   title:   título curto da atualização
//   items:   lista de mudanças (uma frase por item)
window.CHANGELOG = {
	entries: [
        {
            data: "2026-06-13",
            version: "v1.0.39-v1.0.40",
            title: "Hotfix: xp do herói estava com float64",
            items: [
                "Por pegar o xp do jogo como float64, o json.Unmarshal falhava e o monitoramento ficava retentando a cada 5s pra sempre"
            ]
        },
        {
            date: "2026-06-13",
            version: "v1.0.38",
            title: "New Heroes & Inventory tabs, real synthesis odds, and a live Cube panel",
            items: [
                "New Heroes tab: see every hero with all 10 equipped slots, and click any item to inspect its real enchant rolls — each roll is measured against the material's min–max range, so you can tell a near-perfect roll from a floor one (shown as \"% of ceiling\"), with a per-hero average.",
                "New Inventory tab: everything stored outside your heroes — bag, storage and trading post — grouped by type and grade so you can see what's ready to fuse at a glance.",
                "Synthesis odds are now real, read straight from the game's data: each fusible group shows how many items you need and the actual chance to rise or stay a grade — Common is a guaranteed upgrade, while Divine only climbs about 9% of the time. Fusing never lowers a grade, so that's no longer implied. Locked items are left out of the count, since they can't go into the Cube.",
                "New live Cube panel: an EXP progress bar to the next Cube level, plus a row showing which Cube functions you've unlocked — Synthesis, Alchemy, Crafting, Decoration, Engraving, Inscription, Offering and Extraction.",
            ],
        },
        {
            date: "2026-06-13",
            version: "v1.0.37",
            title: "Combined Gold+XP pick, smarter map-switch handling & a 16-language console",
            items: [
                "New \"Best Map · Gold + XP\" recommendation: scores every map against the best gold/h and the best xp/h and adds them up, so a map with slightly less gold but far more XP wins over the pure-gold pick.",
                "The stats panel now opens sorted by stage instead of by gold, and the \"Best map for:\" ranking starts empty — pick Gold or XP yourself when you want it ranked.",
                "Switching maps no longer corrupts a map's stats: the timer re-anchors on the new map and a run that mixed two maps is never credited to the wrong one. Since the app reads the game only through its periodic saves, a run it joined partway can't be timed accurately — so that first partial run is skipped (with a console note explaining why) and the next full run counts.",
                "Fixed a +0 gold reading after buying runes: a clear where you spent gold mid-cycle used to register +0 gold (zeroing the map's gold/h). The optimizer now adds the rune-upgrade cost back — real gain = gold change + amount spent — and records the true gold, even on a brand-new map. If the spend can't be attributed to rune upgrades, it falls back to skipping that run.",
                "The live console now follows the selected language: every monitor message is fully translated across all 16 languages (it used to stay in Portuguese).",
                "Chest tracker ordering fixed: the highest-level maps are listed first and the farm suggestion targets the highest-level chest you can reach, instead of pointing at 1-1.",
            ],
        },
        {
            date: "2026-06-12",
            version: "v1.0.36",
            title: "Tracker de baús em tempo real — saiba o que farmar e quando",
            items: [
                "Novo card \"Tracker de Baús\" no painel: cada baú é detectado automaticamente ao dropar, calculando individualmente as janelas deslizantes de recarga (cooldowns de 9 min para chefes e 11 min para monstros).",
                "Mapeamento inteligente da Steam: o rastreador lê de forma assíncrona o cache local da Steam (ScanSteamInventoryCache) para obter os temporizadores oficiais de drop configurados na API do jogo.",
                "Auto-correção de drops retroativa: corrige automaticamente o ID de baús associados incorretamente a fases de nível baixo (como 1-9 após trocar de mapa rápido) assim que a cache da Steam atualiza na máquina.",
                "Filtro e estabilização de grade: adicionado um botão de alternância no painel para ocultar baús comuns de monstros (focando apenas nos azuis de boss), além de ordenamento estável por raridade para evitar que os baús fiquem mudando de posição visual.",
            ],
        },
        {
            date: "2026-06-12",
            version: "v1.0.35",
            title: "Domínio próprio adicionado",
            items: [
                "Agora não é mais apenas um subdomínio do Cloudflare",
                "Melhorias para o SEO do projeto",
                "URL antiga agora redireciona dinamicamente para taskbarhero.fun",
            ],
        },
        {
            date: "12/06/2026",
            version: "v1.0.34",
            title: "Hotfix — guardas do rastreador mais espertas e dados de recompensa auditados",
            items: [
                "Upar herói novo em mapa baixo não é mais confundido com morte: a projeção de XP de um clear agora usa a retenção média do time ativo (herói baixo retém ~100% enquanto os altos ficam no piso de 1%) em vez do nível mais alto do time.",
                "Ouro e XP esperados das fases auditados contra as tabelas do jogo: um fator empírico antigo dividia tudo por 10 e era compensado em silêncio pela calibração — os valores exibidos agora são os oficiais (ex.: 3-9 espera 19,77M de XP por clear a 100%).",
                "Mensagem do descarte por XP acima do esperado agora deixa claro que o valor é XP, não segundos.",
            ],
        },
        {
            date: "11/06/2026",
            version: "v1.0.33",
            title: "Hotfix — tentativas de boss não inflam mais o tempo da fase",
            items: [
                "Tentativas rápidas de boss em outro mapa não inflam mais o tempo da fase: janela com ganhos de um clear normal mas tempo bem acima da própria média é descartada (era o caso de tentar o boss do ato 3x e voltar — registrava 432s numa fase de ~260s). Se o tempo maior se repetir em 3 corridas seguidas, é aceito como o novo ritmo real da fase.",
            ],
        },
        {
            date: "11/06/2026",
            version: "v1.0.32",
            title: "O otimizador agora fala 16 idiomas",
            items: [
                "Seletor de idioma no painel com as 16 línguas do jogo — interface inteira traduzida em todas elas, e os nomes de itens, fases, monstros, runas, pets e baús extraídos direto dos arquivos do jogo (idênticos aos que você vê jogando).",
                "Os pacotes de idioma ficam no site e são baixados só quando você troca de língua (com cache offline) — o app não ficou nem 1 KB mais pesado.",
                "O site ganhou versão em 16 idiomas (tbh-optimizer.pages.dev/en/, /ja/, /de/, …) com seletor de idioma no topo e SEO internacional (hreflang + sitemap), gerado automaticamente a cada publicação.",
                "Aba de materiais com o tooltip IGUAL ao do jogo: grau, tipo com ícone (decoração/gravação/inscrição/fabricação/oferenda/pedra da alma), descrição oficial, efeitos por slot com tier e faixa de atributo (ex.: \"Redução de Recarga +5,5~7,0%\"), preço de venda, e — nos materiais de fabricação — em quais receitas e em qual nível do Cubo ele é usado. Tudo nas 16 línguas, com os textos exatos do jogo.",
                "Painel ~420 KB mais leve: trocada a build de desenvolvimento do Vue pela de produção.",
                "Pedras da Alma passaram a usar o nome oficial do jogo (que agora as localiza) em vez do nosso nome aproximado.",
                "Busca global agora encontra materiais também — o resultado mostra a categoria (decoração, gravação, …) e abre direto o detalhe na aba de materiais.",
                "Rodapé da barra lateral reorganizado: idioma, console, zerar histórico e encerrar em sequência, com a versão do app e a dos dados do jogo numa linha só, mais compacta.",
            ],
        },
        {
            date: "11/06/2026",
            version: "v1.0.31",
            title: "Hotfix — fases não viram mais \"antiga\" ao reiniciar o app",
            items: [
                "Corrigido: reiniciar o app (ex.: auto-update) marcava TODAS as fases como \"antiga\" e derrubava o XP/h pra valores sem base — o nível em que cada fase foi medida não estava sendo salvo no histórico em disco e se perdia a cada restart. A primeira corrida em cada fase re-carimba e normaliza tudo.",
            ],
        },
        {
            date: "11/06/2026",
            version: "v1.0.30",
            title: "Casos de morte agora tem fallback para não inflar o tempo",
            items: [
                "Detecção direta de morte pela wave: se a wave recuar no mesmo mapa dentro do ciclo (a fase reiniciou), a janela é descartada — funciona em qualquer fase, com ou sem histórico.",
                "Compra de runa no meio do ciclo não esconde mais uma morte em fase já medida: XP muito acima da média própria descarta a janela em vez de só neutralizar o ouro.",
                "Fases com etiqueta \"antiga\" agora mostram tempo, ganho por corrida e ganho por hora contando a mesma história (tudo reprojetado pro seu nível atual) — antes o por corrida exibia a medição velha ao lado de um por hora recalculado.",
            ],
        },
        {
            date: "11/06/2026",
            version: "v1.0.29",
            title: "Detecção de troca de mapa ainda mais esperta",
            items: [
                "Trocar de mapa no meio do ciclo não é mais confundido com auto-avanço: os saves de meio de ciclo agora servem de testemunha de onde as waves realmente rodaram, e janelas que misturam mapas são descartadas em vez de creditar a corrida no mapa errado.",
                "Saves atrasados logo após uma troca de mapa (ex.: uma \"corrida\" de 6 segundos com a recompensa do mapa anterior) não viram mais corrida em fases sem histórico — o tempo estimado da fase agora também serve de piso.",
                "Morrer numa fase nova não infla mais a média: uma janela com falhas + clear rende um múltiplo do XP de um clear só, e agora isso é detectado comparando com o XP projetado pelas outras fases medidas — mesmo quando uma compra de runa no meio esconde o ouro.",
            ],
        },
        {
            date: "11/06/2026",
            version: "v1.0.28",
            title: "Atualizações no site",
            items: [
                "Diversas melhorias de SEO no site https://tbh-optimizer.pages.dev/",
                "Adição de mais detalhes e do changelog correto por versão e atualização do sistema",
                "Adição do repositório do github para que possam analisar o código se desejarem, adicionarem issues e sugestões de melhorias",
                "Multiplicador de xp de fases antigas só vem de fases que foram feitas após essa atualização, pois tem nível do heró para usar de base",
            ],
        },
		{
			date: "11/06/2026",
			version: "v1.0.27",
			title: "Ranking honesto e o site do projeto",
			items: [
				"Fases medidas num nível antigo do herói são recalculadas pro seu nível atual, ganham a etiqueta \"antiga\" e param de aparecer no topo do ranking sem merecer.",
				"Novo site com changelog por versão e link pro código aberto no GitHub.",
				"Ajustes pra o otimizador aparecer nas buscas por Taskbar Hero / TBH.",
			],
		},
		{
			date: "10/06/2026",
			version: "v1.0.21–26",
			title: "Mapa das fases, XP mantido e detecção de conclusão",
			items: [
				"Início do mapa visual na aba de fases e tooltips nos itens.",
				"Cálculo do XP mantido (penalidade por estar acima do nível da fase) totalmente refeito pra bater com o jogo.",
				"Ao concluir por auto-progresso, o tempo da corrida passa a ser creditado na fase certa — não na seguinte.",
				"Troca de mapa e auto-progresso mais espertos pra decidir quando uma corrida conta pra média.",
				"Corrigidos valores anormais de ouro/XP esperado que vinham de uma média errada da fase 1-1.",
			],
		},
		{
			date: "09/06/2026",
			version: "v1.0.13–20",
			title: "Runas, pets, drops e busca global",
			items: [
				"Nova aba de runas: atributos, ouro atual, runas bloqueadas e as que dá pra comprar.",
				"Nova aba de pets: bônus total, quais você já tem e onde conseguir os que faltam.",
				"Busca global — procure qualquer coisa em qualquer aba.",
				"Aba de itens reformulada (cards com mais detalhes) e barra lateral responsiva.",
				"Console na própria página pra acompanhar o que o app está fazendo em tempo real.",
				"Correções de drop: pedra da alma voltou a aparecer, baús de chefe deixaram de mostrar 0% e as porcentagens foram acertadas.",
				"Ouro e XP não estouram mais quando um herói sobe de nível ou quando você vende item / compra runa.",
			],
		},
		{
			date: "08/06/2026",
			version: "v1.0.1–12",
			title: "Lançamento",
			items: [
				"Primeira versão: o painel lê seu save do Taskbar Hero em tempo real e calcula ouro/h e xp/h de cada fase, apontando o melhor mapa.",
				"Veja os monstros e os drops de cada fase.",
				"Penalidade de XP por nível do personagem considerada nos cálculos, usando só os heróis ativos.",
				"Auto-atualização: o app baixa a versão nova e recarrega a aba sozinho.",
				"Log com data e hora de cada corrida concluída.",
			],
		},
	],
};
