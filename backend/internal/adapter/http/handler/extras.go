// Handlers do que pende do card: etiquetas, checklists e anexos.

package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"stacktrack/internal/adapter/http/dto"
	detiqueta "stacktrack/internal/domain/etiqueta"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/go-chi/chi/v5"
)

// ExtrasHandler concentra etiquetas, checklists e anexos. Os três compartilham
// a mesma tradução de erro e o mesmo jeito de descobrir quem está autenticado.
type ExtrasHandler struct {
	etiquetas            *ucboard.EtiquetaUseCase
	checklists           *ucboard.ChecklistUseCase
	anexos               *ucboard.AnexoUseCase
	responsaveis         *ucboard.ResponsavelUseCase
	comentarios          *ucboard.ComentarioUseCase
	atividade            *ucboard.AtividadeUseCase
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoExtrasHandler cria uma instância de ExtrasHandler com os usecases injetados.
func NovoExtrasHandler(
	etiquetas *ucboard.EtiquetaUseCase,
	checklists *ucboard.ChecklistUseCase,
	anexos *ucboard.AnexoUseCase,
	responsaveis *ucboard.ResponsavelUseCase,
	comentarios *ucboard.ComentarioUseCase,
	atividade *ucboard.AtividadeUseCase,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *ExtrasHandler {
	return &ExtrasHandler{
		etiquetas: etiquetas, checklists: checklists, anexos: anexos,
		responsaveis:         responsaveis,
		comentarios:          comentarios,
		atividade:            atividade,
		identidadeDoContexto: identidadeDoContexto,
	}
}

func (h *ExtrasHandler) usuario(w http.ResponseWriter, r *http.Request) (string, bool) {
	identidade, ok := h.identidadeDoContexto(r)
	if !ok {
		responderErro(w, http.StatusUnauthorized, "não autenticado")
		return "", false
	}
	return identidade.UsuarioID, true
}

// --- etiquetas ------------------------------------------------------------

// ListarEtiquetas devolve as etiquetas do quadro.
func (h *ExtrasHandler) ListarEtiquetas(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	etiquetas, err := h.etiquetas.Listar(r.Context(), chi.URLParam(r, "boardID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao listar etiquetas", err)
		return
	}
	responderJSON(w, http.StatusOK, paraEtiquetasResponse(etiquetas))
}

// CriarEtiqueta acrescenta uma etiqueta ao quadro.
func (h *ExtrasHandler) CriarEtiqueta(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.EtiquetaRequest](w, r)
	if !ok {
		return
	}

	e, err := h.etiquetas.Criar(r.Context(), chi.URLParam(r, "boardID"), usuarioID, req.Nome, detiqueta.Cor(req.Cor))
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar etiqueta", err)
		return
	}
	responderJSON(w, http.StatusCreated, paraEtiquetaResponse(*e))
}

// EditarEtiqueta troca nome e cor, valendo para todos os cards que a usam.
func (h *ExtrasHandler) EditarEtiqueta(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.EtiquetaRequest](w, r)
	if !ok {
		return
	}

	e, err := h.etiquetas.Editar(r.Context(), chi.URLParam(r, "etiquetaID"), usuarioID, req.Nome, detiqueta.Cor(req.Cor))
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao editar etiqueta", err)
		return
	}
	responderJSON(w, http.StatusOK, paraEtiquetaResponse(*e))
}

