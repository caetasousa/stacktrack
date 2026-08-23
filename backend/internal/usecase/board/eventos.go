package board

import (
	"context"

	"stacktrack/internal/domain/evento"
)

// Publicador é a porta de saída de eventos. Quem a implementa é o hub, em
// adapter/realtime — e é justamente por ser uma interface declarada AQUI, no
// pacote que a consome, que nenhum usecase precisa saber que WebSocket existe.
//
// Trocar WebSocket por SSE, ou por LISTEN/NOTIFY do Postgres quando houver mais
// de uma instância da API, é escrever outro adaptador. Nada deste pacote muda.
type Publicador interface {
	Publicar(evento.Evento)
}

// RegistroDeEventos é o log do quadro: grava o que aconteceu e devolve a
// posição na história.
//
// O seq é o que permite a um cliente que caiu perguntar "o que houve desde o
// 41?" ao voltar — sem ele, reconectar seria recomeçar do zero sem saber o que
// se perdeu.
type RegistroDeEventos interface {
	Registrar(ctx context.Context, e evento.Evento) (int64, error)
}

// Escrita são os repositórios ligados a uma transação em curso.
//
// Traz TODOS os que participam de mutação de quadro, e não só os três
// estruturais. A versão anterior deixava etiqueta, checklist, anexo,
// responsável e comentário de fora, com o argumento de que os eventos deles são
// um AVISO de "recarregue o quadro" e perder um não deixaria buraco
// perceptível.
//
// O argumento estava certo sobre o SINTOMA e errado sobre a garantia. O que
// acontecia não era só perder o aviso: a mudança era gravada numa transação e o
// evento noutra, então também existia o caminho inverso — evento gravado sem a
// mudança, quando a segunda escrita falhava depois da primeira. Um cliente que
// recebe "etiqueta aplicada" e recarrega para não encontrar etiqueta nenhuma
// não tem como se recuperar disso, porque o log diz que aconteceu. Agora os
// dois caem no mesmo commit, e o par (mudança, evento) vale ou não vale junto.
type Escrita struct {
	Cards        RepositorioCard
	Colunas      RepositorioColuna
	Boards       RepositorioBoard
	Usuarios     buscadorUsuario
	Membros      RepositorioMembro
	Convites     RepositorioConvite
	Etiquetas    RepositorioEtiqueta
	Checklists   RepositorioChecklist
	Anexos       RepositorioAnexo
	Responsaveis RepositorioResponsavel
	Comentarios  RepositorioComentario
	Publicacoes  RepositorioPublicacao
	// Exclusoes é o outbox das exclusões de arquivo. Participa da MESMA
	// transação da mutação que apaga: é o que torna a exclusão recuperável em
	// vez de irreversível.
	//
	// Pode vir nil nos testes de regra, que não têm banco — e aí o arquivo
	// simplesmente fica no disco, que é o resultado seguro.
	Exclusoes RepositorioExclusaoDeArquivo
}

// EscritaAtomica grava a mudança e o evento que a descreve na MESMA transação.
//
// É a porta do outbox transacional. Sem ela, o dado é gravado numa transação e
// o evento noutra, logo depois — e um processo que morra entre as duas deixa a
// mudança sem evento. O cliente que reconecta pede "o que houve desde o 41?",
// recebe do 42 em diante e nunca fica sabendo que houve algo no meio: a tela
// dele passa a discordar do banco em silêncio.
//
// Quem a implementa é adapter/repository.UnidadeDeTrabalho. Nada aqui sabe o
// que é uma transação de Postgres — só que existe um jeito de as duas escritas
// caírem ou valerem juntas.
type EscritaAtomica interface {
	// Escrever devolve o evento CARIMBADO — com o seq que o banco atribuiu e a
	// revisão que a transação reservou.
	//
	// Devolve o evento inteiro, e não só o seq, porque quem publica precisa dos
	// dois números. A versão anterior devolvia apenas o seq e recebia o evento
	// por VALOR: a revisão era gravada na cópia de dentro da transação e nunca
	// chegava de volta, então o log tinha revisão e o evento entregue ao vivo
	// saía com zero. O cliente via um evento sem posição, não conseguia
	// encaixá-lo na sequência e caía em reconciliação a cada mudança — com o
	// banco perfeitamente correto o tempo todo.
	Escrever(ctx context.Context, e evento.Evento, mudanca func(Escrita) error) (evento.Evento, error)
	// ExcluirQuadro executa a exclusão terminal sob o mesmo lock usado pelas
	// demais mutações. Ela é separada de Escrever porque apagar o quadro leva por
	// cascata o próprio log de eventos; inserir um evento depois do DELETE
	// violaria a chave estrangeira para boards.
	ExcluirQuadro(ctx context.Context, boardID string, mudanca func(Escrita) error) error
}

