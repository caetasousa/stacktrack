package board

import (
	"context"
	"errors"
	"time"

	dboard "stacktrack/internal/domain/board"
	dconvite "stacktrack/internal/domain/convite"
	demail "stacktrack/internal/domain/email"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"
	dusuario "stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"

	"github.com/google/uuid"
)

// errParticipacaoMudou pede que Aceitar refaça a escolha do evento depois de
// adquirir o lock. Não atravessa a borda: é apenas a detecção de que a leitura
// otimista anterior à transação ficou velha.
var errParticipacaoMudou = errors.New("a participação mudou enquanto o convite era aceito")

// MembroUseCase reúne quem participa do quadro: listar, convidar, trocar papel
// e remover.
type MembroUseCase struct {
	eventos
	membros      RepositorioMembro
	convites     RepositorioConvite
	usuarios     buscadorUsuario
	boards       RepositorioBoard
	responsaveis RepositorioResponsavel
	// correio é a entrega por email; nil significa envio desligado, e o
	// convite nasce só com o link manual. Ver ComCorreio.
	correio *correioDeConvite
	// intervaloDeReenvio é o mínimo entre dois emails do mesmo convite.
	intervaloDeReenvio time.Duration
}

// NovoMembroUseCase cria uma instância de MembroUseCase com as dependências injetadas.
func NovoMembroUseCase(
	membros RepositorioMembro,
	convites RepositorioConvite,
	usuarios buscadorUsuario,
	boards RepositorioBoard,
	responsaveis RepositorioResponsavel,
) *MembroUseCase {
	return &MembroUseCase{
		membros: membros, convites: convites, usuarios: usuarios,
		boards: boards, responsaveis: responsaveis,
		intervaloDeReenvio: dconvite.IntervaloEntreReenvios,
	}
}

// ComIntervaloDeReenvio troca o mínimo entre dois emails do mesmo convite. Zero
// desliga a espera — o que a suíte E2E precisa para reenviar sem esperar dois
// minutos; em produção, config.Validar exige um intervalo.
func (uc *MembroUseCase) ComIntervaloDeReenvio(intervalo time.Duration) {
	uc.intervaloDeReenvio = intervalo
}

// ResultadoConvite é o que sai de Convidar: sempre um convite pendente e o
// link que o entrega.
//
// Houve um segundo caminho aqui — quem já tinha conta virava membro na hora,
// sem token. Ele foi REMOVIDO: conhecer o email de alguém passava a bastar
// para pôr essa pessoa dentro de um quadro, sem que ela concordasse e sem que
// ficasse sabendo. Agora todo acesso nasce de um token que a pessoa certa
// precisa apresentar estando logada na conta daquele email.
type ResultadoConvite struct {
	// Convite e Token descrevem o convite recém-criado (ou renovado). O token
	// em texto puro aparece NESTA resposta e no email, e em nenhum outro lugar:
	// o banco guarda só o hash — quem perder o link reenvia, e ganha outro.
	Convite *dconvite.Convite
	Token   string
	// Entrega diz se o convite vai por email ou se o link é tudo o que há.
	Entrega Entrega
}

// DetalheConvite é o que um convite mostra antes de ser aceito, para quem tem o
// link. Não exige sessão: quem foi convidado normalmente ainda nem tem conta.
type DetalheConvite struct {
	TituloQuadro string
	// EmailMascarado é o endereço na forma "a***@exemplo.com". A rota é
	// pública: quem tem o link não é necessariamente quem foi convidado, e o
	// endereço inteiro ali transformaria um link vazado em vazamento de email.
	// A máscara ainda deixa a pessoa certa reconhecer o próprio endereço, que é
	// a dúvida real de quem chega nessa tela logado na conta errada.
	EmailMascarado string
	Papel          membro.Papel
	ConvidadoPor   string
	// ExpiraEm é até quando o convite pode ser aceito: quem chega pelo email
	// precisa saber quanto tempo tem para decidir.
	ExpiraEm time.Time
}

