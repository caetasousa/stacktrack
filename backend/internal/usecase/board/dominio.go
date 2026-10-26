package board

import (
	"context"
	"errors"
	"time"

	dboard "stacktrack/internal/domain/board"
	dconvite "stacktrack/internal/domain/convite"
	"stacktrack/internal/domain/dominio"
	"stacktrack/internal/domain/evento"
	"stacktrack/internal/domain/membro"
	dusuario "stacktrack/internal/domain/usuario"
)

// AcessoPorDominio é a porta do domínio de um quadro, como o dono a vê.
type AcessoPorDominio struct {
	// Dominio e Papel descrevem a porta aberta; vazios, ela está fechada.
	Dominio string
	Papel   membro.Papel
	// Disponivel é o domínio que quem pergunta pode abrir — o do email
	// confirmado dele. Vazio quando não há, e aí Impedimento diz por quê.
	Disponivel  string
	Impedimento error
}

// ComDominios liga a leitura da porta do domínio fora da transação — a lista
// do painel e a consulta do dono. Sem ela, ninguém vê quadro aberto ao domínio.
func (uc *MembroUseCase) ComDominios(dominios RepositorioDominio) {
	uc.dominios = dominios
}

// ConsultarDominio devolve a porta do domínio do quadro e o domínio que quem
// pergunta poderia abrir. Só o dono.
func (uc *MembroUseCase) ConsultarDominio(ctx context.Context, boardID, usuarioID string) (*AcessoPorDominio, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, dboard.ErrNaoEncontrado
	}
	acesso := &AcessoPorDominio{}
	acesso.Disponivel, acesso.Impedimento = dominio.ParaAbrir(u.Email, u.EmailConfirmado(), membro.PapelEditor)
	if uc.dominios != nil {
		if acesso.Dominio, acesso.Papel, err = uc.dominios.DoQuadro(ctx, boardID); err != nil {
			return nil, err
		}
	}
	return acesso, nil
}

// AbrirAoDominio abre o quadro a quem tiver email confirmado do MESMO domínio
// do dono que pede, com o papel escolhido. Abrir de novo troca o papel de quem
// ainda vai entrar; quem já entrou mantém o seu.
//
// Retorna dominio.ErrEmailNaoConfirmado, dominio.ErrDominioPessoal ou
// dominio.ErrPapelDoDominio, sem mudar nada.
func (uc *MembroUseCase) AbrirAoDominio(ctx context.Context, boardID, usuarioID string, papel membro.Papel) (*AcessoPorDominio, error) {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return nil, err
	}
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, dboard.ErrNaoEncontrado
	}
	d, err := dominio.ParaAbrir(u.Email, u.EmailConfirmado(), papel)
	if err != nil {
		return nil, err
	}

	agora := time.Now()
	if err := uc.escreverEPublicar(ctx, evento.QuadroDominioAberto, boardID, usuarioID,
		DadosDoQuadro{Dominio: d, Papel: string(papel)},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			return e.Dominios.Definir(ctx, boardID, d, papel, agora)
		}); err != nil {
		return nil, err
	}
	return &AcessoPorDominio{Dominio: d, Papel: papel, Disponivel: d}, nil
}

// FecharAoDominio fecha a porta: quem ainda não entrou deixa de ver o quadro,
// e quem já entrou continua — tirar alguém é Remover. Fechar o que já está
// fechado não grava nada.
func (uc *MembroUseCase) FecharAoDominio(ctx context.Context, boardID, usuarioID string) error {
	if _, err := acessoDeAdministracao(ctx, uc.membros, boardID, usuarioID); err != nil {
		return err
	}
	if uc.dominios == nil {
		return nil
	}
	d, _, err := uc.dominios.DoQuadro(ctx, boardID)
	if err != nil || d == "" {
		return err
	}
	agora := time.Now()
	return uc.escreverEPublicar(ctx, evento.QuadroDominioFechado, boardID, usuarioID,
		DadosDoQuadro{Dominio: d},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarAdministracao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			return e.Dominios.Definir(ctx, boardID, "", "", agora)
		})
}

