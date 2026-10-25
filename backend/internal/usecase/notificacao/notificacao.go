// Package notificacao é o sino de cada pessoa: a lista dos avisos dela, quantos
// ainda pedem atenção e o marcar como lido.
//
// Fica separado de usecase/board de propósito. Os casos de uso do quadro CRIAM
// avisos, dentro da transação do que os causou; este pacote os LÊ, e nada aqui
// toma o lock de quadro nenhum — ler e marcar como lido é assunto de uma pessoa,
// não do quadro.
package notificacao

import (
	"context"
	"time"

	dconvite "stacktrack/internal/domain/convite"
	dnotificacao "stacktrack/internal/domain/notificacao"
)

// Item é um aviso como a lista o mostra: o que aconteceu, quem fez, sobre o quê
// — com os títulos de AGORA — e se ainda vale.
//
// Os títulos vêm do estado atual, e não de uma cópia gravada no aviso, ao
// contrário do histórico do quadro: o histórico registra o que era verdade na
// hora, e o aviso aponta para algo que existe hoje. Um card renomeado aparece
// com o nome novo, que é o nome que a pessoa vai encontrar ao clicar.
type Item struct {
	ID        string
	Tipo      dnotificacao.Tipo
	BoardID   string
	CardID    string
	ConviteID string
	// AutorNome é vazio no prazo, que não tem pessoa por trás.
	AutorNome    string
	TituloQuadro string
	TituloCard   string
	// Trecho é o começo do comentário, na menção — lido do comentário de agora,
	// para nunca mostrar um texto que o autor já mudou.
	Trecho   string
	Prazo    *time.Time
	CriadaEm time.Time
	LidaEm   *time.Time
	// Vigente diz se o fato ainda vale, calculado pelo banco sobre o estado
	// atual (ver adapter/repository/notificacao_postgres.go).
	Vigente bool
	// O estado do convite, lido junto, para a tela dizer o desfecho.
	ConviteAceitoEm   *time.Time
	ConviteRecusadoEm *time.Time
	ConviteRevogadoEm *time.Time
	ConviteExpiraEm   *time.Time
	// Situacao é preenchida pelo caso de uso a partir dos campos acima.
	Situacao dnotificacao.Situacao
}

// Repositorio é o sino do ponto de vista de quem LÊ.
//
// Toda consulta recebe o instante de agora, que é o que decide se um convite já
// venceu: o relógio do processo, e não o now() do banco, pela mesma razão das
// transições do convite.
type Repositorio interface {
	// Listar devolve até `limite` avisos visíveis da pessoa, do mais novo para
	// o mais antigo, a partir do aviso `antesDe` (vazio é o começo).
	Listar(ctx context.Context, usuarioID, antesDe string, limite int, agora time.Time) ([]Item, error)
	// Buscar devolve (nil, nil) quando o aviso não existe ou não é visível
	// para a pessoa.
	Buscar(ctx context.Context, id, usuarioID string, agora time.Time) (*Item, error)
	// ContarNaoLidas conta só o que ainda vale: um convite recusado não pede
	// atenção de ninguém.
	ContarNaoLidas(ctx context.Context, usuarioID string, agora time.Time) (int, error)
	// MarcarLida marca o aviso e cancela o email dele que ainda não saiu.
	// Devolve false quando o aviso não é da pessoa ou não é visível para ela.
	MarcarLida(ctx context.Context, id, usuarioID string, em time.Time) (bool, error)
	// MarcarTodasLidas faz o mesmo com todos os avisos não lidos da pessoa.
	MarcarTodasLidas(ctx context.Context, usuarioID string, em time.Time) error
}

// Campainha avisa as outras telas da pessoa: ler numa aba precisa apagar o
// número do sino nas outras.
type Campainha interface {
	Tocar(usuarioIDs ...string)
}

// UseCase reúne o sino de uma pessoa.
type UseCase struct {
	repo      Repositorio
	campainha Campainha
	relogio   func() time.Time
}

// Novo cria o caso de uso. A campainha pode ser nil: as outras abas descobrem a
// leitura na próxima vez que relerem.
func Novo(repo Repositorio, campainha Campainha) *UseCase {
	return &UseCase{repo: repo, campainha: campainha, relogio: time.Now}
}

// ComRelogio troca o relógio — o dublê de fronteira dos testes, que avançam o
// tempo para ver um convite vencer.
func (uc *UseCase) ComRelogio(relogio func() time.Time) *UseCase {
	uc.relogio = relogio
	return uc
}