// Listar devolve quem participa do quadro. Qualquer membro pode ver — inclusive
// o leitor: saber com quem se divide um quadro é parte de participar dele.
func (uc *MembroUseCase) Listar(ctx context.Context, boardID, usuarioID string) ([]Participante, error) {
	if _, err := acesso(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.membros.Participantes(ctx, boardID)
}

// ListarConvites devolve os convites ainda pendentes, cada um com o último
// envio por email. Só o dono: a lista diz para quem o quadro foi oferecido, o
// que não é da conta de quem só participa.
func (uc *MembroUseCase) ListarConvites(ctx context.Context, boardID, usuarioID string) ([]ConviteComEntrega, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	convites, err := uc.convites.ListarPendentes(ctx, boardID)
	if err != nil {
		return nil, err
	}

	lista := make([]ConviteComEntrega, 0, len(convites))
	ultimos := map[string]demail.Envio{}
	if uc.correio != nil && len(convites) > 0 {
		ids := make([]string, 0, len(convites))
		for _, c := range convites {
			ids = append(ids, c.ID)
		}
		if ultimos, err = uc.correio.fila.UltimosDosConvites(ctx, ids); err != nil {
			return nil, err
		}
	}
	for _, c := range convites {
		item := ConviteComEntrega{Convite: c}
		if envio, ok := ultimos[c.ID]; ok {
			item.Entrega = &envio
		}
		lista = append(lista, item)
	}
	return lista, nil
}

// Convidar cria um convite pendente para o email e devolve o token que vira
// link. Vale igual para quem já tem conta e para quem não tem: o convite é a
// ÚNICA porta de entrada de um quadro.
//
// Antes havia um atalho — email de conta existente virava participação
// imediata. Ele saiu porque dava a quem conhece o email de alguém o poder de
// pôr essa pessoa num quadro sem consentimento nem aviso, e porque tornava a
// participação um efeito de conhecer um endereço, não de provar ser dono dele.
//
// Retorna dconvite.ErrJaEMembro quando a pessoa já participa,
// dconvite.ErrJaConvidado quando já há convite pendente para o email, e
// dconvite.ErrNaoConvidaODono quando alguém tenta convidar a si mesmo.
func (uc *MembroUseCase) Convidar(ctx context.Context, boardID, usuarioID, email string, papel membro.Papel) (*ResultadoConvite, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	if !membro.PapelValido(papel) {
		return nil, membro.ErrPapelInvalido
	}

	email = dusuario.NormalizarEmail(email)
	if email == "" {
		return nil, dconvite.ErrEmailObrigatorio
	}

	quemConvida, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if quemConvida != nil && quemConvida.Email == email {
		return nil, dconvite.ErrNaoConvidaODono
	}

	// A conta é procurada só para responder "essa pessoa JÁ participa?" — não
	// para dar acesso a ela. Quem ainda não tem conta simplesmente não tem
	// participação, e cai no mesmo caminho.
	convidado, err := uc.usuarios.BuscarPorEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if convidado != nil {
		existente, err := uc.membros.Buscar(ctx, boardID, convidado.ID)
		if err != nil {
			return nil, err
		}
		if existente != nil {
			return nil, dconvite.ErrJaEMembro
		}
	}

	t, err := token.Gerar()
	if err != nil {
		return nil, err
	}
	c, err := dconvite.Novo(uuid.NewString(), boardID, email, papel, token.Hash(t), usuarioID)
	if err != nil {
		return nil, err
	}

	// O email é redigido ANTES da transação: redigir não depende de lock, e a
	// transação do quadro precisa ser curta. Ele só entra na fila lá dentro,
	// junto com o convite.
	agora := time.Now()
	var mensagem *emailDoConvite
	if uc.correio != nil {
		if mensagem, err = uc.redigirConvite(ctx, c, t, usuarioID, agora); err != nil {
			return nil, err
		}
	}

	// A checagem de duplicidade e o INSERT ficam DENTRO da unidade de trabalho,
	// sob o lock do quadro. Fora dela, entre "não há convite pendente" e o
	// INSERT cabia outro convite para o mesmo email — e quem perdesse a corrida
	// levava a violação do índice único como erro cru.
	entrega := EntregaManual
	if err := uc.escreverEPublicar(ctx, evento.ConviteCriado, boardID, usuarioID,
		DadosDoMembro{Email: dusuario.MascararEmail(email), Papel: string(papel)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			// A pessoa pode ter criado a conta e aceitado outro convite enquanto
			// esta requisição esperava o lock. Por isso até a resolução do email é
			// refeita aqui; usar apenas `convidado`, lido antes, deixaria passar o
			// caso em que a conta ainda não existia naquela primeira consulta.
			// Usa o repositório ligado à PRÓPRIA transação. Consultar `uc.usuarios`
			// aqui adquiriria outra conexão do pool enquanto esta já segura uma:
			// com o pool cheio de convites, todas as transações poderiam esperar
			// uma conexão adicional que nenhuma delas liberaria até terminar.
			convidadoSobLock, err := e.Usuarios.BuscarPorEmail(ctx, email)
			if err != nil {
				return err
			}
			if convidadoSobLock != nil {
				existente, err := e.Membros.Buscar(ctx, boardID, convidadoSobLock.ID)
				if err != nil {
					return err
				}
				if existente != nil {
					return dconvite.ErrJaEMembro
				}
			}

			pendente, err := e.Convites.BuscarPendentePorEmail(ctx, boardID, email)
			if err != nil {
				return err
			}
			if pendente != nil {
				if pendente.Pendente(agora) {
					return dconvite.ErrJaConvidado
				}
				// Vencido: ele ocupa a vaga do índice de pendência sem valer
				// mais nada. Revogar é o que a libera — e revogar, e não apagar,
				// para a resposta de "quem convidou quem, e quando" sobreviver.
				//
				// O vencimento não cabe no predicado do índice (comparar com
				// now() não é imutável, e o PostgreSQL recusa), então é aqui,
				// sob o lock, que ele vira um fato gravado.
				if err := e.Convites.Revogar(ctx, pendente.ID, agora); err != nil {
					return err
				}
				if err := cancelarEmailsDoConvite(ctx, e, pendente.ID, agora); err != nil {
					return err
				}
			}

			// O teto do quadro é contado aqui, sob o lock e depois de o vencido
			// ter liberado a vaga: contado antes, dois convites simultâneos
			// ocupariam juntos o último lugar.
			pendentes, err := e.Convites.ContarPendentes(ctx, boardID)
			if err != nil {
				return err
			}
			if pendentes >= dconvite.MaximoDePendentesPorQuadro {
				return dconvite.ErrPendentesDemais
			}
			if err := e.Convites.Salvar(ctx, c); err != nil {
				return err
			}
			entrega, err = enfileirarNaTransacao(ctx, e, mensagem, usuarioID, agora)
			return err
		}); err != nil {
		return nil, err
	}
	uc.avisarCorreio(entrega)
	return &ResultadoConvite{Convite: c, Token: t, Entrega: entrega}, nil
}

// ReenviarConvite dá ao convite um link novo, com a validade inteira de novo, e
// o manda por email quando o envio está ligado. O link anterior — inclusive o
// do email anterior — deixa de valer no mesmo commit.
//
// Vale também para o convite vencido: reenviar é o caminho natural de quem
// descobre que o convite expirou sem ser visto.
//
// Retorna dconvite.ErrInvalido para convite desconhecido, de outro quadro, já
// aceito ou revogado, e para quem não administra o quadro — os casos não se
// distinguem, como em RevogarConvite. Retorna dconvite.ErrReenvioCedoDemais
// quando o último email do convite saiu há menos do intervalo mínimo.
func (uc *MembroUseCase) ReenviarConvite(ctx context.Context, boardID, conviteID, usuarioID string) (*ResultadoConvite, error) {
	c, err := uc.convites.BuscarPorID(ctx, conviteID)
	if err != nil {
		return nil, err
	}
	if c == nil || c.BoardID != boardID {
		return nil, dconvite.ErrInvalido
	}
	if _, err := acessoDeAdministracao(ctx, uc.membros, c.BoardID, usuarioID); err != nil {
		return nil, dconvite.ErrInvalido
	}

	t, err := token.Gerar()
	if err != nil {
		return nil, err
	}
	agora := time.Now()
	if err := c.Renovar(token.Hash(t), agora); err != nil {
		return nil, dconvite.ErrInvalido
	}

	var mensagem *emailDoConvite
	if uc.correio != nil {
		if mensagem, err = uc.redigirConvite(ctx, c, t, usuarioID, agora); err != nil {
			return nil, err
		}
	}

	entrega := EntregaManual
	err = uc.escreverEPublicar(ctx, evento.ConviteReenviado, c.BoardID, usuarioID,
		DadosDoMembro{Email: dusuario.MascararEmail(c.Email), Papel: string(c.Papel)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, c.BoardID, usuarioID); err != nil {
				return dconvite.ErrInvalido
			}
			// O intervalo é conferido sob o lock do quadro: duas abas
			// reenviando juntas passariam as duas por uma leitura feita antes,
			// e cada uma trocaria o link da outra.
			if e.Emails != nil && uc.intervaloDeReenvio > 0 {
				anteriores, err := e.Emails.DoConvite(ctx, c.ID)
				if err != nil {
					return err
				}
				if len(anteriores) > 0 && agora.Sub(anteriores[0].CriadoEm) < uc.intervaloDeReenvio {
					return dconvite.ErrReenvioCedoDemais
				}
			}
			if err := e.Convites.Renovar(ctx, c.ID, c.TokenHash, c.ExpiraEm); err != nil {
				return err
			}
			// O email anterior que ainda não saiu levaria o link que acabou de
			// morrer.
			if err := cancelarEmailsDoConvite(ctx, e, c.ID, agora); err != nil {
				return err
			}
			var err error
			entrega, err = enfileirarNaTransacao(ctx, e, mensagem, usuarioID, agora)
			return err
		})
	if errors.Is(err, dconvite.ErrJaResolvido) {
		return nil, dconvite.ErrInvalido
	}
	if err != nil {
		return nil, err
	}
	uc.avisarCorreio(entrega)
	return &ResultadoConvite{Convite: c, Token: t, Entrega: entrega}, nil
}

