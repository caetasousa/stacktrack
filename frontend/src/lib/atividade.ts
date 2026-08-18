// O histórico de um card, em frases.
//
// A tradução vive aqui, e não no servidor, por uma razão: a API devolve o que
// ACONTECEU (tipo + dados), e a frase é apresentação. Montar o texto em Go
// congelaria idioma e redação no backend, e mudar "moveu" para "arrastou"
// viraria deploy de API.
//
// E vive fora do componente porque é lógica pura com muitos casos — é onde um
// `else` esquecido produz uma linha em branco no meio do histórico sem quebrar
// nada, que é o defeito mais fácil de não notar.

/**
 * O payload de um evento, em forma permissiva.
 *
 * Campos opcionais em vez de uma união discriminada por tipo, e a razão é o que
 * acontece quando o servidor ganha um tipo novo: com união, a tela QUEBRA ao
 * receber algo que ela ainda não conhece; assim ela lê o que reconhece e ignora
 * o resto. Num log append-only isso não é conveniência — linhas antigas foram
 * gravadas com formatos antigos, e elas não vão mudar.
 */
export interface DadosDoEvento {
	cardId?: string;
	titulo?: string;
	tituloAnterior?: string;
	coluna?: string;
	deColuna?: string;
	cor?: string;
	/** O nome do que foi mexido: a etiqueta, o anexo, a pessoa, a checklist. */
	alvo?: string;
	nome?: string;
	nomeAnterior?: string;
	email?: string;
	papel?: string;
	papelAnterior?: string;
	fundo?: string;
}

/** Mantido pelo nome antigo: o histórico do card já o usava. */
export type DadosDoCard = DadosDoEvento;

export interface Atividade {
	seq: number;
	tipo: string;
	autorId: string;
	autorNome: string;
	/**
	 * O email de quem agiu — porque NOME NÃO IDENTIFICA NINGUÉM.
	 *
	 * Dois "Ana Silva" no mesmo quadro tornam a auditoria inútil justamente
	 * quando ela é necessária: não há como saber, olhando, se são duas pessoas
	 * ou a mesma.
	 *
	 * Vazio quando a conta foi removida. O histórico sobrevive à conta de
	 * propósito — o LEFT JOIN preserva a linha —, então a tela precisa aguentar
	 * a ausência.
	 */
	autorEmail: string;
	dados?: DadosDoEvento;
	ocorridoEm: string;
}

/**
 * frase descreve uma entrada do histórico, já sem o nome de quem agiu — a tela
 * mostra o autor em negrito e a frase logo depois.
 *
 * Devolve string vazia para o que não sabe descrever, e quem chama omite a
 * linha. É deliberado: um tipo de evento novo aparece como silêncio, e não como
 * "undefined" no meio da conversa.
 */
export function frase(a: Atividade): string {
	const d = a.dados ?? {};
	const alvo = d.alvo ? `"${d.alvo}"` : '';

	switch (a.tipo) {
		case 'card.criado':
			return d.coluna ? `criou este card em ${d.coluna}` : 'criou este card';

		case 'card.movido':
			// O "de onde" é o que dá sentido ao movimento. Quando ele falta — em
			// eventos gravados antes de o payload carregar a origem — a frase
			// encolhe em vez de mentir.
			if (d.deColuna && d.coluna && d.deColuna !== d.coluna) {
				return `moveu de ${d.deColuna} para ${d.coluna}`;
			}
			return d.coluna ? `reordenou em ${d.coluna}` : 'moveu este card';

		case 'card.alterado':
			return d.tituloAnterior ? `renomeou de "${d.tituloAnterior}"` : 'editou este card';

		case 'card.apagado':
			return d.titulo ? `apagou "${d.titulo}"` : 'apagou este card';

		// O que pende do card passou a ter evento próprio, e todos são marcados
		// com o card a que pertencem — então caem aqui sozinhos, sem nada a mais.
		case 'etiqueta.aplicada':
			return alvo ? `marcou com ${alvo}` : 'marcou com uma etiqueta';
		case 'etiqueta.retirada':
			return alvo ? `tirou a etiqueta ${alvo}` : 'tirou uma etiqueta';
		case 'checklist.criada':
			return alvo ? `criou a checklist ${alvo}` : 'criou uma checklist';
		case 'checklist.alterada':
			return alvo ? `mudou a checklist ${alvo}` : 'mudou uma checklist';
		case 'checklist.apagada':
			return alvo ? `apagou a checklist ${alvo}` : 'apagou uma checklist';
		case 'item.criado':
			return alvo ? `acrescentou ${alvo}` : 'acrescentou um item';
		case 'item.alterado':
			return alvo ? `mexeu em ${alvo}` : 'mexeu num item';
		case 'item.apagado':
			return alvo ? `apagou ${alvo}` : 'apagou um item';
		case 'anexo.adicionado':
			return alvo ? `anexou ${alvo}` : 'anexou um arquivo';
		case 'anexo.removido':
			return alvo ? `removeu o anexo ${alvo}` : 'removeu um anexo';
		case 'responsavel.atribuido':
			return alvo ? `pôs ${alvo} como responsável` : 'atribuiu este card';
		case 'responsavel.removido':
			return alvo ? `tirou ${alvo} da responsabilidade` : 'tirou o responsável';

		case 'comentario.criado':
			return 'comentou';
		case 'comentario.editado':
			return 'editou um comentário';
		case 'comentario.apagado':
			return 'apagou um comentário';

		// Gravado antes de a conversa ter um tipo por operação.
		case 'comentario.alterado':
			return 'mexeu na conversa';

		default:
			return '';
	}
}

