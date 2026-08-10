// A confirmação de ações destrutivas, como PROMESSA.
//
// O `confirm()` do navegador funcionava, e foi trocado por três razões:
//
//   1. ele bloqueia a thread — nada na tela se atualiza enquanto a caixa está
//      aberta, num quadro que existe para se atualizar sozinho;
//   2. a aparência é do sistema operacional, não do produto, e o texto não pode
//      distinguir o que é destrutivo do que é rotina;
//   3. no celular ele aparece colado ao topo do navegador, longe do polegar e
//      longe do que a pessoa apertou.
//
// A forma de promessa é deliberada: mantém os pontos de chamada com o mesmo
// formato de guarda que tinham, uma linha só —
//
//	if (!(await confirmar({ ... }))) return;
//
// A alternativa seria cada tela hospedar o seu próprio diálogo e quebrar a ação
// em duas metades (abrir, e continuar depois), o que espalharia estado de
// interface por todo componente que apaga alguma coisa.

/** Pedido é o que o diálogo mostra e como ele se comporta. */
export interface Pedido {
	/** Cabeçalho curto: o que vai acontecer. */
	titulo: string;
	/** O que a pessoa precisa saber ANTES de decidir — sobretudo o que vai junto. */
	detalhe?: string;
	/** Rótulo do botão que confirma. Um verbo, nunca "OK". */
	acao: string;
	/**
	 * Ação destrutiva pinta o botão de vermelho e é o padrão aqui: tudo que
	 * passa por esta função hoje apaga alguma coisa. Fica explícito mesmo assim,
	 * para o dia em que houver uma confirmação que não destrói nada.
	 */
	destrutivo?: boolean;
}

/** pendente é o pedido em curso, ou null quando não há diálogo aberto. */
let pendente = $state<(Pedido & { responder: (sim: boolean) => void }) | null>(null);

/** pedidoAberto é o que o componente do diálogo observa. */
export function pedidoAberto() {
	return pendente;
}

/**
 * confirmar abre o diálogo e resolve com a escolha: `true` confirma, `false`
 * cancela — inclusive por Esc ou clique fora.
 *
 * Um pedido novo com outro já aberto CANCELA o anterior em vez de enfileirar.
 * Enfileirar mostraria à pessoa uma segunda pergunta que ela não fez, sobre uma
 * ação que ela já esqueceu.
 */
export function confirmar(pedido: Pedido): Promise<boolean> {
	pendente?.responder(false);
	return new Promise<boolean>((resolve) => {
		pendente = {
			...pedido,
			responder(sim: boolean) {
				pendente = null;
				resolve(sim);
			}
		};
	});
}

/** responder fecha o diálogo aberto com a escolha. Usado só pelo componente. */
export function responder(sim: boolean) {
	pendente?.responder(sim);
}