// AlterarPapel troca o papel de quem participa. Só o dono, e nunca deixando o
// quadro sem dono nenhum.
func (uc *MembroUseCase) AlterarPapel(ctx context.Context, boardID, usuarioID, alvoID string, papel membro.Papel) (*Participante, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}

	todos, err := uc.membros.Todos(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if err := membro.ValidarTrocaDePapel(todos, alvoID, papel); err != nil {
		return nil, err
	}

	vinculo, err := uc.membros.Buscar(ctx, boardID, alvoID)
	if err != nil {
		return nil, err
	}
	if vinculo == nil {
		return nil, membro.ErrNaoEMembro
	}
	// Guardado ANTES da troca: depois dela o vínculo só sabe o papel de agora, e
	// "promoveu de leitor para editor" perderia a metade que explica a mudança.
	papelAnterior := vinculo.Papel
	if err := vinculo.DefinirPapel(papel); err != nil {
		return nil, err
	}

	u, err := uc.usuarios.BuscarPorID(ctx, alvoID)
	if err != nil {
		return nil, err
	}
	p := &Participante{UsuarioID: alvoID, Papel: papel, CriadoEm: vinculo.CriadoEm}
	if u != nil {
		p.Nome, p.Email = u.Nome, u.Email
	}

	// A regra do último dono é RECONFERIDA dentro da transação, sob o lock do
	// quadro. A checagem acima roda contra uma leitura que já pode estar velha:
	// dois donos rebaixando um ao outro ao mesmo tempo passavam os dois pela
	// validação, gravavam os dois, e o quadro ficava sem dono nenhum — órfão,
	// sem ninguém que pudesse convidar ou apagá-lo.
	//
	// Trocar o papel de alguém muda o que ELA pode fazer, e a tela dela precisa
	// saber na hora: sem este aviso, quem foi rebaixado a leitor continuava
	// vendo os botões de edição até dar F5 — e levava 403 ao usar qualquer um.
	if err := uc.escreverEPublicar(ctx, evento.MembroPapelAlterado, boardID, usuarioID,
		DadosDoMembro{Nome: p.Nome, Email: p.Email, Papel: string(papel), PapelAnterior: string(papelAnterior)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			sobLock, err := e.Membros.Todos(ctx, boardID)
			if err != nil {
				return err
			}
			if err := membro.ValidarTrocaDePapel(sobLock, alvoID, papel); err != nil {
				return err
			}
			return e.Membros.Atualizar(ctx, vinculo)
		}); err != nil {
		return nil, err
	}
	return p, nil
}