// ApagarEtiqueta remove a etiqueta do quadro e de todos os cards.
func (h *ExtrasHandler) ApagarEtiqueta(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.etiquetas.Apagar(r.Context(), chi.URLParam(r, "etiquetaID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar etiqueta", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AplicarEtiqueta pendura a etiqueta no card.
func (h *ExtrasHandler) AplicarEtiqueta(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	err := h.etiquetas.Aplicar(r.Context(), chi.URLParam(r, "cardID"), chi.URLParam(r, "etiquetaID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao aplicar etiqueta", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoverEtiqueta tira a etiqueta do card, sem apagá-la do quadro.
func (h *ExtrasHandler) RemoverEtiqueta(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	err := h.etiquetas.Remover(r.Context(), chi.URLParam(r, "cardID"), chi.URLParam(r, "etiquetaID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao remover etiqueta", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- responsáveis ---------------------------------------------------------

// Atribuir marca alguém como responsável pelo card.
//
// PUT, e não POST: atribuir a mesma pessoa duas vezes leva ao mesmo estado, e
// é isso que o PUT promete. A tela pode repetir a chamada sem consultar antes.
func (h *ExtrasHandler) Atribuir(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	err := h.responsaveis.Atribuir(r.Context(), chi.URLParam(r, "cardID"), chi.URLParam(r, "usuarioID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao atribuir responsável", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Desatribuir tira a pessoa da responsabilidade do card, sem tirá-la do quadro.
func (h *ExtrasHandler) Desatribuir(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	err := h.responsaveis.Desatribuir(r.Context(), chi.URLParam(r, "cardID"), chi.URLParam(r, "usuarioID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao remover responsável", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- comentários ----------------------------------------------------------

// ListarComentarios devolve a conversa do card, do mais antigo para o mais novo.
func (h *ExtrasHandler) ListarComentarios(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	lista, err := h.comentarios.Listar(r.Context(), chi.URLParam(r, "cardID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao listar comentários", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.ListaComentariosResponse{Comentarios: paraComentariosResponse(lista)})
}

// Comentar acrescenta uma mensagem ao card.
func (h *ExtrasHandler) Comentar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.ComentarioRequest](w, r)
	if !ok {
		return
	}

	c, err := h.comentarios.Criar(r.Context(), chi.URLParam(r, "cardID"), usuarioID, req.Texto)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao comentar", err)
		return
	}
	responderJSON(w, http.StatusCreated, paraComentarioResponse(ucboard.ComentarioComAutor{Comentario: *c}))
}

// EditarComentario troca o texto. Só o autor.
func (h *ExtrasHandler) EditarComentario(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.ComentarioRequest](w, r)
	if !ok {
		return
	}

	c, err := h.comentarios.Editar(r.Context(), chi.URLParam(r, "comentarioID"), usuarioID, req.Texto)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao editar comentário", err)
		return
	}
	responderJSON(w, http.StatusOK, paraComentarioResponse(ucboard.ComentarioComAutor{Comentario: *c}))
}

// ApagarComentario remove a mensagem. O autor apaga a própria; quem administra
// o quadro apaga a de qualquer um.
func (h *ExtrasHandler) ApagarComentario(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.comentarios.Apagar(r.Context(), chi.URLParam(r, "comentarioID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar comentário", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- histórico -------------------------------------------------------------

// Atividade devolve o que aconteceu com o card, do mais recente para o mais
// antigo. É um read model sobre o log de eventos — não há tabela própria.
func (h *ExtrasHandler) Atividade(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	lista, err := h.atividade.DoCard(r.Context(), chi.URLParam(r, "cardID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao ler o histórico", err)
		return
	}

	fora := make([]dto.AtividadeResponse, 0, len(lista))
	for _, a := range lista {
		fora = append(fora, dto.AtividadeResponse{
			Seq: a.Seq, Tipo: string(a.Tipo), AutorID: a.AutorID,
			AutorNome: a.AutorNome, AutorEmail: a.AutorEmail,
			Dados: a.Dados, OcorridoEm: a.OcorridoEm,
		})
	}
	responderJSON(w, http.StatusOK, dto.ListaAtividadeResponse{Atividade: fora})
}

// AtividadeDoQuadro devolve o histórico do quadro inteiro — a auditoria.
//
// Responde a pergunta que o histórico por card não responde quando há muitos
// cards: "quem mexeu na ordem deste quadro, e quando". Por padrão traz só as
// movimentações, que é o assunto; `filtro=tudo` abre para o resto.
//
// Parâmetros, todos opcionais:
//
//	filtro=movimentacoes|tudo   recorte (padrão: movimentacoes)
//	autor=<usuarioID>           só o que aquela pessoa fez
//	antesDe=<seq>               cursor da página seguinte
//
// O cursor é o seq da última linha recebida, e não um número de página: o log
// recebe escrita o tempo todo, e paginar por deslocamento pularia linhas que
// entrassem no meio — numa auditoria, pular em silêncio é o pior defeito.
func (h *ExtrasHandler) AtividadeDoQuadro(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	consulta := r.URL.Query()
	filtro := ucboard.FiltroDeAtividade{
		// Qualquer valor que não seja exatamente "tudo" cai no recorte estreito.
		// É a direção segura para um parâmetro vindo da URL: um erro de digitação
		// devolve menos, e não a história inteira do quadro.
		SoMovimentacoes: consulta.Get("filtro") != "tudo",
		AutorID:         consulta.Get("autor"),
	}
	// Cursor inválido é tratado como ausente, e não como erro: quem chama com
	// lixo recebe a primeira página, que é o que ele veria de qualquer forma.
	if antes, err := strconv.ParseInt(consulta.Get("antesDe"), 10, 64); err == nil && antes > 0 {
		filtro.AntesDe = antes
	}

	pagina, err := h.atividade.DoBoard(r.Context(), chi.URLParam(r, "boardID"), usuarioID, filtro)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao ler a auditoria do quadro", err)
		return
	}

	fora := make([]dto.AtividadeResponse, 0, len(pagina.Linhas))
	for _, a := range pagina.Linhas {
		fora = append(fora, dto.AtividadeResponse{
			Seq: a.Seq, Tipo: string(a.Tipo), AutorID: a.AutorID,
			AutorNome: a.AutorNome, AutorEmail: a.AutorEmail,
			Dados: a.Dados, OcorridoEm: a.OcorridoEm,
		})
	}
	responderJSON(w, http.StatusOK, dto.ListaAtividadeResponse{Atividade: fora, TemMais: pagina.TemMais})
}

// --- checklists -----------------------------------------------------------

// CriarChecklist acrescenta uma lista de verificação ao card.
func (h *ExtrasHandler) CriarChecklist(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.ChecklistRequest](w, r)
	if !ok {
		return
	}

	c, err := h.checklists.Criar(r.Context(), chi.URLParam(r, "cardID"), usuarioID, req.Titulo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar checklist", err)
		return
	}
	responderJSON(w, http.StatusCreated, dto.ChecklistResponse{
		ID: c.ID, CardID: c.CardID, Titulo: c.Titulo, Posicao: c.Posicao,
		Itens: []dto.ChecklistItemResponse{},
	})
}

// RenomearChecklist troca o título da lista.
func (h *ExtrasHandler) RenomearChecklist(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.ChecklistRequest](w, r)
	if !ok {
		return
	}

	c, err := h.checklists.Renomear(r.Context(), chi.URLParam(r, "checklistID"), usuarioID, req.Titulo)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao renomear checklist", err)
		return
	}
	responderJSON(w, http.StatusOK, dto.ChecklistResponse{
		ID: c.ID, CardID: c.CardID, Titulo: c.Titulo, Posicao: c.Posicao,
		Itens: []dto.ChecklistItemResponse{},
	})
}

// ApagarChecklist remove a lista e os itens dela.
func (h *ExtrasHandler) ApagarChecklist(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.checklists.Apagar(r.Context(), chi.URLParam(r, "checklistID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar checklist", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CriarItem acrescenta uma linha à checklist.
func (h *ExtrasHandler) CriarItem(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.ItemRequest](w, r)
	if !ok {
		return
	}

	item, err := h.checklists.CriarItem(r.Context(), chi.URLParam(r, "checklistID"), usuarioID, req.Texto)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao criar item", err)
		return
	}
	responderJSON(w, http.StatusCreated, paraItemResponse(*item))
}

// EditarItem troca o texto e/ou marca a linha.
func (h *ExtrasHandler) EditarItem(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.ItemPatchRequest](w, r)
	if !ok {
		return
	}

	item, err := h.checklists.EditarItem(r.Context(), chi.URLParam(r, "itemID"), usuarioID, req.Texto, req.Concluido)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao editar item", err)
		return
	}
	responderJSON(w, http.StatusOK, paraItemResponse(*item))
}

// ApagarItem remove a linha da checklist.
func (h *ExtrasHandler) ApagarItem(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.checklists.ApagarItem(r.Context(), chi.URLParam(r, "itemID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar item", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- anexos ---------------------------------------------------------------

// AnexarLink pendura uma URL no card.
func (h *ExtrasHandler) AnexarLink(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}
	req, ok := decodificarJSON[dto.LinkRequest](w, r)
	if !ok {
		return
	}

	a, err := h.anexos.AnexarLink(r.Context(), chi.URLParam(r, "cardID"), usuarioID, req.Nome, req.URL)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao anexar link", err)
		return
	}
	responderJSON(w, http.StatusCreated, paraAnexoResponse(*a))
}

// nomeDoCampoDoArquivo é o campo do multipart que carrega o conteúdo.
const nomeDoCampoDoArquivo = "arquivo"

// AnexarArquivo recebe um upload multipart em STREAMING.
//
// `MultipartReader`, e não `FormFile`. A diferença é o que o processo segura na
// memória: `FormFile` chama `ParseMultipartForm`, que materializa o formulário
// inteiro — até 32 MiB em memória por padrão, o excedente em arquivos
// temporários do sistema, e todos os campos antes de qualquer decisão. Dez
// envios simultâneos de 10 MiB davam centenas de megabytes num container de
// 384 MiB. Com o reader, o conteúdo passa por um buffer de 32 KiB e vai direto
// para o disco.
//
// O teto de transporte já foi aplicado pelo middleware LimitarCorpo, que
// reconhece o multipart e usa o limite maior. Aqui o domínio decide se o
// arquivo é aceitável, a partir do que foi MEDIDO — e não do que o cliente
// declarou —, e responde uma mensagem que faz sentido para quem enviou.
func (h *ExtrasHandler) AnexarArquivo(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	partes, err := r.MultipartReader()
	if err != nil {
		responderErro(w, http.StatusBadRequest, "envie o arquivo como multipart/form-data")
		return
	}

	// As partes são percorridas até achar a do arquivo. Cada `NextPart` fecha a
	// anterior, então nenhuma delas é acumulada: um formulário com campos extras
	// passa por aqui sem custo de memória.
	for {
		parte, err := partes.NextPart()
		if errors.Is(err, io.EOF) {
			responderErro(w, http.StatusBadRequest, "envie um arquivo no campo 'arquivo'")
			return
		}
		if err != nil {
			responderErro(w, http.StatusBadRequest, "envio interrompido ou malformado")
			return
		}
		if parte.FormName() != nomeDoCampoDoArquivo {
			parte.Close()
			continue
		}

		// O nome de arquivo é METADADO, e vai sanitizado pelo domínio (ver
		// anexo.NovoArquivo, que corta para o nome-base). Ele nunca vira
		// caminho: o nome físico é sorteado pelo armazém.
		a, err := h.anexos.AnexarArquivo(r.Context(),
			chi.URLParam(r, "cardID"), usuarioID, parte.FileName(), parte,
		)
		parte.Close()
		if err != nil {
			responderErroDeQuadro(w, r, "erro ao anexar arquivo", err)
			return
		}
		responderJSON(w, http.StatusCreated, paraAnexoResponse(*a))
		return
	}
}

// BaixarAnexo entrega o conteúdo do arquivo a quem participa do quadro.
func (h *ExtrasHandler) BaixarAnexo(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	conteudo, err := h.anexos.Baixar(r.Context(), chi.URLParam(r, "anexoID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao baixar anexo", err)
		return
	}
	defer conteudo.Leitura.Close()

	// attachment, e não inline: o arquivo veio de quem enviou, e abrir no
	// navegador na NOSSA origem transformaria um upload em execução de conteúdo
	// de terceiro dentro do nosso domínio.
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", nomeParaURL(conteudo.Anexo.Nome)))
	w.Header().Set("Content-Type", "application/octet-stream")
	// Diz aos navegadores para não adivinharem o tipo pelo conteúdo.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if conteudo.Anexo.Tamanho > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", conteudo.Anexo.Tamanho))
	}

	if _, err := copiarParaResposta(w, conteudo.Leitura); err != nil {
		// O cabeçalho já foi enviado: não dá para trocar o status agora, só
		// registrar que a entrega saiu incompleta.
		logarFalhaDeDownload(r, conteudo.Anexo.ID, err)
	}
}

// ApagarAnexo remove o anexo do card.
func (h *ExtrasHandler) ApagarAnexo(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.anexos.Apagar(r.Context(), chi.URLParam(r, "anexoID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao apagar anexo", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nomeParaURL escapa o nome para o filename* do Content-Disposition, que exige
// percent-encoding — nome com acento ou espaço quebraria o cabeçalho.
func nomeParaURL(nome string) string {
	var b strings.Builder
	for _, r := range []byte(nome) {
		seguro := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_'
		if seguro {
			b.WriteByte(r)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", r)
	}
	return b.String()
}