/**
 * fraseNoQuadro descreve a mesma entrada para a AUDITORIA do quadro, onde a
 * frase precisa dizer em qual card a coisa aconteceu.
 *
 * No histórico de um card o contexto é a própria tela ("moveu de A para B" —
 * moveu este card, o que está aberto). Numa lista que mistura o quadro inteiro,
 * a mesma frase fica sem sujeito: cinquenta linhas dizendo "moveu de A para B"
 * não deixam auditar nada.
 *
 * O título vem do payload do evento, e é o que o card tinha NA HORA. Um card
 * renomeado depois — ou apagado — continua identificável pela linha do log.
 */
export function fraseNoQuadro(a: Atividade): string {
	const d = a.dados ?? {};
	const card = d.titulo ? `"${d.titulo}"` : 'um card';
	const alvo = d.alvo ? `"${d.alvo}"` : '';
	const coluna = d.titulo ? `"${d.titulo}"` : 'uma coluna';
	const quem = d.nome || d.email || 'alguém';

	switch (a.tipo) {
		// --- card ---
		case 'card.criado':
			return d.coluna ? `criou ${card} em ${d.coluna}` : `criou ${card}`;
		case 'card.movido':
			if (d.deColuna && d.coluna && d.deColuna !== d.coluna) {
				return `moveu ${card} de ${d.deColuna} para ${d.coluna}`;
			}
			return d.coluna ? `reordenou ${card} em ${d.coluna}` : `moveu ${card}`;
		case 'card.alterado':
			return d.tituloAnterior ? `renomeou "${d.tituloAnterior}" para ${card}` : `editou ${card}`;
		case 'card.apagado':
			return `apagou ${card}`;

		// --- coluna ---
		case 'coluna.criada':
			return `criou a coluna ${coluna}`;
		case 'coluna.alterada':
			return d.tituloAnterior
				? `renomeou a coluna "${d.tituloAnterior}" para ${coluna}`
				: `mudou a coluna ${coluna}`;
		case 'coluna.apagada':
			return `apagou a coluna ${coluna}`;
		case 'coluna.movida':
			return `reordenou a coluna ${coluna}`;

		// --- quadro ---
		case 'quadro.renomeado':
			return d.tituloAnterior
				? `renomeou o quadro de "${d.tituloAnterior}" para "${d.titulo}"`
				: `renomeou o quadro para "${d.titulo}"`;
		case 'quadro.fundo':
			return d.fundo ? `mudou o fundo do quadro para ${d.fundo}` : 'mudou o fundo do quadro';

		// --- etiquetas ---
		case 'etiqueta.criada':
			return `criou a etiqueta "${d.nome}"`;
		case 'etiqueta.alterada':
			return d.nomeAnterior
				? `renomeou a etiqueta "${d.nomeAnterior}" para "${d.nome}"`
				: `mudou a etiqueta "${d.nome}"`;
		case 'etiqueta.apagada':
			return `apagou a etiqueta "${d.nome}"`;
		case 'etiqueta.aplicada':
			return alvo ? `marcou ${card} com ${alvo}` : `marcou ${card} com uma etiqueta`;
		case 'etiqueta.retirada':
			return alvo ? `tirou ${alvo} de ${card}` : `tirou uma etiqueta de ${card}`;

		// --- checklists ---
		case 'checklist.criada':
			return alvo ? `criou a checklist ${alvo} em ${card}` : `criou uma checklist em ${card}`;
		case 'checklist.alterada':
			return alvo ? `mudou a checklist ${alvo} em ${card}` : `mudou uma checklist em ${card}`;
		case 'checklist.apagada':
			return alvo ? `apagou a checklist ${alvo} de ${card}` : `apagou uma checklist de ${card}`;
		case 'item.criado':
			return alvo ? `acrescentou ${alvo} a ${card}` : `acrescentou um item a ${card}`;
		case 'item.alterado':
			return alvo ? `mexeu em ${alvo}, em ${card}` : `mexeu num item de ${card}`;
		case 'item.apagado':
			return alvo ? `apagou ${alvo} de ${card}` : `apagou um item de ${card}`;

		// --- anexos ---
		case 'anexo.adicionado':
			return alvo ? `anexou ${alvo} em ${card}` : `anexou algo em ${card}`;
		case 'anexo.removido':
			return alvo ? `removeu o anexo ${alvo} de ${card}` : `removeu um anexo de ${card}`;

		// --- responsáveis ---
		case 'responsavel.atribuido':
			return alvo ? `pôs ${alvo} como responsável por ${card}` : `atribuiu ${card} a alguém`;
		case 'responsavel.removido':
			return alvo
				? `tirou ${alvo} da responsabilidade de ${card}`
				: `tirou o responsável de ${card}`;

		// --- comentários ---
		case 'comentario.criado':
			return `comentou em ${card}`;
		case 'comentario.editado':
			return `editou um comentário em ${card}`;
		case 'comentario.apagado':
			return `apagou um comentário em ${card}`;

		// --- participação ---
		case 'membro.adicionado':
			return d.papel
				? `adicionou ${quem} ao quadro como ${d.papel}`
				: `adicionou ${quem} ao quadro`;
		case 'membro.entrou':
			return d.papel ? `entrou no quadro como ${d.papel}` : 'entrou no quadro';
		case 'membro.papel':
			return d.papelAnterior
				? `mudou ${quem} de ${d.papelAnterior} para ${d.papel}`
				: `mudou ${quem} para ${d.papel}`;
		case 'membro.removido':
			return `removeu ${quem} do quadro`;

		// Tipos que o log guarda de ANTES da separação por ação. Não há como
		// dizer o que mudou — o payload deles era vazio —, mas some-los seria
		// pior: a auditoria mostraria um buraco onde houve atividade.
		case 'quadro.alterado':
			return 'mexeu no quadro';
		case 'comentario.alterado':
			return `mexeu na conversa de ${card}`;
		case 'membros.alterados':
			return 'mexeu na participação do quadro';

		// Tipo desconhecido vira silêncio, e a tela omite a linha: um evento novo
		// que ainda não tem frase é melhor ausente do que como "undefined" no meio
		// da auditoria.
		default:
			return '';
	}
}

