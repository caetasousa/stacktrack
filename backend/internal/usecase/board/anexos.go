package board

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	danexo "stacktrack/internal/domain/anexo"
	dcard "stacktrack/internal/domain/card"
	"stacktrack/internal/domain/evento"

	"github.com/google/uuid"
)

// AnexoUseCase reúne os arquivos e links pendurados nos cards.
type AnexoUseCase struct {
	eventos
	membros RepositorioMembro
	colunas RepositorioColuna
	cards   RepositorioCard
	anexos  RepositorioAnexo
	armazem armazemDeArquivos
	cotas   Cotas
}

// NovoAnexoUseCase cria uma instância de AnexoUseCase com as dependências injetadas.
func NovoAnexoUseCase(
	membros RepositorioMembro,
	colunas RepositorioColuna,
	cards RepositorioCard,
	anexos RepositorioAnexo,
	armazem armazemDeArquivos,
) *AnexoUseCase {
	return &AnexoUseCase{
		membros: membros, colunas: colunas, cards: cards, anexos: anexos, armazem: armazem,
		// Os tetos do produto valem por padrão: um usecase construído sem
		// dizer nada já limita, em vez de aceitar tudo até alguém lembrar de
		// configurá-lo.
		cotas: CotasPadrao(),
	}
}

// ConteudoDeAnexo é um arquivo pronto para ser servido ao navegador.
type ConteudoDeAnexo struct {
	Anexo   danexo.Anexo
	Leitura io.ReadCloser
}

// Listar devolve os anexos do card. Qualquer membro pode ver.
func (uc *AnexoUseCase) Listar(ctx context.Context, cardID, usuarioID string) ([]danexo.Anexo, error) {
	if _, err := uc.boardComAcesso(ctx, cardID, usuarioID, false); err != nil {
		return nil, err
	}
	return uc.anexos.ListarDoCard(ctx, cardID)
}

// AnexarLink pendura uma URL no card. Exige papel de edição.
func (uc *AnexoUseCase) AnexarLink(ctx context.Context, cardID, usuarioID, nome, endereco string) (*danexo.Anexo, error) {
	boardID, err := uc.boardComAcesso(ctx, cardID, usuarioID, true)
	if err != nil {
		return nil, err
	}

	a, err := danexo.NovoLink(uuid.NewString(), cardID, nome, endereco, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := uc.escreverEPublicarNoCard(ctx, evento.AnexoAdicionado, boardID, cardID, usuarioID,
		DadosDoCard{CardID: cardID, Titulo: tituloDoCard(ctx, uc.cards, cardID), Alvo: a.Nome},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			// O link conta para o teto de ANEXOS do card, e não para o de
			// bytes do quadro: ele não ocupa disco nenhum.
			if err := uc.conferirCotaDoCard(ctx, e, cardID); err != nil {
				return err
			}
			return e.Anexos.Salvar(ctx, a)
		}); err != nil {
		return nil, err
	}
	return a, nil
}

// Cotas são os tetos de anexo que este usecase aplica.
//
// São campos, e não constantes lidas direto do domínio, por uma razão prática
// que virou boa: com o teto fixo em 1 GiB, testar a cota do quadro exigiria
// gravar cem arquivos de 10 MiB — um gigabyte em memória para provar uma
// comparação. Injetando os tetos, o teste usa números pequenos e exercita
// exatamente a mesma lógica.
//
// Os padrões continuam no domínio (anexo.MaximoDeAnexosPorCard,
// anexo.MaximoDeBytesPorBoard): quem decide o que é um limite razoável é o
// produto, não a configuração.
type Cotas struct {
	// PorCard é quantos anexos um card comporta.
	PorCard int
	// BytesPorBoard é quanto os ARQUIVOS de um quadro podem ocupar. Link não
	// entra na conta — ele não ocupa disco.
	BytesPorBoard int64
}

// CotasPadrao devolve os tetos do produto.
func CotasPadrao() Cotas {
	return Cotas{
		PorCard:       danexo.MaximoDeAnexosPorCard,
		BytesPorBoard: danexo.MaximoDeBytesPorBoard,
	}
}

// ComCotas troca os tetos deste usecase. Zero em qualquer campo desliga aquele
// teto.
func (uc *AnexoUseCase) ComCotas(c Cotas) {
	uc.cotas = c
}

// conferirCotaDoCard recusa o anexo quando o card já está no teto.
//
// A contagem acontece DENTRO da transação, sob o lock do quadro. Feita antes
// dela, dois envios simultâneos leriam "19" os dois e gravariam o vigésimo
// primeiro juntos — que é exatamente o cenário que o critério de aceite de A4
// chama de "cotas resistem a requisições concorrentes".
func (uc *AnexoUseCase) conferirCotaDoCard(ctx context.Context, e Escrita, cardID string) error {
	if uc.cotas.PorCard <= 0 {
		return nil
	}
	atuais, err := e.Anexos.ContarDoCard(ctx, cardID)
	if err != nil {
		return err
	}
	if atuais >= uc.cotas.PorCard {
		return danexo.ErrAnexosDemaisNoCard
	}
	return nil
}

