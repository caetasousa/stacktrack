package board

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"

	danexo "kanbango/internal/domain/anexo"
	dcard "kanbango/internal/domain/card"

	"github.com/google/uuid"
)

// AnexoUseCase reúne os arquivos e links pendurados nos cards.
type AnexoUseCase struct {
	membros repositorioMembro
	colunas repositorioColuna
	cards   repositorioCard
	anexos  repositorioAnexo
	armazem armazemDeArquivos
}

// NovoAnexoUseCase cria uma instância de AnexoUseCase com as dependências injetadas.
func NovoAnexoUseCase(
	membros repositorioMembro,
	colunas repositorioColuna,
	cards repositorioCard,
	anexos repositorioAnexo,
	armazem armazemDeArquivos,
) *AnexoUseCase {
	return &AnexoUseCase{membros: membros, colunas: colunas, cards: cards, anexos: anexos, armazem: armazem}
}

// ConteudoDeAnexo é um arquivo pronto para ser servido ao navegador.
type ConteudoDeAnexo struct {
	Anexo   danexo.Anexo
	Leitura io.ReadCloser
}

// Listar devolve os anexos do card. Qualquer membro pode ver.
func (uc *AnexoUseCase) Listar(cardID, usuarioID string) ([]danexo.Anexo, error) {
	if _, err := uc.boardComAcesso(cardID, usuarioID, false); err != nil {
		return nil, err
	}
	return uc.anexos.ListarDoCard(cardID)
}

// AnexarLink pendura uma URL no card. Exige papel de edição.
func (uc *AnexoUseCase) AnexarLink(cardID, usuarioID, nome, endereco string) (*danexo.Anexo, error) {
	if _, err := uc.boardComAcesso(cardID, usuarioID, true); err != nil {
		return nil, err
	}

	a, err := danexo.NovoLink(uuid.NewString(), cardID, nome, endereco, usuarioID)
	if err != nil {
		return nil, err
	}
	if err := uc.anexos.Salvar(a); err != nil {
		return nil, err
	}
	return a, nil
}

// AnexarArquivo grava o conteúdo no armazém e registra o anexo. Exige papel de
// edição.
//
// A ordem importa: o arquivo é gravado ANTES da linha no banco, e se o banco
// falhar o arquivo é apagado em seguida. O contrário deixaria linha apontando
// para arquivo inexistente — que a tela mostraria como anexo quebrado, sem
// conserto possível pela interface.
func (uc *AnexoUseCase) AnexarArquivo(
	cardID, usuarioID, nomeOriginal, mime string,
	tamanho int64,
	conteudo io.Reader,
) (*danexo.Anexo, error) {
	if _, err := uc.boardComAcesso(cardID, usuarioID, true); err != nil {
		return nil, err
	}

	// Validar ANTES de gravar: não faz sentido escrever no disco um arquivo
	// grande demais só para apagá-lo depois.
	if tamanho <= 0 {
		return nil, danexo.ErrArquivoVazio
	}
	if tamanho > danexo.TamanhoMaximoArquivo {
		return nil, danexo.ErrArquivoGrande
	}
	if !danexo.TipoPermitido(mime) {
		return nil, danexo.ErrTipoNaoPermitido
	}

	caminho, err := uc.armazem.Guardar(conteudo, extensaoDe(nomeOriginal))
	if err != nil {
		return nil, err
	}

	a, err := danexo.NovoArquivo(uuid.NewString(), cardID, nomeOriginal, caminho, mime, tamanho, usuarioID)
	if err != nil {
		uc.descartar(caminho)
		return nil, err
	}
	if err := uc.anexos.Salvar(a); err != nil {
		uc.descartar(caminho)
		return nil, err
	}
	return a, nil
}

// Baixar devolve o conteúdo do anexo para quem participa do quadro.
//
// O arquivo NÃO é servido por um caminho estático adivinhável: passa por aqui
// justamente para a mesma checagem de participação valer para ele.
func (uc *AnexoUseCase) Baixar(anexoID, usuarioID string) (*ConteudoDeAnexo, error) {
	a, err := uc.anexos.BuscarPorID(anexoID)
	if err != nil {
		return nil, err
	}
	if a == nil || a.Tipo != danexo.TipoArquivo {
		return nil, danexo.ErrNaoEncontrado
	}
	if _, err := uc.boardComAcesso(a.CardID, usuarioID, false); err != nil {
		return nil, traduzirParaAnexo(err)
	}

	leitura, err := uc.armazem.Abrir(a.Caminho)
	if err != nil {
		return nil, err
	}
	return &ConteudoDeAnexo{Anexo: *a, Leitura: leitura}, nil
}

// Apagar remove o anexo e, quando é arquivo, o conteúdo do armazém.
func (uc *AnexoUseCase) Apagar(anexoID, usuarioID string) error {
	a, err := uc.anexos.BuscarPorID(anexoID)
	if err != nil {
		return err
	}
	if a == nil {
		return danexo.ErrNaoEncontrado
	}
	if _, err := uc.boardComAcesso(a.CardID, usuarioID, true); err != nil {
		return traduzirParaAnexo(err)
	}

	if err := uc.anexos.Apagar(anexoID); err != nil {
		return err
	}
	// A linha some primeiro: com o arquivo órfão no disco a tela fica correta,
	// e sobra lixo. Na ordem inversa, um erro deixaria a tela mostrando anexo
	// que não abre.
	if a.Tipo == danexo.TipoArquivo {
		uc.descartar(a.Caminho)
	}
	return nil
}

// boardComAcesso percorre card → coluna → quadro e confere o papel exigido.
func (uc *AnexoUseCase) boardComAcesso(cardID, usuarioID string, precisaEditar bool) (string, error) {
	c, err := uc.cards.BuscarPorID(cardID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", dcard.ErrNaoEncontrado
	}
	col, err := uc.colunas.BuscarPorID(c.ColunaID)
	if err != nil {
		return "", err
	}
	if col == nil {
		return "", dcard.ErrNaoEncontrado
	}

	if precisaEditar {
		_, err = acessoDeEdicao(uc.membros, col.BoardID, usuarioID)
	} else {
		_, err = acesso(uc.membros, col.BoardID, usuarioID)
	}
	if err != nil {
		return "", traduzirParaCard(err)
	}
	return col.BoardID, nil
}

// descartar apaga um arquivo do armazém sem interromper o fluxo: se a limpeza
// falhar, sobra lixo no disco — chato, e melhor do que derrubar uma operação
// que já deu certo do ponto de vista de quem pediu.
func (uc *AnexoUseCase) descartar(caminho string) {
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
