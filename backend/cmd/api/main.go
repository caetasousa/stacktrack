// Package main é o entrypoint do servidor HTTP.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"stacktrack/config"
	"stacktrack/internal/adapter/armazem"
	"stacktrack/internal/adapter/http/handler"
	"stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/adapter/security"
	"stacktrack/internal/pkg/logging"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/internal/usecase/manutencao"

	"stacktrack/internal/adapter/http/ws"
	"stacktrack/internal/adapter/realtime/despachante"
	"stacktrack/internal/adapter/realtime/hub"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
)

func main() {
	// logger estruturado (slog): JSON em produção, texto legível em dev.
	// Configurado antes de tudo, para todo log já sair no formato certo.
	logging.Configurar(config.EhProducao())

	// contexto de vida da aplicação: cancelado em SIGINT/SIGTERM, usado pelo
	// desligamento gracioso do servidor HTTP — e, da fase 5 em diante, pelo hub
	// de WebSocket, que precisa fechar as conexões abertas antes de o processo
	// morrer.
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	// A configuração é conferida ANTES do banco, do disco e da porta. Subir
	// primeiro e validar depois deixaria o processo atendendo requisições com
	// uma proteção desligada durante a janela entre um e outro — e, se o erro
	// aparecesse só no primeiro acesso, ele apareceria para um usuário.
	if err := config.Validar(); err != nil {
		slog.Error("configuração recusada", slog.String("erro", err.Error()))
		os.Exit(1)
	}

	proxiesConfiaveis, err := config.ProxiesConfiaveis()
	if err != nil {
		slog.Error("PROXIES_CONFIAVEIS inválida", slog.String("erro", err.Error()))
		os.Exit(1)
	}

	pool, err := config.NovoPool(context.Background())
	if err != nil {
		slog.Error("erro ao conectar no banco", slog.String("erro", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	// O pool com TETO DE ESPERA por conexão livre.
	//
	// `pgxpool` não expõe prazo de aquisição separado do contexto de quem chama,
	// então a espera herdava o orçamento inteiro da requisição — dez segundos.
	// Sob saturação, cada requisição nova ficava viva esses dez segundos
	// segurando goroutine e memória para ser atendida depois de quem pediu já
	// ter desistido. Recusar em dois segundos faz a fila parar de crescer.
	//
	// Todo repositório passa a receber ESTE, e não o pool cru: um que receba o
	// cru continua funcionando e fica de fora do teto, que é o tipo de exceção
	// silenciosa que este projeto evita.
	banco := repository.NovoPoolComEspera(pool, config.EsperaPorConexaoLivre())

	avisarSobreTetosDesligados()

	// repositórios
	usuarioRepo := repository.NovoUsuarioPostgres(banco)
	sessionRepo := repository.NovoSessionPostgres(banco)
	boardRepo := repository.NovoBoardPostgres(banco)
	membroRepo := repository.NovoMembroPostgres(banco)
	colunaRepo := repository.NovoColunaPostgres(banco)
	cardRepo := repository.NovoCardPostgres(banco)
	conviteRepo := repository.NovoConvitePostgres(banco)
	etiquetaRepo := repository.NovoEtiquetaPostgres(banco)
	checklistRepo := repository.NovoChecklistPostgres(banco)
	anexoRepo := repository.NovoAnexoPostgres(banco)
	responsavelRepo := repository.NovoResponsavelPostgres(banco)
	comentarioRepo := repository.NovoComentarioPostgres(banco)
	publicacaoRepo := repository.NovoPublicacaoPostgres(banco)
	// O outbox das exclusões de arquivo: a chave física de cada anexo apagado,
	// gravada na mesma transação do CASCADE. Ver usecase/board/exclusao.go.
	exclusaoRepo := repository.NovoExclusaoDeArquivoPostgres(banco)
	// O log de eventos é repositório como os outros: escreve o outbox e, desde a
	// fase 11, também É a fonte do histórico — o feed é um read model sobre ele.
	logDeEventos := repository.NovoEventoPostgres(banco)

	// armazém de anexos: disco, num volume próprio. O binário não vai para o
	// banco — incharia backup e restore de um schema que guarda texto curto no
	// resto todo.
	armazemAnexos, err := armazem.NovoDisco(config.DiretorioDeAnexos())
	if err != nil {
		slog.Error("erro ao preparar o diretório de anexos", slog.String("erro", err.Error()))
		os.Exit(1)
	}

	// segurança
	hasher := security.NovoHasherArgon2id()

	// usecases
	cadastrarUC := ucauth.NovoCadastrarUseCase(usuarioRepo, sessionRepo, hasher)
	// Conta e sessão no mesmo commit: sem isto, uma falha entre as duas
	// escritas deixava conta criada sem sessão — inacessível, porque a segunda
	// tentativa de cadastro bate no email já em uso e não há recuperação de
	// senha para oferecer.
	cadastrarUC.ComUnidadeDeTrabalho(repository.NovaUnidadeDeAutenticacao(banco))
	loginUC := ucauth.NovoLoginUseCase(usuarioRepo, sessionRepo, hasher)
	logoutUC := ucauth.NovoLogoutUseCase(sessionRepo)
	validarSessaoUC := ucauth.NovoValidarSessaoUseCase(sessionRepo)
	perfilUC := ucauth.NovoPerfilUseCase(usuarioRepo)
	quadroUC := ucboard.NovoQuadroUseCase(boardRepo, membroRepo, colunaRepo, cardRepo, etiquetaRepo, checklistRepo, anexoRepo, responsavelRepo, comentarioRepo, armazemAnexos)
	colunaUC := ucboard.NovoColunaUseCase(membroRepo, colunaRepo, anexoRepo, armazemAnexos)
	cardUC := ucboard.NovoCardUseCase(boardRepo, membroRepo, colunaRepo, cardRepo, etiquetaRepo, checklistRepo, anexoRepo, responsavelRepo, comentarioRepo, armazemAnexos)
	etiquetaUC := ucboard.NovoEtiquetaUseCase(membroRepo, colunaRepo, cardRepo, etiquetaRepo)
	checklistUC := ucboard.NovoChecklistUseCase(membroRepo, colunaRepo, cardRepo, checklistRepo)
	anexoUC := ucboard.NovoAnexoUseCase(membroRepo, colunaRepo, cardRepo, anexoRepo, armazemAnexos)
	membroUC := ucboard.NovoMembroUseCase(membroRepo, conviteRepo, usuarioRepo, boardRepo, responsavelRepo)
	responsavelUC := ucboard.NovoResponsavelUseCase(membroRepo, colunaRepo, cardRepo, responsavelRepo)
	comentarioUC := ucboard.NovoComentarioUseCase(membroRepo, colunaRepo, cardRepo, comentarioRepo)
	atividadeUC := ucboard.NovoAtividadeUseCase(membroRepo, colunaRepo, cardRepo, logDeEventos)
	publicacaoUC := ucboard.NovoPublicacaoUseCase(publicacaoRepo, membroRepo, boardRepo, colunaRepo, cardRepo, etiquetaRepo, checklistRepo)
	// O quadro só CONSULTA a publicação, para dizer a quem edita que ele está à
	// vista de fora. Publicar e revogar é com o publicacaoUC.
	quadroUC.ComPublicacoes(publicacaoRepo)
	// O selo de "quem moveu por último" em cada card vem do log de eventos, que
	// já é a fonte do histórico. Nenhuma tabela nova: auditoria é leitura.
	quadroUC.ComAtividades(logDeEventos)
	// O quadro e o modal de card passam a ser lidos sobre instantâneos únicos
	// (REPEATABLE READ, READ ONLY). Sem isto, as consultas que os montam enxergam
	// instantes diferentes do banco, mas a resposta ainda sairia carimbada com
	// uma única revisão, como se descrevesse um estado coerente.
	instantaneo := repository.NovoInstantaneo(banco, config.TempoMaximoDeComando())
	quadroUC.ComInstantaneo(instantaneo)
	cardUC.ComInstantaneo(instantaneo)
	publicacaoUC.ComInstantaneo(instantaneo)

	// O hub é o adaptador que implementa a porta Publicador. Ligá-lo aqui, e
	// não no construtor de cada usecase, é o que mantém os testes construindo
	// os mesmos usecases sem saber que tempo real existe.
	salaDeEventos := hub.Novo()

	// O DESPACHANTE fica entre quem comita e o hub.
	//
	// Antes, a mesma goroutine que comitava chamava o hub logo depois. Isso
	// tinha dois furos invisíveis de dentro: duas mutações que comitam nas
	// revisões 5 e 6 podiam chegar ao hub na ordem inversa, e um processo que
	// morresse entre o commit e a chamada não entregava aquele evento a
	// ninguém.
	//
	// Agora quem publica apenas ACORDA o despachante, e ele entrega o que está
	// GRAVADO, em ordem de revisão, com polling curto corrigindo wake-up
	// perdido. O pior caso deixa de ser "o evento sumiu" e passa a ser "o
	// evento chegou um segundo depois".
	entregador := despachante.Novo(
		logDeEventos, salaDeEventos, config.IntervaloDoDespachante(), slog.Default(),
	)
	go entregador.Rodar(ctx)
	// A unidade de trabalho é o que faz o dado e o evento caírem no MESMO
	// commit. Sem ela ligada, o usecase ainda funciona — grava numa transação e
	// registra noutra —, e é assim que os testes de regra rodam, sem banco.
	// Em produção, essa separação deixaria buraco invisível no log do quadro.
	unidadeDeTrabalho := repository.NovaUnidadeDeTrabalho(
		pool, config.EsperaPorLockDeQuadro(), config.TempoMaximoDeComando(),
	)
	for _, uc := range []interface {
		ComPublicador(ucboard.Publicador)
		ComPublicadorEfemero(ucboard.Publicador)
		ComRegistro(ucboard.RegistroDeEventos)
		ComEscritaAtomica(ucboard.EscritaAtomica)
	}{
		quadroUC, colunaUC, cardUC, etiquetaUC, checklistUC, anexoUC, responsavelUC, comentarioUC,
		membroUC, publicacaoUC,
	} {
		uc.ComPublicador(entregador)
		// O sinal de quadro apagado não está no log — o CASCADE levou o próprio
		// board_events junto —, então ele fala direto com o hub.
		uc.ComPublicadorEfemero(salaDeEventos)
		uc.ComRegistro(logDeEventos)
		uc.ComEscritaAtomica(unidadeDeTrabalho)
	}

	// handlers e middlewares
	autenticacao := middleware.NovoAuth(
		validarSessaoUC, config.CookieSeguro(),
		config.RateLimitSessaoDesconhecida(), config.JanelaLimitePorConta,
	)
	identidadeDoContexto := func(r *http.Request) (ucauth.Identidade, bool) {
		return middleware.IdentidadeDoContexto(r.Context())
	}
	authHandler := handler.NovoAuthHandler(
		cadastrarUC, loginUC, logoutUC, perfilUC,
		config.CookieSeguro(),
		handler.NovoLimitadorPorConta(config.RateLimitLoginPorConta(), config.JanelaLimitePorConta),
		identidadeDoContexto,
	)
	boardHandler := handler.NovoBoardHandler(quadroUC, colunaUC, cardUC, identidadeDoContexto)
	membroHandler := handler.NovoMembroHandler(membroUC, config.OrigemFrontend(), identidadeDoContexto)
	extrasHandler := handler.NovoExtrasHandler(etiquetaUC, checklistUC, anexoUC, responsavelUC, comentarioUC, atividadeUC, identidadeDoContexto)
	publicacaoHandler := handler.NovoPublicacaoHandler(publicacaoUC, config.OrigemFrontend(), identidadeDoContexto)

	// OriginPatterns com a origem do frontend, e nada além: WebSocket NÃO
	// obedece CORS, então sem esta lista qualquer site que a vítima visitar
	// abre uma conexão autenticada com o cookie dela e lê o quadro inteiro em
	// tempo real (Cross-Site WebSocket Hijacking).
	wsHandler := ws.NovoHandler(
		salaDeEventos,
		logDeEventos,
		quadroUC,
		func(ctx context.Context) (string, bool) {
			id, ok := middleware.IdentidadeDoContexto(ctx)
			return id.UsuarioID, ok
		},
		// O nome é resolvido uma vez por conexão, no handshake — e não a cada
		// evento. Se a consulta falhar, a presença mostra um rótulo genérico em
		// vez de derrubar a conexão: ver quem está no quadro é acessório, e
		// perder o tempo real por causa disso seria trocar o essencial pelo
		// secundário.
		func(ctx context.Context, usuarioID string) string {
			u, err := perfilUC.Executar(ctx, usuarioID)
			if err != nil || u == nil {
				return "alguém"
			}
			return u.Nome
		},
		// A sessão é reconferida enquanto a conexão vive. O middleware autentica
		// uma vez, no handshake, e a conexão dura horas: sem isto, o logout
		// apaga a sessão no banco e o socket continua transmitindo o quadro.
		func(ctx context.Context, token string) bool {
			_, err := validarSessaoUC.Executar(ctx, token)
			return err == nil
		},
		handler.NomeCookieSessao(config.CookieSeguro()),
		[]string{origemSemEsquema(config.OrigemFrontend())},
		slog.Default(),
	).ComObservador(entregador).ComLimites(
		config.ConexoesPorConta(),
		config.ConexoesSimultaneas(),
		config.HandshakesPorMinuto(),
		time.Minute,
	).ComPrazoDePreparacao(config.PrazoDaRequisicao())

	// O porteiro do disco. Disco cheio não degrada, ele QUEBRA: o upload falha
	// no meio, o Postgres para de aceitar escrita porque o WAL não tem para onde
	// ir, e nada disso avisa antes. Ele transforma isso em "recuso escrita
	// agora, e a leitura continua".
	guardaDoDisco := armazem.NovaGuarda(
		config.DiretorioDeAnexos(),
		config.MinimoDeDiscoLivreBytes(),
		config.MinimoDeDiscoLivrePorCem(),
		config.ValidadeDaMedicaoDeDisco(),
	)

	r := config.NovoRouter(proxiesConfiaveis)
	// O porteiro fica na borda inteira, e nao somente depois da autenticacao:
	// cadastro e qualquer futura escrita publica tambem consomem disco. Leituras,
	// DELETE e login/logout continuam para permitir recuperacao.
	r.Use(middleware.PorteiroDeDisco(guardaDoDisco, slog.Default()))
	r.Get("/health", health)
	r.Get("/ready", ready(banco, guardaDoDisco, entregador, salaDeEventos))

	r.Route("/auth", func(r chi.Router) {
		// As respostas daqui entregam identidade e cookie de sessão — nenhuma
		// delas pode ser guardada por navegador, proxy ou CDN.
		r.Use(middleware.SemCache)
		// Os tetos por IP entram por grupo, e não na rota: assim uma rota nova
		// de autenticação nasce protegida em vez de depender de alguém lembrar.
		r.Group(func(r chi.Router) {
			r.Use(limitePorIP(config.RateLimitCadastroPorMinuto()))
			r.Post("/cadastro", authHandler.Cadastrar)
		})
		r.Group(func(r chi.Router) {
			r.Use(limitePorIP(config.RateLimitLoginPorMinuto()))
			r.Post("/login", authHandler.Login)
		})

		r.Post("/logout", authHandler.Logout)

		r.Group(func(r chi.Router) {
			r.Use(autenticacao.Autenticar)
			r.Get("/me", authHandler.Me)
		})
	})

	// Detalhe do convite é PÚBLICO de propósito: quem foi convidado costuma
	// ainda não ter conta, e precisa ver de que quadro se trata antes de criar
	// uma. O token é a credencial — quem não o tem recebe o mesmo 404 de um
	// convite vencido. Aceitar, esse sim, exige sessão.
	//
	// O quadro público é a OUTRA rota sem sessão, e a única que serve conteúdo
	// de quadro sem ela. O token da URL é a autorização inteira; quem não o tem
	// recebe o mesmo 404 de um link revogado. O que ela devolve é uma projeção
	// própria, sem pessoas e sem ids — ver usecase/board.QuadroPublico.
	//
	// O teto por IP importa mais aqui do que no resto: é a única porta que
	// alguém sem conta consegue bater, e um quadro publicado é um GET que lê o
	// quadro inteiro.
	r.Group(func(r chi.Router) {
		r.Use(limitePorIP(config.RateLimitPublicoPorMinuto()))
		// Sem sessão, mas com dado privado: as duas respondem conteúdo de um
		// quadro a quem apresenta um segredo na URL. Um cache compartilhado no
		// caminho guardaria essa resposta e a serviria a quem chegasse depois
		// com outro token.
		r.Use(middleware.SemCache)
		r.Get("/convites/{token}", membroHandler.DetalharConvite)
		r.Get("/publico/{token}", publicacaoHandler.Ver)
	})

	// O WebSocket exige sessao, mas fica fora do teto de requisicoes por sessao:
	// ocupacao e reconexao ja tem limites proprios, e uma conexao dura horas.
	r.Group(func(r chi.Router) {
		r.Use(autenticacao.Autenticar)
		r.Use(middleware.SemCache)
		r.Get("/ws", wsHandler.Acompanhar)
	})

	// Tudo daqui para baixo exige sessão. O teto por sessão fica no grupo, e
	// não em cada rota, pela mesma razão dos tetos de /auth: rota nova nasce
	// coberta em vez de depender de alguém lembrar.
	r.Group(func(r chi.Router) {
		r.Use(autenticacao.Autenticar)
		r.Use(limitePorSessao(config.RateLimitAutenticadoPorMinuto(), config.CookieSeguro()))
		// Tudo daqui para baixo é resposta de UMA pessoa. no-store no grupo, e
		// não handler a handler, pelo mesmo motivo dos tetos: rota nova nasce
		// coberta em vez de depender de alguém lembrar.
		r.Use(middleware.SemCache)

		r.Route("/boards", func(r chi.Router) {
			r.Get("/", boardHandler.Listar)
			r.Post("/", boardHandler.Criar)

			// As rotas de UM quadro ficam num sub-roteador de `/{boardID}`, e
			// não achatadas com o padrão repetido em cada linha. A diferença não
			// é de estilo: é aqui, e só aqui, que o chi já resolveu `boardID`
			// quando os middlewares do grupo rodam — o que permite validar o id
			// ANTES de o caso de uso existir (ver middleware.IDsDaURLValidos).
			// Registrado no grupo acima, o middleware veria a lista de
			// parâmetros vazia e não validaria nada.
			r.Route("/{boardID}", func(r chi.Router) {
				r.Use(middleware.IDsDaURLValidos)

				r.Get("/", boardHandler.Detalhar)
				r.Patch("/", boardHandler.Renomear)
				r.Delete("/", boardHandler.Apagar)
				r.Post("/colunas", boardHandler.CriarColuna)

				r.Get("/membros", membroHandler.Listar)
				r.Post("/membros", membroHandler.Convidar)
				r.Patch("/membros/{usuarioID}", membroHandler.AlterarPapel)
				r.Delete("/membros/{usuarioID}", membroHandler.Remover)
				r.Delete("/convites/{conviteID}", membroHandler.RevogarConvite)

				r.Patch("/fundo", boardHandler.DefinirFundo)

				// O link público. As três exigem papel de dono — o token é o
				// segredo, e quem o recebe pode repassá-lo a quem quiser.
				r.Get("/publicacao", publicacaoHandler.Consultar)
				r.Put("/publicacao", publicacaoHandler.Publicar)
				r.Delete("/publicacao", publicacaoHandler.Revogar)

				// A auditoria do quadro: quem mexeu no quê. Qualquer membro lê,
				// pela mesma razão do histórico de um card — ver não é mexer, e
				// a mesma informação já saía card a card.
				r.Get("/atividade", extrasHandler.AtividadeDoQuadro)

				r.Get("/etiquetas", extrasHandler.ListarEtiquetas)
				r.Post("/etiquetas", extrasHandler.CriarEtiqueta)
			})
		})

		r.Route("/etiquetas/{etiquetaID}", func(r chi.Router) {
			r.Use(middleware.IDsDaURLValidos)
			r.Patch("/", extrasHandler.EditarEtiqueta)
			r.Delete("/", extrasHandler.ApagarEtiqueta)
		})

		r.Route("/checklists/{checklistID}", func(r chi.Router) {
			r.Use(middleware.IDsDaURLValidos)
			r.Patch("/", extrasHandler.RenomearChecklist)
			r.Delete("/", extrasHandler.ApagarChecklist)
			r.Post("/itens", extrasHandler.CriarItem)
		})

		r.Route("/comentarios/{comentarioID}", func(r chi.Router) {
			r.Use(middleware.IDsDaURLValidos)
			r.Patch("/", extrasHandler.EditarComentario)
			r.Delete("/", extrasHandler.ApagarComentario)
		})

		r.Route("/itens/{itemID}", func(r chi.Router) {
			r.Use(middleware.IDsDaURLValidos)
			r.Patch("/", extrasHandler.EditarItem)
			r.Delete("/", extrasHandler.ApagarItem)
		})

		r.Route("/anexos/{anexoID}", func(r chi.Router) {
			r.Use(middleware.IDsDaURLValidos)
			r.Get("/", extrasHandler.BaixarAnexo)
			r.Delete("/", extrasHandler.ApagarAnexo)
		})

		r.Post("/convites/{token}/aceitar", membroHandler.AceitarConvite)

		// Coluna e card são endereçados pelo próprio id, e não sob o caminho
		// do quadro: o servidor descobre a que quadro pertencem para autorizar
		// (card → coluna → quadro). Aceitar o quadro pela URL deixaria alguém
		// mexer em coluna alheia informando o id de um quadro próprio.
		r.Route("/colunas/{colunaID}", func(r chi.Router) {
			r.Use(middleware.IDsDaURLValidos)
			r.Patch("/", boardHandler.RenomearColuna)
			r.Patch("/mover", boardHandler.MoverColuna)
			r.Delete("/", boardHandler.ApagarColuna)
			r.Post("/cards", boardHandler.CriarCard)
		})

		r.Route("/cards/{cardID}", func(r chi.Router) {
			r.Use(middleware.IDsDaURLValidos)
			r.Get("/", boardHandler.DetalharCard)
			r.Patch("/", boardHandler.EditarCard)
			r.Delete("/", boardHandler.ApagarCard)
			r.Patch("/prazo", boardHandler.DefinirPrazo)
			r.Patch("/mover", boardHandler.MoverCard)
			r.Put("/etiquetas/{etiquetaID}", extrasHandler.AplicarEtiqueta)
			r.Delete("/etiquetas/{etiquetaID}", extrasHandler.RemoverEtiqueta)
			r.Put("/responsaveis/{usuarioID}", extrasHandler.Atribuir)
			r.Delete("/responsaveis/{usuarioID}", extrasHandler.Desatribuir)
			r.Get("/atividade", extrasHandler.Atividade)
			r.Get("/comentarios", extrasHandler.ListarComentarios)
			r.Post("/comentarios", extrasHandler.Comentar)
			r.Post("/checklists", extrasHandler.CriarChecklist)
			r.Post("/anexos/link", extrasHandler.AnexarLink)
			r.Post("/anexos/arquivo", extrasHandler.AnexarArquivo)
		})
	})

	// O log de 404 precisa saber quais primeiros segmentos existem de verdade,
	// para conseguir dizer "bateram em algo sob /boards" sem nunca registrar um
	// caminho desconhecido — que é onde um token adivinhado apareceria. A lista
	// é extraída do próprio roteador, e não escrita à mão, para não envelhecer
	// no dia em que alguém acrescentar uma rota.
	logging.DefinirPrefixosConhecidos(prefixosDeRota(r))

	// A faxina roda FORA de qualquer requisição, num intervalo fixo. Antes,
	// quem limpava sessões vencidas era o próprio login: cada pessoa que entrava
	// pagava a limpeza de todo mundo, e a conta crescia justamente quando havia
	// mais gente usando.
	// O coletor de arquivos excluídos entra na MESMA faxina: ele é uma limpeza
	// periódica como as outras, em lotes e fora de qualquer requisição.
	//
	// ⚠️ A porta de cobertura é CoberturaNegada, e isso é a decisão, não um
	// esquecimento: enquanto A6 não fornecer os manifests dos backups externos,
	// nenhuma exclusão é considerada coberta e NENHUM byte sai do disco. O
	// outbox acumula, o worker relata, e produção fica em fail-closed. Disco é
	// barato; um anexo apagado cedo demais não volta.
	coletor := manutencao.NovoColetor(
		exclusaoRepo, ucboard.CoberturaNegada{}, armazemAnexos, slog.Default(),
	)

	faxina := manutencao.Nova(
		config.IntervaloDaFaxina(), config.PrazoDaFaxina(), slog.Default(),
		sessionRepo, conviteRepo, coletor,
	)
	go faxina.Rodar(ctx)

	srv := config.NovoServidor(r)

	go func() {
		slog.Info("servidor no ar", slog.String("endereco", config.Porta()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("erro ao iniciar servidor", slog.String("erro", err.Error()))
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	// O hub fecha ANTES do Shutdown, e a ordem é o ponto: conexão de tempo real
	// não termina sozinha, então o Shutdown esperaria o timeout inteiro com
	// elas abertas e só então as cortaria no meio. Fechando aqui, cada conexão
	// encerra de forma limpa e o Shutdown só espera as requisições HTTP comuns.
	slog.Info("encerrando: fechando conexões de tempo real")
	salaDeEventos.Fechar()

	slog.Info("encerrando: aguardando requisições em andamento")
	ctxDesligamento, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if err := srv.Shutdown(ctxDesligamento); err != nil {
		slog.Error("desligamento forçado", slog.String("erro", err.Error()))
	}

	slog.Info("servidor encerrado")
}

// prefixosDeRota devolve o primeiro segmento de cada rota registrada, sem
// repetição.
func prefixosDeRota(r chi.Routes) []string {
	vistos := make(map[string]struct{})
	_ = chi.Walk(r, func(_ string, rota string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		primeiro, _, _ := strings.Cut(strings.TrimPrefix(rota, "/"), "/")
		// Um segmento de PARÂMETRO como primeiro nível ("/{token}") não é
		// prefixo conhecido: ele casa com qualquer coisa, inclusive com um
		// segredo, e registrá-lo devolveria ao log exatamente o que esta lista
		// existe para manter fora dele.
		if primeiro == "" || strings.HasPrefix(primeiro, "{") || strings.HasPrefix(primeiro, "*") {
			return nil
		}
		vistos[primeiro] = struct{}{}
		return nil
	})

	lista := make([]string, 0, len(vistos))
	for p := range vistos {
		lista = append(lista, p)
	}
	sort.Strings(lista)
	return lista
}

// limitePorIP devolve o middleware de teto por IP, ou um middleware neutro
// quando o limite é 0 (desligado). Sem o caso neutro, desligar o teto exigiria
// montar a rota de outro jeito.
func limitePorIP(porMinuto int) func(http.Handler) http.Handler {
	if porMinuto <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return httprate.LimitByIP(porMinuto, time.Minute)
}

// limitePorSessao devolve o middleware de teto por sessão, chaveado pelo
// cookie e não pelo IP: depois do login, o IP deixa de identificar quem abusa
// (um escritório inteiro sai pelo mesmo endereço, e a mesma conta troca de
// rede). Sem cookie, cai no IP — nesse ponto a requisição já foi autenticada,
// então isso só cobre um caminho que não deveria existir.
func limitePorSessao(porMinuto int, cookieSeguro bool) func(http.Handler) http.Handler {
	if porMinuto <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	nome := handler.NomeCookieSessao(cookieSeguro)
	return httprate.Limit(porMinuto, time.Minute, httprate.WithKeyFuncs(
		func(r *http.Request) (string, error) {
			if c, err := r.Cookie(nome); err == nil {
				return c.Value, nil
			}
			return httprate.KeyByIP(r)
		},
	))
}

// avisarSobreTetosDesligados registra um WARN quando algum teto de requisições
// está em zero. É fácil demais copiar um .env de desenvolvimento (onde os
// tetos atrapalham os testes) para um ambiente exposto e não perceber que o
// login ficou sem proteção nenhuma.
func avisarSobreTetosDesligados() {
	desligados := make([]string, 0, 5)
	for nome, limite := range map[string]int{
		"RATE_LIMIT_LOGIN_POR_MINUTO":       config.RateLimitLoginPorMinuto(),
		"RATE_LIMIT_CADASTRO_POR_MINUTO":    config.RateLimitCadastroPorMinuto(),
		"RATE_LIMIT_LOGIN_POR_CONTA":        config.RateLimitLoginPorConta(),
		"RATE_LIMIT_AUTENTICADO_POR_MINUTO": config.RateLimitAutenticadoPorMinuto(),
		"RATE_LIMIT_PUBLICO_POR_MINUTO":     config.RateLimitPublicoPorMinuto(),
	} {
		if limite == 0 {
			desligados = append(desligados, nome)
		}
	}
	if len(desligados) > 0 {
		sort.Strings(desligados)
		slog.Warn("rate limiting parcialmente desligado: as rotas cobertas ficam sem teto",
			slog.String("variaveis_em_zero", strings.Join(desligados, ", ")))
	}
}

// health informa que o processo está no ar. Não toca em dependência nenhuma —
// use /ready para saber se a API consegue atender.
func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// timeoutReady limita o ping de readiness: sem teto, um banco travado (em vez de
// fora do ar) seguraria a checagem até o timeout do cliente, e o orquestrador
// ficaria sem resposta justamente quando ela mais importa.
const timeoutReady = 2 * time.Second

// ready informa se a API consegue atender de fato, e DISTINGUE os motivos.
//
// A distinção é o ponto. Um readiness booleano obriga a escolher entre duas
// respostas erradas quando o disco enche: dizer "pronto" mantém o tráfego indo
// para uma API que falha em toda escrita, e dizer "não pronto" tira do ar
// também a LEITURA, que continuava funcionando perfeitamente. Nenhuma das duas
// é o que se quer.
//
// Aqui:
//
//   - banco fora do ar é 503, porque sem ele nem leitura existe;
//   - disco sem margem é 200 com `escrita: false`. O orquestrador mantém a
//     instância recebendo tráfego, quem lê continua lendo, e quem escreve
//     recebe um erro claro em vez de uma falha no meio do upload.
//
// O readiness usa o pool COM TETO de espera de propósito: sob saturação, um
// ping que fica dez segundos na fila faz a sonda expirar e a instância ser
// marcada como fora do ar por um problema de fila, não de banco. Com o teto, a
// resposta chega rápido e diz a verdade.
func ready(
	pool *repository.PoolComEspera,
	guarda *armazem.Guarda,
	entregador *despachante.Despachante,
	salas *hub.Hub,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancelar := context.WithTimeout(r.Context(), timeoutReady)
		defer cancelar()

		w.Header().Set("Content-Type", "application/json")

		if err := pool.Ping(ctx); err != nil {
			slog.Error("readiness: banco indisponível", slog.String("erro", err.Error()))
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "degradado", "banco": false, "escrita": false,
				"erro": "banco indisponível",
			})
			return
		}

		estadoDaEntrega := entregador.Estado()
		derrubados := salas.DerrubadosPorLentidao()

		podeEscrever, espaco, err := guarda.PodeEscrever(ctx)
		if err != nil {
			// Falha ao MEDIR não é falha de espaço: transformar "não consegui
			// olhar" em "recuso tudo" derrubaria o produto por causa do
			// instrumento.
			slog.Warn("readiness: não foi possível medir o disco", slog.String("erro", err.Error()))
		}

		resposta := map[string]any{
			"status": "ok", "banco": true, "escrita": podeEscrever,
			"discoLivreBytes":  espaco.LivreBytes,
			"discoLivrePorCem": espaco.LivrePorCem,
			// Sinais de DEGRADAÇÃO do tempo real. Não mudam o status — o
			// serviço está no ar e correto —, mas precisam ser visíveis de
			// fora: sem eles, "o quadro está lento" é um relato sem número.
			//
			// A8 transforma isto em métrica e alerta; aqui eles apenas deixam
			// de ser invisíveis.
			"tempoReal": map[string]any{
				"quadrosObservados":     estadoDaEntrega.QuadrosObservados,
				"atrasoMaximoRevisoes":  estadoDaEntrega.AtrasoMaximo,
				"derrubadosPorLentidao": derrubados,
			},
		}
		if !podeEscrever {
			resposta["status"] = "degradado"
			resposta["erro"] = "sem margem de disco: escrita suspensa"
			slog.Error("readiness: disco sem margem, escrita suspensa",
				slog.Uint64("livre_bytes", espaco.LivreBytes),
				slog.Float64("livre_por_cem", espaco.LivrePorCem))
		}
		json.NewEncoder(w).Encode(resposta)
	}
}

// origemSemEsquema devolve só o host da origem, que é o formato do
// OriginPatterns do coder/websocket — ele compara contra o host do cabeçalho
// Origin, não contra a URL inteira.
func origemSemEsquema(origem string) string {
	u, err := url.Parse(origem)
	if err != nil || u.Host == "" {
		return origem
	}
	return u.Host
}