// eventos é embutido em cada usecase de escrita. Fica separado do usecase para
// que ligar a publicação seja uma linha por usecase, e não um parâmetro novo em
// sete construtores — e em todos os testes que os chamam.
type eventos struct {
	pub Publicador
	// efemero entrega sinais que não estão no log, direto ao hub. Ver
	// publicarEfemero.
	efemero Publicador
	log     RegistroDeEventos
	atomica EscritaAtomica
}

// ComPublicador liga a saída de eventos deste usecase. Sem ela o usecase segue
// funcionando e não publica nada, que é exatamente o que os testes querem: eles
// exercitam regra de negócio, não entrega.
func (e *eventos) ComPublicador(p Publicador) {
	e.pub = p
}

// ComPublicadorEfemero liga a entrega DIRETA ao hub, para os sinais que não
// pertencem ao log.
//
// Existe porque a entrega normal passou a ser feita pelo despachante, que lê o
// que está gravado. Um sinal que nunca é gravado precisa de outro caminho.
func (e *eventos) ComPublicadorEfemero(p Publicador) {
	e.efemero = p
}

// ComRegistro liga o log de eventos. Sem ele o usecase continua publicando ao
// vivo, só que sem história — que é o que os testes querem.
func (e *eventos) ComRegistro(r RegistroDeEventos) {
	e.log = r
}

// escrita monta os repositórios do caminho NÃO transacional — os mesmos de
// sempre, ligados direto ao pool. É o que escreverEPublicar usa quando não há
// EscritaAtomica ligada, que é o caso dos testes de regra de negócio.
//
// Cada usecase declara o que as SUAS mutações tocam. Um campo que fique nil
// aqui vira panic na primeira chamada que o use — e é assim que se quer:
// esquecer de declarar um repositório é um erro de programação, e um erro de
// programação precisa aparecer no primeiro teste, não como "o evento não
// chegou" em produção.
func (uc *CardUseCase) escrita() Escrita {
	return Escrita{
		Membros: uc.membros, Cards: uc.cards, Colunas: uc.colunas,
		Anexos: uc.anexos, Exclusoes: uc.outbox,
	}
}

func (uc *ColunaUseCase) escrita() Escrita {
	return Escrita{
		Membros: uc.membros, Colunas: uc.colunas,
		Anexos: uc.anexos, Exclusoes: uc.outbox,
	}
}

func (uc *QuadroUseCase) escrita() Escrita {
	return Escrita{
		Boards: uc.boards, Membros: uc.membros, Colunas: uc.colunas,
		Cards: uc.cards, Anexos: uc.anexos, Exclusoes: uc.outbox,
	}
}

// exclusoes é embutido nos três usecases que apagam algo com anexo pendurado.
//
// Entra por ComExclusoes, e não pelo construtor, pela mesma razão do publicador
// de eventos: sem ele o usecase continua funcionando — e o arquivo simplesmente
// fica no disco, que é o resultado SEGURO. Um outbox obrigatório no construtor
// obrigaria todo teste de regra a montar um.
type exclusoes struct {
	outbox RepositorioExclusaoDeArquivo
}

// ComExclusoes liga o outbox das exclusões de arquivo.
func (e *exclusoes) ComExclusoes(r RepositorioExclusaoDeArquivo) {
	e.outbox = r
}

func (uc *MembroUseCase) escrita() Escrita {
	return Escrita{
		Boards: uc.boards, Usuarios: uc.usuarios, Membros: uc.membros,
		Convites: uc.convites, Responsaveis: uc.responsaveis,
	}
}