// Pagina é um pedaço da lista, com o total de não lidas.
type Pagina struct {
	Itens []Item
	// NaoLidas é o número do sino. Vem junto da lista para a tela nunca
	// mostrar uma lista e um número de instantes diferentes.
	NaoLidas int
	// TemMais diz se há avisos mais antigos, para a tela não oferecer um
	// "carregar mais" vazio.
	TemMais bool
}

// Listar devolve uma página dos avisos da pessoa e quantos ela não leu.
func (uc *UseCase) Listar(ctx context.Context, usuarioID, antesDe string) (*Pagina, error) {
	agora := uc.relogio()
	// Um a mais, para saber se há página seguinte sem um COUNT.
	itens, err := uc.repo.Listar(ctx, usuarioID, antesDe, dnotificacao.TamanhoDaPagina+1, agora)
	if err != nil {
		return nil, err
	}
	pagina := &Pagina{TemMais: len(itens) > dnotificacao.TamanhoDaPagina}
	if pagina.TemMais {
		itens = itens[:dnotificacao.TamanhoDaPagina]
	}
	for i := range itens {
		itens[i].Situacao = situacaoDe(itens[i], agora)
	}
	pagina.Itens = itens
	if pagina.NaoLidas, err = uc.repo.ContarNaoLidas(ctx, usuarioID, agora); err != nil {
		return nil, err
	}
	return pagina, nil
}

// Detalhar devolve um aviso da pessoa — é o que a página do link do email
// consulta. Retorna dnotificacao.ErrNaoEncontrada para aviso de outra pessoa,
// de quadro de que ela já não participa, ou que não existe mais.
func (uc *UseCase) Detalhar(ctx context.Context, id, usuarioID string) (*Item, error) {
	agora := uc.relogio()
	item, err := uc.repo.Buscar(ctx, id, usuarioID, agora)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, dnotificacao.ErrNaoEncontrada
	}
	item.Situacao = situacaoDe(*item, agora)
	return item, nil
}

// ContarNaoLidas é o número do sino.
func (uc *UseCase) ContarNaoLidas(ctx context.Context, usuarioID string) (int, error) {
	return uc.repo.ContarNaoLidas(ctx, usuarioID, uc.relogio())
}

// MarcarLida marca o aviso como lido — e cancela o email dele, se ainda não
// saiu: quem já viu no sistema não precisa do email. Marcar de novo não é erro.
// Retorna dnotificacao.ErrNaoEncontrada nos mesmos casos de Detalhar.
func (uc *UseCase) MarcarLida(ctx context.Context, id, usuarioID string) error {
	marcada, err := uc.repo.MarcarLida(ctx, id, usuarioID, uc.relogio())
	if err != nil {
		return err
	}
	if !marcada {
		return dnotificacao.ErrNaoEncontrada
	}
	uc.tocar(usuarioID)
	return nil
}

// MarcarTodasLidas marca todos os avisos não lidos da pessoa.
func (uc *UseCase) MarcarTodasLidas(ctx context.Context, usuarioID string) error {
	if err := uc.repo.MarcarTodasLidas(ctx, usuarioID, uc.relogio()); err != nil {
		return err
	}
	uc.tocar(usuarioID)
	return nil
}

func (uc *UseCase) tocar(usuarioID string) {
	if uc.campainha != nil {
		uc.campainha.Tocar(usuarioID)
	}
}

// situacaoDe diz se o aviso ainda vale e, quando não vale, por quê.
//
// O convite responde pelo próprio estado, com a mesma régua do domínio do
// convite — aceito e recusado são fatos e vencem o relógio. Os demais tipos
// respondem pelo que o banco calculou.
func situacaoDe(i Item, agora time.Time) dnotificacao.Situacao {
	if i.Tipo == dnotificacao.TipoConvite && i.ConviteExpiraEm != nil {
		c := dconvite.Convite{
			AceitoEm: i.ConviteAceitoEm, RecusadoEm: i.ConviteRecusadoEm,
			RevogadoEm: i.ConviteRevogadoEm, ExpiraEm: *i.ConviteExpiraEm,
		}
		switch c.EstadoEm(agora) {
		case dconvite.EstadoAceito:
			return dnotificacao.SituacaoConviteAceito
		case dconvite.EstadoRecusado:
			return dnotificacao.SituacaoConviteRecusado
		case dconvite.EstadoRevogado:
			return dnotificacao.SituacaoConviteRevogado
		case dconvite.EstadoExpirado:
			return dnotificacao.SituacaoConviteVencido
		}
		return dnotificacao.SituacaoVigente
	}
	if i.Vigente {
		return dnotificacao.SituacaoVigente
	}
	return dnotificacao.SituacaoEncerrada
}