// conferirCotaDoQuadro recusa o arquivo quando ele não cabe no que resta do
// quadro. Só ARQUIVOS entram na conta — link não ocupa disco.
func (uc *AnexoUseCase) conferirCotaDoQuadro(ctx context.Context, e Escrita, boardID string, tamanho int64) error {
	if uc.cotas.BytesPorBoard <= 0 {
		return nil
	}
	usados, err := e.Anexos.BytesDoBoard(ctx, boardID)
	if err != nil {
		return err
	}
	if usados+tamanho > uc.cotas.BytesPorBoard {
		return danexo.ErrCotaDoQuadroExcedida
	}
	return nil
}

// AnexarArquivo recebe o conteúdo em STREAMING, decide com os fatos medidos e
// só então publica o arquivo. Exige papel de edição.
//
// A ordem das três fases é o desenho inteiro:
//
//  1. RECEBER. O conteúdo vai para um temporário no mesmo diretório dos anexos,
//     medindo tamanho, tipo e hash no caminho. O processo nunca segura o
//     arquivo em memória, e o teto é aplicado durante a leitura — não pelo
//     Content-Length, que é entrada do cliente e é exatamente o que quem ataca
//     falsifica.
//  2. DECIDIR. Tipo e tamanho vêm do que foi MEDIDO. As cotas são conferidas
//     dentro da transação, sob o lock do quadro, junto da gravação da linha e
//     do evento.
//  3. PUBLICAR. Um `rename` atômico promove o temporário ao nome definitivo.
//     Nenhum leitor jamais encontra um arquivo publicado pela metade.
//
// A publicação vem DEPOIS do commit, e é a ordem que evita o pior dos dois
// estados possíveis. Publicando antes, uma falha do commit deixaria um arquivo
// órfão no disco — lixo, mas inofensivo. Publicando depois, o risco é a linha
// existir com o arquivo ainda por publicar, e é por isso que a falha do
// `Publicar` desfaz o anexo em seguida: linha apontando para arquivo
// inexistente é o anexo quebrado que a tela mostra e ninguém consegue
// consertar pela interface.
func (uc *AnexoUseCase) AnexarArquivo(ctx context.Context,
	cardID, usuarioID, nomeOriginal string,
	conteudo io.Reader,
) (*danexo.Anexo, error) {
	boardID, err := uc.boardComAcesso(ctx, cardID, usuarioID, true)
	if err != nil {
		return nil, err
	}

	recebido, err := uc.armazem.Receber(conteudo, danexo.TamanhoMaximoArquivo)
	if err != nil {
		if errors.Is(err, ErrArquivoAcimaDoLimite) {
			return nil, danexo.ErrArquivoGrande
		}
		return nil, err
	}
	// Qualquer saída que não seja publicação bem-sucedida apaga o temporário.
	publicado := false
	defer func() {
		if !publicado {
			_ = uc.armazem.Descartar(recebido)
		}
	}()

	if recebido.Bytes() <= 0 {
		return nil, danexo.ErrArquivoVazio
	}
	// O tipo vem do CONTEÚDO. O Content-Type do multipart é escolhido por quem
	// envia; aceitá-lo faria a lista de permissão do domínio deixar de
	// significar o que ela diz significar.
	if !danexo.TipoPermitido(recebido.Tipo()) {
		return nil, danexo.ErrTipoNaoPermitido
	}

	// O anexo é montado com um caminho AINDA VAZIO: o nome definitivo só existe
	// depois da publicação, e publicar antes de o domínio aprovar seria gravar
	// o que vai ser recusado.
	a, err := danexo.NovoArquivo(uuid.NewString(), cardID, nomeOriginal, "",
		recebido.Tipo(), recebido.Bytes(), usuarioID)
	if err != nil {
		return nil, err
	}

	caminho, err := uc.armazem.Publicar(recebido, extensaoDe(nomeOriginal))
	if err != nil {
		return nil, err
	}
	publicado = true
	a.Caminho = caminho

	if err := uc.escreverEPublicarNoCard(ctx, evento.AnexoAdicionado, boardID, cardID, usuarioID,
		DadosDoCard{CardID: cardID, Titulo: tituloDoCard(ctx, uc.cards, cardID), Alvo: a.Nome},
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}
			// As duas cotas são conferidas DENTRO da transação, sob o lock do
			// quadro: é o que faz dois envios simultâneos não passarem os dois.
			if err := uc.conferirCotaDoCard(ctx, e, cardID); err != nil {
				return err
			}
			if err := uc.conferirCotaDoQuadro(ctx, e, boardID, recebido.Bytes()); err != nil {
				return err
			}
			return e.Anexos.Salvar(ctx, a)
		}); err != nil {
		// O arquivo já foi publicado: desfazer é apagá-lo do disco. Sem isto, um
		// envio recusado pela cota deixaria bytes sem nenhuma linha apontando
		// para eles — órfão permanente, invisível.
		uc.descartar(ctx, caminho)
		return nil, err
	}
	return a, nil
}

