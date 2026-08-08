// Handlers do que pende do card: etiquetas, checklists e anexos.

package handler

import (
	"fmt"
	"net/http"
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
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoExtrasHandler cria uma instância de ExtrasHandler com os usecases injetados.
func NovoExtrasHandler(
	etiquetas *ucboard.EtiquetaUseCase,
	checklists *ucboard.ChecklistUseCase,
	anexos *ucboard.AnexoUseCase,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *ExtrasHandler {
	return &ExtrasHandler{
		etiquetas: etiquetas, checklists: checklists, anexos: anexos,
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

// AnexarArquivo recebe um upload multipart e guarda o conteúdo.
//
// O teto de transporte já foi aplicado pelo middleware LimitarCorpo, que
// reconhece o multipart e usa o limite maior. Aqui o domínio decide se o
// arquivo é aceitável, e responde uma mensagem que faz sentido para quem enviou.
func (h *ExtrasHandler) AnexarArquivo(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	arquivo, cabecalho, err := r.FormFile("arquivo")
	if err != nil {
		responderErro(w, http.StatusBadRequest, "envie um arquivo no campo 'arquivo'")
		return
	}
	defer arquivo.Close()

	// O Content-Type declarado pelo navegador é palpite dele; para o que
	// importa (não servir HTML da nossa origem) a lista de permissão do domínio
	// decide, e o download força Content-Disposition de qualquer forma.
	mime := cabecalho.Header.Get("Content-Type")

	a, err := h.anexos.AnexarArquivo(r.Context(),
		chi.URLParam(r, "cardID"), usuarioID, cabecalho.Filename, mime, cabecalho.Size, arquivo,
	)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao anexar arquivo", err)
		return
	}
	responderJSON(w, http.StatusCreated, paraAnexoResponse(*a))
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
