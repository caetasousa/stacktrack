package board

import (
	"context"
	"errors"
	"time"

	dboard "stacktrack/internal/domain/board"
	dconvite "stacktrack/internal/domain/convite"
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
	}
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
	// Convite e Token descrevem o convite recém-criado. O token em texto puro
	// aparece NESTA resposta e em nenhuma outra: o banco guarda só o hash,
	// então quem não copiar o link agora precisa gerar outro.
	Convite *dconvite.Convite
	Token   string
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
}

// Listar devolve quem participa do quadro. Qualquer membro pode ver — inclusive
// o leitor: saber com quem se divide um quadro é parte de participar dele.
func (uc *MembroUseCase) Listar(ctx context.Context, boardID, usuarioID string) ([]Participante, error) {
	if _, err := acesso(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.membros.Participantes(ctx, boardID)
}

// ListarConvites devolve os convites ainda pendentes. Só o dono: a lista diz
// para quem o quadro foi oferecido, o que não é da conta de quem só participa.
func (uc *MembroUseCase) ListarConvites(ctx context.Context, boardID, usuarioID string) ([]dconvite.Convite, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	return uc.convites.ListarPendentes(ctx, boardID)
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

	// A checagem de duplicidade e o INSERT ficam DENTRO da unidade de trabalho,
	// sob o lock do quadro. Fora dela, entre "não há convite pendente" e o
	// INSERT cabia outro convite para o mesmo email — e quem perdesse a corrida
	// levava a violação do índice único como erro cru.
	agora := time.Now()
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
			}
			return e.Convites.Salvar(ctx, c)
		}); err != nil {
		return nil, err
	}
	return &ResultadoConvite{Convite: c, Token: t}, nil
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
			return e.Convites.Revogar(ctx, conviteID, agora)
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
		existenteAntes, err := uc.membros.Buscar(ctx, c.BoardID, usuarioID)
		if err != nil {
			return nil, "", err
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