// Remover tira alguém do quadro. Só o dono, e nunca o último dono — um quadro
// sem dono fica órfão, sem ninguém que possa convidar ou apagá-lo.
func (uc *MembroUseCase) Remover(ctx context.Context, boardID, usuarioID, alvoID string) error {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return err
	}

	todos, err := uc.membros.Todos(ctx, boardID)
	if err != nil {
		return err
	}
	if err := membro.ValidarRemocao(todos, alvoID); err != nil {
		return err
	}

	// Quem está saindo é resolvido ANTES da remoção: depois dela não há vínculo
	// nem por onde descobrir de quem se tratava, e o evento diria só que "alguém
	// saiu".
	nomeDoAlvo, emailDoAlvo := "", ""
	if u, err := uc.usuarios.BuscarPorID(ctx, alvoID); err == nil && u != nil {
		nomeDoAlvo, emailDoAlvo = u.Nome, u.Email
	}

	// As três escritas — atribuições, vínculo e evento — caem no MESMO commit.
	//
	// Antes elas eram sequenciais, e a ordem era a única defesa: as atribuições
	// saíam primeiro para que uma falha no meio deixasse "vínculo sem
	// atribuição" (que a próxima remoção conserta) em vez de "atribuição sem
	// vínculo" (que deixa o nome de quem não tem mais acesso pendurado nos
	// cards). Era o melhor arranjo possível sem transação; com ela, nenhum dos
	// dois estados parciais existe.
	//
	// A regra do último dono é reconferida sob o lock, pela mesma razão de
	// AlterarPapel: duas remoções simultâneas passavam as duas pela validação
	// feita contra leituras independentes.
	if err := uc.escreverEPublicar(ctx, evento.MembroRemovido, boardID, usuarioID,
		DadosDoMembro{Nome: nomeDoAlvo, Email: emailDoAlvo},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			sobLock, err := e.Membros.Todos(ctx, boardID)
			if err != nil {
				return err
			}
			if err := membro.ValidarRemocao(sobLock, alvoID); err != nil {
				return err
			}
			if err := e.Responsaveis.RemoverDoBoard(ctx, boardID, alvoID); err != nil {
				return err
			}
			return e.Membros.Remover(ctx, boardID, alvoID)
		}); err != nil {
		return err
	}
	return nil
}