func (uc *EtiquetaUseCase) escrita() Escrita {
	return Escrita{Membros: uc.membros, Etiquetas: uc.etiquetas}
}

func (uc *ChecklistUseCase) escrita() Escrita {
	return Escrita{Membros: uc.membros, Checklists: uc.checklists}
}

func (uc *AnexoUseCase) escrita() Escrita {
	return Escrita{Membros: uc.membros, Cards: uc.cards, Anexos: uc.anexos}
}

func (uc *ResponsavelUseCase) escrita() Escrita {
	return Escrita{Membros: uc.membros, Responsaveis: uc.responsaveis}
}

func (uc *ComentarioUseCase) escrita() Escrita {
	return Escrita{Membros: uc.membros, Comentarios: uc.comentarios}
}

func (uc *PublicacaoUseCase) escrita() Escrita {
	return Escrita{Membros: uc.membros, Publicacoes: uc.publicacoes}
}

// ComEscritaAtomica liga o outbox transacional. Sem ele, o usecase continua
// funcionando com a escrita e o registro em transações separadas — que é o que
// os testes de regra de negócio querem, já que não há banco nenhum ali.
func (e *eventos) ComEscritaAtomica(a EscritaAtomica) {
	e.atomica = a
}

// tituloDoCard resolve o título para o payload de um evento.
//
// A frase da auditoria diz "anexou contrato.pdf em X", e sem o título ela vira
// "anexou contrato.pdf em um card" — que é a metade inútil da informação, o
// mesmo defeito que a coluna de origem do movimento já tinha corrigido.
//
// O título é gravado no evento, e não resolvido na leitura: é o que o card
// tinha NA HORA, pela mesma razão de DadosDoCard guardar nomes. Falha vira
// string vazia, e a frase encolhe em vez de mentir.
func tituloDoCard(ctx context.Context, cards RepositorioCard, cardID string) string {
	c, err := cards.BuscarPorID(ctx, cardID)
	if err != nil || c == nil {
		return ""
	}
	return c.Titulo
}

// publicarEfemero entrega um sinal que, por definição, NÃO pertence ao log.
//
// A única mutação autorizada aqui é a exclusão terminal do quadro: depois do
// commit já não existe board ao qual persistir o evento (a FK e o CASCADE
// impedem), mas as abas ainda abertas precisam saber que devem sair. Todas as
// mutações não terminais continuam obrigatoriamente em escreverEPublicar.
//
// ⚠️ Vai por `efemero`, e NÃO pelo publicador comum. Desde que a entrega passou
// a ser feita pelo despachante — que ignora o evento recebido e entrega o que
// está GRAVADO —, mandar este sinal por lá seria mandá-lo para o nada: ele não
// está no log, e nunca estará. `efemero` fala direto com o hub.
//
// Sem `efemero` ligado, cai no publicador comum: é o que os testes de regra
// querem, onde o publicador é um espião em memória e não há despachante nenhum.
func (e *eventos) publicarEfemero(tipo evento.Tipo, boardID, autorID string, dados any) {
	ev := evento.Novo(tipo, boardID, autorID, dados)
	if e.efemero != nil {
		e.efemero.Publicar(ev)
		return
	}
	if e.pub != nil {
		e.pub.Publicar(ev)
	}
}

// escreverEPublicar grava a mudança e o evento na MESMA transação e, só depois
// do commit, entrega ao vivo.
//
// É o caminho das mudanças estruturais — card e coluna criados, movidos,
// alterados, apagados. A atomicidade importa porque um evento faltando aqui é
// INVISÍVEL para quem reconecta: ele pede o intervalo a partir do último seq
// que aplicou, recebe o que existe, e não tem como saber que houve uma mudança
// que nunca virou evento.
//
// A entrega ao vivo fica FORA da transação de propósito. Publicar antes do
// commit anunciaria uma mudança que o rollback ainda pode desfazer, e quem
// recebesse o evento recarregaria o quadro para encontrar o estado anterior.
//
// Sem EscritaAtomica ligada (o caso dos testes de regra), cai no caminho não
// transacional: a mudança roda contra os repositórios de sempre e o evento é
// registrado em seguida.
func (e *eventos) escreverEPublicar(
	ctx context.Context,
	tipo evento.Tipo, boardID, autorID string, dados any,
	padrao Escrita,
	mudanca func(Escrita) error,
) error {
	return e.escreverEPublicarNoCard(ctx, tipo, boardID, "", autorID, dados, padrao, mudanca)
}