/** Data curta: num histórico o que importa é quando, não a precisão. */
export function quando(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleString('pt-BR', {
		day: '2-digit',
		month: 'short',
		hour: '2-digit',
		minute: '2-digit'
	});
}

/**
 * haQuanto devolve "agora", "há 5 min", "há 3 h", "há 2 d" — o tempo relativo
 * que cabe no rodapé de um card.
 *
 * Relativo, e não a data: no card o que importa é se aquilo foi AGORA HÁ POUCO
 * ou faz tempo, e "17 de ago., 14:32" obriga quem lê a fazer a conta. A data
 * exata continua disponível no `title` e na tela de auditoria.
 *
 * Acima de uma semana volta a ser data: "há 43 d" não diz nada a ninguém.
 */
export function haQuanto(iso: string, agora: Date = new Date()): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '';

	const segundos = Math.floor((agora.getTime() - d.getTime()) / 1000);
	// Relógio do cliente adiantado em relação ao servidor produz futuro por
	// alguns segundos. "em 3 s" seria absurdo num histórico — vira "agora".
	if (segundos < 60) return 'agora';
	if (segundos < 3600) return `há ${Math.floor(segundos / 60)} min`;
	if (segundos < 86400) return `há ${Math.floor(segundos / 3600)} h`;
	if (segundos < 7 * 86400) return `há ${Math.floor(segundos / 86400)} d`;
	return quando(iso);
}

/** Entradas que a tela sabe descrever — o resto é omitido. */
export function descritas(lista: Atividade[]): { atividade: Atividade; texto: string }[] {
	return lista
		.map((atividade) => ({ atividade, texto: frase(atividade) }))
		.filter((linha) => linha.texto !== '');
}