// RevogarConvite invalida o link já entregue, marcando o convite como revogado.
//
// Marca, e não apaga: o DELETE anterior levava junto a resposta para "quem
// convidou quem, e quando", e tornava indistinguível "revoguei agora" de
// "nunca existiu".
//
// Revogar o que já foi aceito ou revogado devolve dconvite.ErrInvalido — o
// mesmo erro de um convite desconhecido. Quem administra não precisa saber em
// que microssegundo a corrida foi decidida; precisa saber que o link não vale
// mais, e ele não vale.
func (uc *MembroUseCase) RevogarConvite(ctx context.Context, conviteID, usuarioID string) error {
	c, err := uc.convites.BuscarPorID(ctx, conviteID)
	if err != nil {
		return err
	}
	if c == nil {
		return dconvite.ErrInvalido
	}
	if _, err := acessoDeAdministracao(ctx, uc.membros, c.BoardID, usuarioID); err != nil {
		// Quem não administra o quadro não fica sabendo que o convite existe.
		return dconvite.ErrInvalido
	}

	agora := time.Now()
	err = uc.escreverEPublicar(ctx, evento.ConviteRevogado, c.BoardID, usuarioID,
		DadosDoMembro{Email: dusuario.MascararEmail(c.Email), Papel: string(c.Papel)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, c.BoardID, usuarioID); err != nil {
				return dconvite.ErrInvalido
			}
			if err := e.Convites.Revogar(ctx, conviteID, agora); err != nil {
				return err
			}
			// O email que ainda não saiu levaria um link que acabou de morrer.
			return cancelarEmailsDoConvite(ctx, e, conviteID, agora)
		})
	if errors.Is(err, dconvite.ErrJaResolvido) {
		return dconvite.ErrInvalido
	}
	return err
}

