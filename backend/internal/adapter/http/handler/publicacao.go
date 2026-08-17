package handler

import (
	"errors"
	"net/http"
	"net/url"

	"stacktrack/internal/adapter/http/dto"
	dpublicacao "stacktrack/internal/domain/publicacao"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"

	"github.com/go-chi/chi/v5"
)

// PublicacaoHandler expõe o link público de acompanhamento de um quadro.
//
// As três primeiras rotas são do DONO e vivem no grupo autenticado; Ver é a
// única rota da aplicação inteira que serve conteúdo de quadro sem sessão, e
// por isso mora num handler separado — quem for auditar "o que dá para ver sem
// estar logado?" tem um arquivo só para ler.
type PublicacaoHandler struct {
	publicacoes *ucboard.PublicacaoUseCase
	// origemFrontend é a base do link: o backend não sabe em que endereço a
	// página pública é servida. Mesmo caminho do link de convite.
	origemFrontend       string
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool)
}

// NovoPublicacaoHandler cria uma instância de PublicacaoHandler com o usecase injetado.
func NovoPublicacaoHandler(
	publicacoes *ucboard.PublicacaoUseCase,
	origemFrontend string,
	identidadeDoContexto func(r *http.Request) (ucauth.Identidade, bool),
) *PublicacaoHandler {
	return &PublicacaoHandler{
		publicacoes:          publicacoes,
		origemFrontend:       origemFrontend,
		identidadeDoContexto: identidadeDoContexto,
	}
}

// Consultar devolve o estado do link público do quadro. Só o dono.
func (h *PublicacaoHandler) Consultar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	p, err := h.publicacoes.Atual(r.Context(), chi.URLParam(r, "boardID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao consultar o link público", err)
		return
	}
	responderJSON(w, http.StatusOK, h.paraResposta(p))
}

// Publicar liga o link público do quadro e devolve a URL. Só o dono.
//
// É PUT, e não POST, porque a operação é idempotente: repetir devolve o mesmo
// link em vez de criar um segundo. Um POST prometeria o contrário.
func (h *PublicacaoHandler) Publicar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	p, err := h.publicacoes.Publicar(r.Context(), chi.URLParam(r, "boardID"), usuarioID)
	if err != nil {
		responderErroDeQuadro(w, r, "erro ao publicar o quadro", err)
		return
	}
	responderJSON(w, http.StatusOK, h.paraResposta(p))
}

// Revogar desliga o link público do quadro (204). Só o dono. Revogar um quadro
// que não está publicado responde 204 igual: o resultado pretendido já vale.
func (h *PublicacaoHandler) Revogar(w http.ResponseWriter, r *http.Request) {
	usuarioID, ok := h.usuario(w, r)
	if !ok {
		return
	}

	if err := h.publicacoes.Revogar(r.Context(), chi.URLParam(r, "boardID"), usuarioID); err != nil {
		responderErroDeQuadro(w, r, "erro ao revogar o link público", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Ver devolve o quadro a quem chega pelo link público, SEM sessão. Responde 404
// para token desconhecido, revogado ou de quadro apagado — os três iguais.
func (h *PublicacaoHandler) Ver(w http.ResponseWriter, r *http.Request) {
	quadro, err := h.publicacoes.Ver(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		if errors.Is(err, dpublicacao.ErrNaoEncontrada) {
			responderErro(w, http.StatusNotFound, err.Error())
			return
		}
		responderErroInterno(w, r, "erro ao carregar o quadro público", err)
		return
	}

	// no-store, e não um cache curto: revogar precisa valer NA HORA. Guardada
	// por um intermediário — o proxy reverso à frente da API, o cache de uma
	// rede corporativa —, uma cópia continuaria sendo servida depois de o dono
	// ter desligado o link, e ele não teria como saber.
	w.Header().Set("Cache-Control", "no-store")
	// O link é para quem o dono mandou, não para quem procurar no buscador. O
	// mesmo pedido vai na página (meta robots), porque quem indexa a resposta da
	// API e quem indexa a página são rastreadores diferentes.
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")

	colunas := make([]dto.ColunaPublicaResponse, 0, len(quadro.Colunas))
	for _, c := range quadro.Colunas {
		cards := make([]dto.CardPublicoResponse, 0, len(c.Cards))
		for _, card := range c.Cards {
			etiquetas := make([]dto.EtiquetaPublicaResponse, 0, len(card.Etiquetas))
			for _, e := range card.Etiquetas {
				etiquetas = append(etiquetas, dto.EtiquetaPublicaResponse{Nome: e.Nome, Cor: string(e.Cor)})
			}
			cards = append(cards, dto.CardPublicoResponse{
				Titulo:    card.Titulo,
				Descricao: card.Descricao,
				Cor:       string(card.Cor),
				Prazo:     card.Prazo,
				Vencido:   card.Vencido,
				Etiquetas: etiquetas,
				Checklist: dto.ProgressoResponse{
					Concluidos: card.Checklist.Concluidos,
					Total:      card.Checklist.Total,
				},
			})
		}
		colunas = append(colunas, dto.ColunaPublicaResponse{
			Titulo: c.Titulo, Cor: string(c.Cor), Cards: cards,
		})
	}

	responderJSON(w, http.StatusOK, dto.QuadroPublicoResponse{
		Titulo:       quadro.Titulo,
		Fundo:        quadro.Fundo,
		AtualizadoEm: quadro.AtualizadoEm,
		Colunas:      colunas,
	})
}

// paraResposta traduz a publicação (ou a ausência dela) para o corpo que a tela
// do dono consome.
func (h *PublicacaoHandler) paraResposta(p *dpublicacao.Publicacao) dto.PublicacaoResponse {
	if p == nil {
		return dto.PublicacaoResponse{Publicado: false}
	}
	criadoEm := p.CriadoEm
	return dto.PublicacaoResponse{
		Publicado: true,
		URL:       h.origemFrontend + "/publico/" + url.PathEscape(p.Token),
		CriadoEm:  &criadoEm,
	}
}

// usuario recupera a identidade injetada pelo middleware, respondendo 401
// quando ela não está lá.
func (h *PublicacaoHandler) usuario(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := h.identidadeDoContexto(r)
	if !ok {
		responderErro(w, http.StatusUnauthorized, "não autenticado")
		return "", false
	}
	return id.UsuarioID, true
}
