package board

import (
	"context"
	"time"

	dboard "stacktrack/internal/domain/board"
	dconvite "stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"
	dusuario "stacktrack/internal/domain/usuario"
	"stacktrack/internal/pkg/token"

	"github.com/google/uuid"
)

// MembroUseCase reúne quem participa do quadro: listar, convidar, trocar papel
// e remover.
type MembroUseCase struct {
	eventos
	membros      RepositorioMembro
	convites     repositorioConvite
	usuarios     buscadorUsuario
	boards       RepositorioBoard
	responsaveis RepositorioResponsavel
}

// NovoMembroUseCase cria uma instância de MembroUseCase com as dependências injetadas.
func NovoMembroUseCase(
	membros RepositorioMembro,
	convites repositorioConvite,
	usuarios buscadorUsuario,
	boards RepositorioBoard,
	responsaveis RepositorioResponsavel,
) *MembroUseCase {
	return &MembroUseCase{
		membros: membros, convites: convites, usuarios: usuarios,
		boards: boards, responsaveis: responsaveis,
	}
}

// ResultadoConvite é o que sai de Convidar. Os dois caminhos são diferentes o
// bastante para a tela precisar saber qual aconteceu: quem já tinha conta entrou
// agora, e quem não tinha precisa receber um link.
type ResultadoConvite struct {
	// Adicionado é true quando a pessoa já tinha conta e virou membro na hora.
	Adicionado bool
	// Participante vem preenchido quando Adicionado é true.
	Participante *Participante
	// Convite e Token vêm preenchidos quando o convite ficou pendente. O token
	// em texto puro aparece NESTA resposta e em nenhuma outra: o banco guarda
	// só o hash, então quem não copiar o link agora precisa gerar outro.
	Convite *dconvite.Convite
	Token   string
}

// DetalheConvite é o que um convite mostra antes de ser aceito, para quem tem o
// link. Não exige sessão: quem foi convidado normalmente ainda nem tem conta.
type DetalheConvite struct {
	TituloQuadro string
	Email        string
	Papel        membro.Papel
	ConvidadoPor string
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

// Convidar acrescenta alguém ao quadro pelo email. Quem já tem conta vira
// membro na hora; quem não tem recebe um convite pendente com token, e o dono
// entrega o link.
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

	convidado, err := uc.usuarios.BuscarPorEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if convidado != nil {
		return uc.adicionarDireto(ctx, boardID, usuarioID, convidado, papel)
	}

	pendente, err := uc.convites.BuscarPendentePorEmail(ctx, boardID, email)
	if err != nil {
		return nil, err
	}
	if pendente != nil && pendente.Pendente(time.Now()) {
		return nil, dconvite.ErrJaConvidado
	}

	t, err := token.Gerar()
	if err != nil {
		return nil, err
	}
	c, err := dconvite.Novo(uuid.NewString(), boardID, email, papel, token.Hash(t), usuarioID)
	if err != nil {
		return nil, err
	}
	if err := uc.convites.Salvar(ctx, c); err != nil {
		return nil, err
	}
	return &ResultadoConvite{Convite: c, Token: t}, nil
}