// DetalharConvite descreve um convite a partir do token, SEM exigir sessão —
// quem foi convidado costuma ainda não ter conta, e precisa ver do que se trata
// antes de decidir criar uma.
//
// O token é a credencial: quem o tem pode ver o título do quadro e para qual
// email o convite foi feito. Quem não tem recebe o mesmo ErrInvalido de um
// convite vencido, sem distinção.
func (uc *MembroUseCase) DetalharConvite(ctx context.Context, tokenPuro string) (*DetalheConvite, error) {
	c, err := uc.convitePendente(ctx, tokenPuro)
	if err != nil {
		return nil, err
	}

	b, err := uc.boards.BuscarPorID(ctx, c.BoardID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, dconvite.ErrInvalido
	}

	detalhe := &DetalheConvite{
		TituloQuadro:   b.Titulo,
		EmailMascarado: dusuario.MascararEmail(c.Email),
		Papel:          c.Papel,
		ExpiraEm:       c.ExpiraEm,
	}
	if autor, err := uc.usuarios.BuscarPorID(ctx, c.CriadoPor); err == nil && autor != nil {
		detalhe.ConvidadoPor = autor.Nome
	}
	return detalhe, nil
}

// Aceitar transforma o convite em vínculo, devolvendo o quadro e o papel com
// que a pessoa entrou. Exige que a conta autenticada tenha o email para o qual
// o convite foi feito: sem isso, o link vazado colocaria qualquer pessoa dentro
// do quadro.
// Aceitar publica MembroEntrou para que quem já está no quadro veja o
// recém-chegado sem recarregar.
func (uc *MembroUseCase) Aceitar(ctx context.Context, tokenPuro, usuarioID string) (*dboard.Board, membro.Papel, error) {
	c, err := uc.convitePendente(ctx, tokenPuro)
	if err != nil {
		return nil, "", err
	}
	return uc.aceitar(ctx, c, usuarioID)
}

// ConviteRecebido é um convite em aberto para o email da conta, como o painel
// o mostra em "Convites para você".
type ConviteRecebido struct {
	ID           string
	TituloQuadro string
	ConvidadoPor string
	Papel        membro.Papel
	ExpiraEm     time.Time
}