// Baixar devolve o conteúdo do anexo para quem participa do quadro.
//
// O arquivo NÃO é servido por um caminho estático adivinhável: passa por aqui
// justamente para a mesma checagem de participação valer para ele.
func (uc *AnexoUseCase) Baixar(ctx context.Context, anexoID, usuarioID string) (*ConteudoDeAnexo, error) {
	a, err := uc.anexos.BuscarPorID(ctx, anexoID)
	if err != nil {
		return nil, err
	}
	if a == nil || a.Tipo != danexo.TipoArquivo {
		return nil, danexo.ErrNaoEncontrado
	}
	if _, err := uc.boardComAcesso(ctx, a.CardID, usuarioID, false); err != nil {
		return nil, traduzirParaAnexo(err)
	}

	leitura, err := uc.armazem.Abrir(a.Caminho)
	if err != nil {
		return nil, err
	}
	return &ConteudoDeAnexo{Anexo: *a, Leitura: leitura}, nil
}

// Apagar remove o anexo e, quando é arquivo, o conteúdo do armazém.
func (uc *AnexoUseCase) Apagar(ctx context.Context, anexoID, usuarioID string) error {
	a, err := uc.anexos.BuscarPorID(ctx, anexoID)
	if err != nil {
		return err
	}
	if a == nil {
		return danexo.ErrNaoEncontrado
	}
	boardID, err := uc.boardComAcesso(ctx, a.CardID, usuarioID, true)
	if err != nil {
		return traduzirParaAnexo(err)
	}

	dados := &DadosDoCard{CardID: a.CardID}
	var removido *danexo.Anexo
	if err := uc.escreverEPublicarNoCard(ctx, evento.AnexoRemovido, boardID, a.CardID, usuarioID,
		dados,
		uc.escrita(), func(e Escrita) error {
			if err := revalidarEdicao(ctx, e, boardID, usuarioID); err != nil {
				return err
			}

			// A busca otimista acima só descobre em qual quadro obter o lock. O
			// anexo é relido aqui, já sob esse lock: se outra exclusão venceu
			// enquanto esta requisição esperava, não existe mudança nem evento a
			// registrar.
			atual, err := e.Anexos.BuscarPorID(ctx, anexoID)
			if err != nil {
				return err
			}
			if atual == nil || atual.CardID != a.CardID {
				return danexo.ErrNaoEncontrado
			}

			apagou, err := e.Anexos.Apagar(ctx, anexoID)
			if err != nil {
				return err
			}
			if !apagou {
				return danexo.ErrNaoEncontrado
			}

			removido = atual
			dados.CardID = atual.CardID
			dados.Titulo = tituloDoCard(ctx, e.Cards, atual.CardID)
			dados.Alvo = atual.Nome
			return nil
		}); err != nil {
		return err
	}
	// A linha some primeiro — agora junto do evento, no mesmo commit. Com o
	// arquivo órfão no disco a tela fica correta e sobra lixo; na ordem
	// inversa, um erro deixaria a tela mostrando anexo que não abre.
	//
	// O descarte é o ponto fraco que sobra, e A4 o resolve: apagar bytes fora
	// da transação não tem como ser desfeito, então o correto é registrar a
	// exclusão num outbox (arquivo_exclusoes) e deixar um worker removê-los
	// depois de o backup cobrir a exclusão.
	if removido != nil && removido.Tipo == danexo.TipoArquivo {
		uc.descartar(ctx, removido.Caminho)
	}
	return nil
}

// boardComAcesso percorre card → coluna → quadro e confere o papel exigido.
func (uc *AnexoUseCase) boardComAcesso(ctx context.Context, cardID, usuarioID string, precisaEditar bool) (string, error) {
	c, err := uc.cards.BuscarPorID(ctx, cardID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(ctx, c.ColunaID)
	if err != nil {
		return "", err
	}
	if col == nil {
		return "", dcard.ErrNaoEncontrado
	}

	if precisaEditar {
		_, err = acessoDeEdicao(ctx, uc.membros, col.BoardID, usuarioID)
	} else {
		_, err = acesso(ctx, uc.membros, col.BoardID, usuarioID)
	}
	if err != nil {
		return "", traduzirParaCard(err)
	}
	return col.BoardID, nil
}

// descartar apaga um arquivo do armazém sem interromper o fluxo: se a limpeza
// falhar, sobra lixo no disco — chato, e melhor do que derrubar uma operação
// que já deu certo do ponto de vista de quem pediu.
func (uc *AnexoUseCase) descartar(ctx context.Context, caminho string) {
	if err := uc.armazem.Remover(caminho); err != nil {
		slog.Warn("anexo órfão no armazém",
			slog.String("caminho", caminho), slog.String("erro", err.Error()))
	}
}

// extensaoDe devolve a extensão do nome original, em minúsculas, para o arquivo
// gravado manter o formato reconhecível no disco. Só extensão curta e
// alfanumérica: o resto do nome vem de quem envia e não entra em caminho.
func extensaoDe(nome string) string {
	ext := strings.ToLower(filepath.Ext(filepath.Base(nome)))
	if len(ext) < 2 || len(ext) > 8 {
		return ""
	}
	for _, r := range ext[1:] {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return ""
		}
	}
	return ext
}