// adicionarDireto cria o vínculo de quem já tem conta, sem passar por convite:
// não há o que confirmar quando a pessoa já provou ser dona daquele email ao
// se cadastrar.
//
// Publica MembrosAlterados: quem já estava no quadro vê o novo participante na
// lista de membros e no seletor de responsáveis sem recarregar.
func (uc *MembroUseCase) adicionarDireto(ctx context.Context, boardID, quemAdiciona string, u *dusuario.Usuario, papel membro.Papel) (*ResultadoConvite, error) {
	existente, err := uc.membros.Buscar(ctx, boardID, u.ID)
	if err != nil {
		return nil, err
	}
	if existente != nil {
		return nil, dconvite.ErrJaEMembro
	}

	vinculo, err := membro.Novo(boardID, u.ID, papel)
	if err != nil {
		return nil, err
	}
	if err := uc.membros.Salvar(ctx, vinculo); err != nil {
		return nil, err
	}
	// O autor é quem ADICIONOU, e o payload diz quem foi adicionado. Publicar
	// sem autor fazia a auditoria mostrar "conta removida entrou no quadro" —
	// uma frase falsa sobre um fato verdadeiro.
	uc.publicar(ctx, evento.MembroAdicionado, boardID, quemAdiciona,
		DadosDoMembro{Nome: u.Nome, Email: u.Email, Papel: string(papel)})

	return &ResultadoConvite{
		Adicionado: true,
		Participante: &Participante{
			UsuarioID: u.ID,
			Nome:      u.Nome,
			Email:     u.Email,
			Papel:     papel,
			CriadoEm:  vinculo.CriadoEm,
		},
	}, nil
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
	if err := uc.membros.Atualizar(ctx, vinculo); err != nil {
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
	// Trocar o papel de alguém muda o que ELA pode fazer, e a tela dela precisa
	// saber na hora: sem este aviso, quem foi rebaixado a leitor continuava
	// vendo os botões de edição até dar F5 — e levava 403 ao usar qualquer um.
	uc.publicar(ctx, evento.MembroPapelAlterado, boardID, usuarioID,
		DadosDoMembro{Nome: p.Nome, Email: p.Email, Papel: string(papel), PapelAnterior: string(papelAnterior)})
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

	// As atribuições saem ANTES do vínculo. A ordem é o que importa se algo
	// falhar no meio: sobrar vínculo sem atribuição é um estado que a próxima
	// remoção conserta, enquanto sobrar atribuição sem vínculo deixaria o nome
	// de quem não tem mais acesso pendurado nos cards — e o filtro "meus cards"
	// mostraria a essa pessoa cards que ela não consegue abrir.
	// Quem está saindo é resolvido ANTES da remoção: depois dela não há vínculo
	// nem por onde descobrir de quem se tratava, e o evento diria só que "alguém
	// saiu".
	nomeDoAlvo, emailDoAlvo := "", ""
	if u, err := uc.usuarios.BuscarPorID(ctx, alvoID); err == nil && u != nil {
		nomeDoAlvo, emailDoAlvo = u.Nome, u.Email
	}
	if err := uc.responsaveis.RemoverDoBoard(ctx, boardID, alvoID); err != nil {
		return err
	}
	if err := uc.membros.Remover(ctx, boardID, alvoID); err != nil {
		return err
	}
	uc.publicar(ctx, evento.MembroRemovido, boardID, usuarioID, DadosDoMembro{Nome: nomeDoAlvo, Email: emailDoAlvo})
	return nil
}

// RevogarConvite apaga um convite pendente, invalidando o link já entregue.
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
	return uc.convites.Remover(ctx, conviteID)
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

	detalhe := &DetalheConvite{TituloQuadro: b.Titulo, Email: c.Email, Papel: c.Papel}
	if autor, err := uc.usuarios.BuscarPorID(ctx, c.CriadoPor); err == nil && autor != nil {
		detalhe.ConvidadoPor = autor.Nome
	}
	return detalhe, nil
}

// Aceitar transforma o convite em vínculo, devolvendo o quadro e o papel com
// que a pessoa entrou. Exige que a conta autenticada tenha o email para o qual
// o convite foi feito: sem isso, o link vazado colocaria qualquer pessoa dentro
// do quadro.
// Aceitar publica MembrosAlterados pelo mesmo motivo de adicionarDireto: quem
// já está no quadro vê o recém-chegado sem recarregar.
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
	if u.Email != c.Email {
		return nil, "", dconvite.ErrOutroDestinatario
	}

	b, err := uc.boards.BuscarPorID(ctx, c.BoardID)
	if err != nil {
		return nil, "", err
	}
	if b == nil {
		return nil, "", dconvite.ErrInvalido
	}

	// Já ser membro não é erro: o convite se dá por cumprido e a pessoa segue
	// para o quadro. Clicar duas vezes no link não pode virar tela de erro.
	existente, err := uc.membros.Buscar(ctx, c.BoardID, usuarioID)
	if err != nil {
		return nil, "", err
	}
	if existente == nil {
		vinculo, err := membro.Novo(c.BoardID, usuarioID, c.Papel)
		if err != nil {
			return nil, "", err
		}
		if err := uc.membros.Salvar(ctx, vinculo); err != nil {
			return nil, "", err
		}
	}

	if err := c.Aceitar(time.Now()); err != nil {
		return nil, "", err
	}
	if err := uc.convites.Atualizar(ctx, c); err != nil {
		return nil, "", err
	}
	uc.publicar(ctx, evento.MembroEntrou, c.BoardID, usuarioID,
		DadosDoMembro{Email: c.Email, Papel: string(c.Papel)})
	return b, c.Papel, nil
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