// QuadrosDoMeuDominio devolve os quadros abertos ao domínio da conta de que
// ela ainda não participa. Lista vazia — e não erro — para conta sem email
// confirmado ou com email pessoal: para ela, simplesmente não há nenhum.
func (uc *MembroUseCase) QuadrosDoMeuDominio(ctx context.Context, usuarioID string) ([]QuadroDoDominio, error) {
	vazia := []QuadroDoDominio{}
	if uc.dominios == nil {
		return vazia, nil
	}
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	if u == nil || !u.EmailConfirmado() {
		return vazia, nil
	}
	d := dominio.DoEmail(u.Email)
	if d == "" || dominio.Pessoal(d) {
		return vazia, nil
	}
	return uc.dominios.AbertosPara(ctx, d, usuarioID)
}

// EntrarPeloDominio põe a conta no quadro aberto ao domínio do email dela, com
// o papel que o dono escolheu, e avisa quem está no quadro (MembroEntrou).
// Entrar de novo, já participando, devolve o papel de agora sem gravar nada.
//
// Responde dboard.ErrNaoEncontrado para tudo o que não dá direito de entrar —
// porta fechada, outro domínio, email não confirmado, pessoa que o dono tirou
// —, como o quadro de que se não participa: distinguir os casos diria a quem
// tenta ids que o quadro existe. Retorna membro.ErrQuadroCheio no teto.
func (uc *MembroUseCase) EntrarPeloDominio(ctx context.Context, boardID, usuarioID string) (*dboard.Board, membro.Papel, error) {
	if uc.dominios == nil {
		return nil, "", dboard.ErrNaoEncontrado
	}
	u, err := uc.usuarios.BuscarPorID(ctx, usuarioID)
	if err != nil {
		return nil, "", err
	}
	if u == nil || !u.EmailConfirmado() {
		return nil, "", dboard.ErrNaoEncontrado
	}
	b, err := uc.boards.BuscarPorID(ctx, boardID)
	if err != nil {
		return nil, "", err
	}
	if b == nil {
		return nil, "", dboard.ErrNaoEncontrado
	}

	// O papel vai no evento, que é montado antes da transação: lido aqui, e
	// conferido de novo sob o lock. Se o dono o trocou no meio, repete — como
	// o aceite do convite faz com a participação.
	for tentativa := 0; tentativa < 2; tentativa++ {
		d, papel, errLeitura := uc.dominios.DoQuadro(ctx, boardID)
		if errLeitura != nil {
			return nil, "", errLeitura
		}
		if !dominio.Pertence(u.Email, d) {
			return nil, "", dboard.ErrNaoEncontrado
		}
		err = uc.escreverEPublicar(ctx, evento.MembroEntrou, boardID, usuarioID,
			DadosDoMembro{Email: dusuario.MascararEmail(u.Email), Papel: string(papel), Dominio: d},
			uc.escrita(), func(e Escrita) error {
				dSobLock, papelSobLock, err := e.Dominios.DoQuadro(ctx, boardID)
				if err != nil {
					return err
				}
				if !dominio.Pertence(u.Email, dSobLock) {
					return dboard.ErrNaoEncontrado
				}
				if papelSobLock != papel {
					return errParticipacaoMudou
				}
				bloqueado, err := e.Dominios.Bloqueado(ctx, boardID, usuarioID)
				if err != nil {
					return err
				}
				if bloqueado {
					return dboard.ErrNaoEncontrado
				}
				todos, err := e.Membros.Todos(ctx, boardID)
				if err != nil {
					return err
				}
				for _, m := range todos {
					if m.UsuarioID == usuarioID {
						return dconvite.ErrJaEMembro
					}
				}
				if len(todos) >= membro.MaximoPorQuadro {
					return membro.ErrQuadroCheio
				}
				vinculo, err := membro.Novo(boardID, usuarioID, papel)
				if err != nil {
					return err
				}
				return e.Membros.Salvar(ctx, vinculo)
			})
		if errors.Is(err, errParticipacaoMudou) {
			continue
		}
		if errors.Is(err, dconvite.ErrJaEMembro) {
			// Duas abas clicando juntas: a segunda encontra a primeira já
			// dentro. Para ela, o resultado é o mesmo — está no quadro.
			atual, errBusca := uc.membros.Buscar(ctx, boardID, usuarioID)
			if errBusca != nil || atual == nil {
				return nil, "", dboard.ErrNaoEncontrado
			}
			return b, atual.Papel, nil
		}
		if err != nil {
			return nil, "", err
		}
		return b, papel, nil
	}
	return nil, "", err
}
