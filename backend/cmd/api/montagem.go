// A montagem da aplicação: configuração, banco, repositórios, casos de uso,
// rotas e workers. Mora fora do main para que o teste de servidor
// (servidor_test.go) suba EXATAMENTE a mesma montagem da produção, contra um
// banco efêmero — e não uma versão parecida, escrita à mão, que divergiria da
// real no dia em que alguém mexesse só numa.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"stacktrack/config"
	"stacktrack/internal/adapter/armazem"
	aemail "stacktrack/internal/adapter/email"
	"stacktrack/internal/adapter/http/handler"
	"stacktrack/internal/adapter/http/middleware"
	"stacktrack/internal/adapter/repository"
	"stacktrack/internal/adapter/security"
	"stacktrack/internal/pkg/logging"
	ucauth "stacktrack/internal/usecase/auth"
	ucboard "stacktrack/internal/usecase/board"
	"stacktrack/internal/usecase/correio"
	"stacktrack/internal/usecase/manutencao"

	"stacktrack/internal/adapter/http/ws"
	"stacktrack/internal/adapter/realtime/despachante"
	"stacktrack/internal/adapter/realtime/hub"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// aplicacao é o servidor montado: o roteador HTTP e o que roda em segundo
// plano. Quem a sobe decide quando começar os trabalhos e quando fechar.
type aplicacao struct {
	roteador *chi.Mux
	sala     *hub.Hub
	pool     *pgxpool.Pool
	// trabalhos são os laços de segundo plano — despachante de eventos, faxina,
	// worker de email —, todos parando quando o contexto acaba.
	trabalhos []func(context.Context)
}

// rodarTrabalhos dispara os laços de segundo plano e volta na hora.
func (a *aplicacao) rodarTrabalhos(ctx context.Context) {
	for _, trabalho := range a.trabalhos {
		go trabalho(ctx)
	}
}

// fecharTempoReal derruba as conexões de WebSocket, antes do Shutdown HTTP.
func (a *aplicacao) fecharTempoReal() {
	a.sala.Fechar()
}

// fechar devolve as conexões do banco. Por último, depois de tudo o que as usa.
func (a *aplicacao) fechar() {
	a.pool.Close()
}

// montar monta a aplicação a partir do ambiente. Não abre porta nem dispara
// goroutine: isso é de quem a sobe.
func montar(ctx context.Context) (*aplicacao, error) {
	proxiesConfiaveis, err := config.ProxiesConfiaveis()
	if err != nil {
		return nil, fmt.Errorf("PROXIES_CONFIAVEIS inválida: %w", err)
	}

	pool, err := config.NovoPool(context.Background())
	if err != nil {
		return nil, fmt.Errorf("conectar no banco: %w", err)
	}
	app := &aplicacao{pool: pool}

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
		pool.Close()
		return nil, fmt.Errorf("preparar o diretório de anexos: %w", err)
	}

	// segurança
	hasher := security.NovoHasherArgon2id()

	// usecases
	cadastrarUC := ucauth.NovoCadastrarUseCase(usuarioRepo, sessionRepo, hasher)
	// Conta e sessão no mesmo commit: sem isto, uma falha entre as duas
	// escritas deixava conta criada sem sessão, e a segunda tentativa de
	// cadastro batia no email já em uso.
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
	app.trabalhos = append(app.trabalhos, entregador.Rodar)
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

	// O envio de email dos convites. Ligado só com SMTP configurado; sem ele, o
	// convite nasce com o link manual e nada mais muda — nenhum fluxo de quadro
	// depende de email para funcionar.
	membroUC.ComIntervaloDeReenvio(config.IntervaloEntreReenvios())
	emailRepo, err := ligarCorreio(app, banco, unidadeDeTrabalho, membroUC)
	if err != nil {
		pool.Close()
		return nil, err
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
				r.Post("/convites/{conviteID}/reenviar", membroHandler.ReenviarConvite)

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

	faxineiros := []manutencao.Faxineiro{sessionRepo, conviteRepo, coletor}
	if emailRepo != nil {
		// A mensagem que venceu sem sair tem o link dentro: a faxina a apaga.
		faxineiros = append(faxineiros, emailRepo)
	}
	faxina := manutencao.Nova(
		config.IntervaloDaFaxina(), config.PrazoDaFaxina(), slog.Default(), faxineiros...,
	)
	app.trabalhos = append(app.trabalhos, faxina.Rodar)

	app.roteador = r
	app.sala = salaDeEventos
	return app, nil
}

// ligarCorreio liga o envio de email dos convites, se o SMTP estiver
// configurado. Devolve o outbox (nil com o envio desligado), que a faxina
// também precisa.
func ligarCorreio(
	app *aplicacao,
	banco repository.Fonte,
	unidade *repository.UnidadeDeTrabalho,
	membros *ucboard.MembroUseCase,
) (*repository.EmailPostgres, error) {
	if !config.EmailLigado() {
		slog.Warn("envio de email desligado: SMTP_HOST vazio — os convites saem só com o link manual")
		return nil, nil
	}

	chave, efemera, err := config.ChaveDoEmail()
	if err != nil {
		return nil, fmt.Errorf("EMAIL_CHAVE inválida: %w", err)
	}
	if efemera {
		slog.Warn("EMAIL_CHAVE vazia: usando uma chave efêmera, e as mensagens na fila não sobrevivem a um reinício (só fora de produção)")
	}
	cifra, err := security.NovaCifra(chave)
	if err != nil {
		return nil, fmt.Errorf("cifra do outbox de email: %w", err)
	}

	emails := repository.NovoEmailPostgres(banco, cifra)
	unidade.ComCifraDeEmail(cifra)

	smtp := config.ConfiguracaoSMTP()
	carteiro := correio.NovoCarteiro(emails, aemail.NovoEnviadorSMTP(aemail.ConfigSMTP{
		Host: smtp.Host, Porta: smtp.Porta, Usuario: smtp.Usuario, Senha: smtp.Senha,
		STARTTLS: smtp.STARTTLS, Remetente: smtp.Remetente, NomeDoRemetente: smtp.NomeDoRemetente,
	}), slog.Default()).ComIntervalo(config.IntervaloDoCarteiro())

	membros.ComCorreio(emails, aemail.NovoRedator(), config.OrigemFrontend(), carteiro.Acordar)
	app.trabalhos = append(app.trabalhos, carteiro.Rodar)

	slog.Info("envio de email ligado",
		slog.String("servidor", smtp.Host), slog.Int("porta", smtp.Porta), slog.Bool("starttls", smtp.STARTTLS))
	return emails, nil
}
