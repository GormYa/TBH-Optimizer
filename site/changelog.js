// Changelog da landing. Edite SÓ este arquivo pra adicionar novidades.
// Coloque a entrada mais nova NO TOPO da lista. Depois: git add/commit/push -> o CI publica.
//   date:    "AAAA-MM-DD"  (o site formata pra "11 jun 2026" sozinho)
//   version: build(s) do dia, ex. "v1.0.27" ou "v1.0.21–26" (o nº é o run do GitHub Actions)
//   title:   título curto da atualização
//   items:   lista de mudanças (uma frase por item)
window.CHANGELOG = {
	entries: [
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