// escreverEPublicarNoCard é o mesmo, marcando o card a que o evento pertence.
func (e *eventos) escreverEPublicarNoCard(
	ctx context.Context,
	tipo evento.Tipo, boardID, cardID, autorID string, dados any,
	padrao Escrita,
	mudanca func(Escrita) error,
) error {
	ev := evento.Novo(tipo, boardID, autorID, dados).NoCard(cardID)

	if e.atomica == nil {
		if err := mudanca(padrao); err != nil {
			return err
		}
		congelarDados(&ev)
		if e.log != nil {
			seq, err := e.log.Registrar(ctx, ev)
			if err != nil {
				return err
			}
			ev.Seq = seq
		}
	} else {
		// O evento volta CARIMBADO. Reatribuir `ev` inteiro, e não só o seq, é o
		// que faz a revisão chegar a quem publica: ela é reservada dentro da
		// transação, sobre a cópia que a unidade de trabalho recebeu.
		carimbado, err := e.atomica.Escrever(ctx, ev, mudanca)
		if err != nil {
			return err
		}
		ev = carimbado
		congelarDados(&ev)
	}

	if e.pub != nil {
		e.pub.Publicar(ev)
	}
	return nil
}

// congelarDados transforma os poucos payloads montados por ponteiro de volta
// em valor antes de entregá-los aos consumidores em memória. O ponteiro existe
// somente durante a UoW para o callback preencher nomes lidos sob o lock; no
// protocolo e nos testes, Dados continua tendo exatamente os tipos de antes.
func congelarDados(ev *evento.Evento) {
	switch dados := ev.Dados.(type) {
	case *DadosDoCard:
		ev.Dados = *dados
	case *DadosDaColuna:
		ev.Dados = *dados
	}
}

// Leitura são os repositórios ligados a uma transação de LEITURA em curso.
//
// É o mesmo conjunto de Escrita, e o alias é deliberado: as portas são as
// mesmas, e duplicar dez campos idênticos só para trocar o nome do tipo criaria
// dois lugares para esquecer de acrescentar o próximo repositório. O que muda é
// a transação por trás — REPEATABLE READ, READ ONLY —, e isso quem decide é o
// adaptador, não o formato do struct.
type Leitura = Escrita

// InstantaneoConsistente executa uma leitura sobre UM ÚNICO instantâneo do
// banco.
//
// O problema que ela resolve: montar o quadro custa dez consultas
// independentes — colunas, cards, etiquetas por card, progresso das checklists,
// contagem de anexos, responsáveis, comentários. Sob READ COMMITTED, que é o
// padrão do PostgreSQL, cada uma dessas consultas enxerga o banco no instante em
// que ELA rodou. Uma escrita que aconteça no meio da sequência é vista pelas
// consultas seguintes e não pelas anteriores.
//
// O resultado é um snapshot que nunca existiu: card presente na lista de cards e
// ausente da contagem de comentários, coluna que sumiu deixando cards órfãos na
// resposta. E o defeito é pior do que parece, porque o snapshot vem carimbado
// com uma revisão — o cliente aplica os eventos seguintes por cima de um estado
// incoerente e nunca descobre que partiu errado.
//
// REPEATABLE READ congela um instantâneo no primeiro comando e o mantém até o
// fim. READ ONLY declara a intenção e permite ao PostgreSQL recusar escrita
// acidental ali dentro.
//
// Sem a porta ligada, o usecase lê pelos repositórios de sempre — que é o que os
// testes de regra querem, já que não há banco nenhum ali.
type InstantaneoConsistente interface {
	Executar(ctx context.Context, leitura func(Leitura) error) error
}