// ListarRecebidos devolve os convites em aberto e válidos para o email da conta.
//
// Só para conta confirmada (dconvite.ErrEmailNaoConfirmado nas outras): o
// convite no painel dispensa o link, e sem o link o que prova que a pessoa é
// dona daquele email é a confirmação. Sem ela, cadastrar o email de alguém
// bastaria para ver os convites dele.
func (uc *MembroUseCase) ListarRecebidos(ctx context.Context, usuarioID string) ([]ConviteRecebido, error) {
	u, err := uc.contaConfirmada(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	convites, err := uc.convites.PendentesDoEmail(ctx, u.Email, time.Now())
	if err != nil {
		return nil, err
	}
	recebidos := make([]ConviteRecebido, 0, len(convites))
	for _, c := range convites {
		b, err := uc.boards.BuscarPorID(ctx, c.BoardID)
		if err != nil {
			return nil, err
		}
		if b == nil {
			continue
		}
		r := ConviteRecebido{ID: c.ID, TituloQuadro: b.Titulo, Papel: c.Papel, ExpiraEm: c.ExpiraEm}
		if autor, err := uc.usuarios.BuscarPorID(ctx, c.CriadoPor); err == nil && autor != nil {
			r.ConvidadoPor = autor.Nome
		}
		recebidos = append(recebidos, r)
	}
	return recebidos, nil
}

// AceitarRecebido aceita, pelo painel, um convite para o email da conta. É o
// mesmo aceite do link — mesma transação, mesmo evento —, com a confirmação de
// email no lugar do token. Convite de outro email, vencido ou já resolvido
// responde dconvite.ErrInvalido: saber o id de um convite não vale nada.
func (uc *MembroUseCase) AceitarRecebido(ctx context.Context, conviteID, usuarioID string) (*dboard.Board, membro.Papel, error) {
	c, err := uc.recebidoPendente(ctx, conviteID, usuarioID)
	if err != nil {
		return nil, "", err
	}
	return uc.aceitar(ctx, c, usuarioID)
}

// RecusarRecebido é o "não" de quem foi convidado: encerra o convite (o link
// do email morre junto), tira da fila o email que ainda não saiu e registra
// convite.recusado no histórico do quadro — é assim que quem convidou fica
// sabendo. A vaga é liberada: o dono pode convidar de novo.
func (uc *MembroUseCase) RecusarRecebido(ctx context.Context, conviteID, usuarioID string) error {
	c, err := uc.recebidoPendente(ctx, conviteID, usuarioID)
	if err != nil {
		return err
	}
	agora := time.Now()
	err = uc.escreverEPublicar(ctx, evento.ConviteRecusado, c.BoardID, usuarioID,
		DadosDoMembro{Email: dusuario.MascararEmail(c.Email), Papel: string(c.Papel)},
		uc.escrita(), func(e Escrita) error {
			if err := e.Convites.Recusar(ctx, c.ID, agora); err != nil {
				return err
			}
			return cancelarEmailsDoConvite(ctx, e, c.ID, agora)
		})
	if errors.Is(err, dconvite.ErrJaResolvido) {
		return dconvite.ErrInvalido
	}
	return err
}

// contaConfirmada busca a conta e exige o email confirmado.
func (uc *MembroUseCase) contaConfirmada(ctx context.Context, usuarioID string) (*dusuario.Usuario, error) {
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.EmailConfirmado() {
		return nil, dconvite.ErrEmailNaoConfirmado
	}
	return u, nil
}

// recebidoPendente resolve um convite do painel: conta confirmada, convite em
// aberto e para o email dela. Todo o resto é dconvite.ErrInvalido.
func (uc *MembroUseCase) recebidoPendente(ctx context.Context, conviteID, usuarioID string) (*dconvite.Convite, error) {
	u, err := uc.contaConfirmada(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	c, err := uc.convites.BuscarPorID(ctx, conviteID)
	if err != nil {
		return nil, err
	}
	if c == nil || !c.Pendente(time.Now()) ||
		dusuario.NormalizarEmail(c.Email) != dusuario.NormalizarEmail(u.Email) {
		return nil, dconvite.ErrInvalido
	}
	return c, nil
}

// aceitar é o aceite em si, depois de o convite ter sido resolvido — pelo
// token do link ou pelo id no painel.
func (uc *MembroUseCase) aceitar(ctx context.Context, c *dconvite.Convite, usuarioID string) (*dboard.Board, membro.Papel, error) {
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, "", err
	}
	if u == nil {
		return nil, "", dconvite.ErrInvalido
	}
	// A comparação é entre emails NORMALIZADOS dos dois lados. Ambos já são
	// gravados assim, e normalizar de novo aqui é barato: é a checagem que
	// impede o link vazado de valer para qualquer conta, e ela não pode
	// depender de nenhuma linha ter sido gravada pela régua certa.
	if dusuario.NormalizarEmail(u.Email) != dusuario.NormalizarEmail(c.Email) {
		return nil, "", dconvite.ErrOutroDestinatario
	}

	b, err := uc.boards.BuscarPorID(ctx, c.BoardID)
	if err != nil {
		return nil, "", err
	}
	if b == nil {
		return nil, "", dconvite.ErrInvalido
	}

	// O consumo do convite e a criação do vínculo caem no MESMO commit, sob o
	// lock do quadro.
	//
	// Sem isso havia duas corridas. A primeira: duas abas clicando no link ao
	// mesmo tempo liam "pendente" as duas, criavam vínculo as duas e gravavam a
	// aceitação as duas — dois eventos "entrou no quadro" para a mesma pessoa.
	// A segunda: o vínculo era gravado numa transação e a aceitação noutra, de
	// modo que uma falha no meio deixava a pessoa DENTRO do quadro com o
	// convite ainda pendente — um link que continuava valendo depois de já ter
	// sido usado.
	//
	// Quem perde a corrida recebe convite.ErrJaResolvido do UPDATE condicional,
	// e a transação inteira desfaz: nenhum vínculo duplicado, nenhum evento a
	// mais.
	agora := time.Now()
	papelFinal := c.Papel
	// O tipo do evento depende de a aceitação realmente criar participação.
	// Essa resposta só é confiável sob o lock, mas o evento é montado antes de
	// a unidade de trabalho entrar. Fazemos uma leitura otimista e, caso ela não
	// confira com a leitura transacional, abortamos sem efeitos e repetimos.
	for tentativa := 0; tentativa < 2; tentativa++ {
		// `errBusca`, e não `err`: um `:=` com `err` aqui criaria uma variável
		// nova dentro do laço, o resultado da transação iria para ela, e o `err`
		// conferido depois do laço continuaria nil — o aceite que perdeu a
		// corrida respondia sucesso sem ter feito nada.
		existenteAntes, errBusca := uc.membros.Buscar(ctx, c.BoardID, usuarioID)
		if errBusca != nil {
			return nil, "", errBusca
		}
		jaEraMembro := existenteAntes != nil
		tipo := evento.MembroEntrou
		if jaEraMembro {
			// O convite ficou redundante. Revogá-lo é a descrição verdadeira: a
			// pessoa não entrou agora, e publicar MembroEntrou faria a auditoria
			// afirmar uma mudança que não ocorreu.
			tipo = evento.ConviteRevogado
			papelFinal = existenteAntes.Papel
		}

		err = uc.escreverEPublicar(ctx, tipo, c.BoardID, usuarioID,
			// O email vai MASCARADO no payload: o evento é lido por todo mundo que
			// participa do quadro, e o endereço completo de quem entrou não é
			// informação que a auditoria precise carregar.
			DadosDoMembro{Email: dusuario.MascararEmail(c.Email), Papel: string(papelFinal)},
			uc.escrita(), func(e Escrita) error {
				existente, err := e.Membros.Buscar(ctx, c.BoardID, usuarioID)
				if err != nil {
					return err
				}
				if (existente != nil) != jaEraMembro ||
					(existente != nil && existenteAntes != nil && existente.Papel != existenteAntes.Papel) {
					return errParticipacaoMudou
				}

				// Aceito pelo link manual antes de o email sair: o email ficaria
				// sem propósito, chegando depois com um link já usado.
				if err := cancelarEmailsDoConvite(ctx, e, c.ID, agora); err != nil {
					return err
				}
				if existente != nil {
					papelFinal = existente.Papel
					return e.Convites.Revogar(ctx, c.ID, agora)
				}
				if err := e.Convites.Aceitar(ctx, c.ID, agora); err != nil {
					return err
				}
				vinculo, err := membro.Novo(c.BoardID, usuarioID, c.Papel)
				if err != nil {
					return err
				}
				return e.Membros.Salvar(ctx, vinculo)
			})
		if !errors.Is(err, errParticipacaoMudou) {
			break
		}
	}
	if errors.Is(err, dconvite.ErrJaResolvido) {
		// Outra requisição consumiu o convite. Para quem chamou, o link não
		// vale mais — a mesma resposta de um token vencido.
		return nil, "", dconvite.ErrInvalido
	}
	if err != nil {
		return nil, "", err
	}
	return b, papelFinal, nil
}

// convitePendente resolve o token e devolve o convite só se ele ainda vale.
// Token desconhecido, vencido e já aceito respondem o mesmo erro — distinguir
// os casos ajudaria quem está testando links.
func (uc *MembroUseCase) convitePendente(ctx context.Context, tokenPuro string) (*dconvite.Convite, error) {
	c, err := uc.convites.BuscarPorTokenHash(ctx, token.Hash(tokenPuro))
	if err != nil {
		return nil, err
	}
	if c == nil || !c.Pendente(time.Now()) {
		return nil, dconvite.ErrInvalido
	}
	return c, nil
}
