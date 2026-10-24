//go:build integracao

// O servidor inteiro por HTTP, com a MESMA montagem da produção — `montar`, em
// montagem.go: configuração lida do ambiente, pool, repositórios, casos de uso,
// rotas, middlewares e workers. O banco é um PostgreSQL efêmero do
// Testcontainers, com as migrations de verdade; o único dublê é o servidor
// SMTP, na fronteira de fora.
//
// Como isto quebra — cada teste cobre um item:
//
//	H1 a montagem esquece de ligar o correio, ou o worker: convidar responde 201
//	   e nenhum email sai;
//	H2 o link que chega no email não abre o convite pela rota pública, ou o
//	   aceite pela API não põe a pessoa no quadro;
//	H3 a resposta do convite — que carrega o token — sai sem no-store, e um
//	   cache no caminho a entrega a outra pessoa;
//	H4 sem SMTP configurado, o servidor não sobe, ou o convite volta sem link;
//	H5 reenviar cedo demais vira 500 em vez de 429 com a explicação;
//	H6 o quadro aceito não aparece na listagem de quem foi convidado, ou aparece
//	   sem dizer de quem é;
//	H7 a montagem não liga a confirmação de email ao cadastro, ou as rotas de
//	   "Convites para você" deixam passar conta sem email confirmado.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"stacktrack/test/smtpfalso"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var bancoDeTeste struct {
	host, porta, nome, usuario, senha string
	pool                              *pgxpool.Pool
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("stacktrack_servidor"),
		postgres.WithUsername("teste"),
		postgres.WithPassword("teste"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		panic("não foi possível subir o Postgres de teste: " + err.Error())
	}
	host, err := container.Host(ctx)
	if err != nil {
		panic(err)
	}
	porta, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		panic(err)
	}
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		panic(err)
	}
	if err := aplicarMigrations(ctx, pool); err != nil {
		panic("migrations: " + err.Error())
	}
	bancoDeTeste.host, bancoDeTeste.porta = host, porta.Port()
	bancoDeTeste.nome, bancoDeTeste.usuario, bancoDeTeste.senha = "stacktrack_servidor", "teste", "teste"
	bancoDeTeste.pool = pool

	codigo := m.Run()
	pool.Close()
	_ = container.Terminate(ctx)
	os.Exit(codigo)
}

// aplicarMigrations roda os mesmos .sql que o Flyway aplica, na ordem.
func aplicarMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	caminhos, err := filepath.Glob(filepath.Join("..", "..", "migrations", "V*.sql"))
	if err != nil {
		return err
	}
	versao := func(caminho string) int {
		nome := strings.TrimPrefix(filepath.Base(caminho), "V")
		n, _ := strconv.Atoi(nome[:strings.Index(nome, "__")])
		return n
	}
	sort.Slice(caminhos, func(i, j int) bool { return versao(caminhos[i]) < versao(caminhos[j]) })
	for _, caminho := range caminhos {
		sql, err := os.ReadFile(caminho)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(caminho), err)
		}
	}
	return nil
}

// subirServidor monta a aplicação exatamente como o main monta, a partir do
// ambiente, e a serve por HTTP. Com smtp nil, o ambiente fica sem SMTP — como
// um servidor em que o envio de email não foi configurado.
func subirServidor(t *testing.T, smtp *smtpfalso.Servidor) *httptest.Server {
	t.Helper()
	chave := make([]byte, 32)
	_, _ = rand.Read(chave)

	ambiente := map[string]string{
		"APP_ENV":         "test",
		"FRONTEND_ORIGIN": "http://localhost:5173",
		"DB_HOST":         bancoDeTeste.host,
		"DB_PORT":         bancoDeTeste.porta,
		"DB_NAME":         bancoDeTeste.nome,
		"DB_USER":         bancoDeTeste.usuario,
		"DB_PASSWORD":     bancoDeTeste.senha,
		"ANEXOS_DIR":      t.TempDir(),
		// Os tetos por IP ficam frouxos: o teste inteiro sai do mesmo endereço.
		"RATE_LIMIT_CADASTRO_POR_MINUTO":    "1000",
		"RATE_LIMIT_LOGIN_POR_MINUTO":       "1000",
		"RATE_LIMIT_AUTENTICADO_POR_MINUTO": "10000",
		"RATE_LIMIT_PUBLICO_POR_MINUTO":     "1000",
		"RATE_LIMIT_SESSAO_DESCONHECIDA":    "1000",
		// O intervalo de produção, para o H5 poder esbarrar nele.
		"CONVITE_INTERVALO_REENVIO_SEG": "120",
		"EMAIL_INTERVALO_MS":            "200",
	}
	if smtp != nil {
		host, porta := smtp.Endereco()
		ambiente["SMTP_HOST"] = host
		ambiente["SMTP_PORT"] = strconv.Itoa(porta)
		ambiente["SMTP_STARTTLS"] = "false"
		ambiente["EMAIL_REMETENTE"] = "convites@stacktrack.teste"
		ambiente["EMAIL_REMETENTE_NOME"] = "stacktrack"
		ambiente["EMAIL_CHAVE"] = base64.StdEncoding.EncodeToString(chave)
	} else {
		for _, nome := range []string{"SMTP_HOST", "SMTP_PORT", "EMAIL_REMETENTE", "EMAIL_CHAVE"} {
			ambiente[nome] = ""
		}
	}
	for nome, valor := range ambiente {
		t.Setenv(nome, valor)
	}

	ctx, cancelar := context.WithCancel(context.Background())
	app, err := montar(ctx)
	if err != nil {
		cancelar()
		t.Fatalf("montar a aplicação: %v", err)
	}
	go app.rodarTrabalhos(ctx)
	srv := httptest.NewServer(app.roteador)
	t.Cleanup(func() {
		srv.Close()
		cancelar()
		app.fechar()
	})
	return srv
}

// pessoa é um cliente HTTP com a própria sessão.
type pessoa struct {
	t       *testing.T
	base    string
	cliente *http.Client
}

func novaPessoa(t *testing.T, srv *httptest.Server) *pessoa {
	jarra, _ := cookiejar.New(nil)
	return &pessoa{t: t, base: srv.URL, cliente: &http.Client{Jar: jarra, Timeout: 10 * time.Second}}
}

// pedir faz a requisição e devolve o status, o corpo decodificado e os cabeçalhos.
func (p *pessoa) pedir(metodo, caminho string, corpo any) (int, map[string]any, http.Header) {
	p.t.Helper()
	var leitor io.Reader
	if corpo != nil {
		b, _ := json.Marshal(corpo)
		leitor = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, p.base+caminho, leitor)
	if err != nil {
		p.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	// A API confere a origem das escritas: o navegador de verdade sempre manda.
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := p.cliente.Do(req)
	if err != nil {
		p.t.Fatalf("%s %s: %v", metodo, caminho, err)
	}
	defer resp.Body.Close() //nolint:errcheck // o corpo já foi lido inteiro
	bruto, _ := io.ReadAll(resp.Body)
	var decodificado map[string]any
	_ = json.Unmarshal(bruto, &decodificado)
	return resp.StatusCode, decodificado, resp.Header
}

func (p *pessoa) cadastrar(nome, email string) {
	p.t.Helper()
	status, corpo, _ := p.pedir(http.MethodPost, "/auth/cadastro",
		map[string]string{"nome": nome, "email": email, "senha": "uma frase longa de teste para " + nome})
	if status != http.StatusCreated {
		p.t.Fatalf("cadastro de %s: %d %v", nome, status, corpo)
	}
}

func emailUnico(nome string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%x@teste.dev", nome, b)
}

var tokenNoLink = regexp.MustCompile(`http://localhost:5173/convite/([A-Za-z0-9_-]+)`)

// emailsPara são as mensagens que chegaram para o endereço. Filtra porque o
// cadastro também manda email — o de confirmação da conta —, e contar "o
// primeiro email que chegou" leria o da pessoa errada.
func emailsPara(smtp *smtpfalso.Servidor, para string) []smtpfalso.Recebida {
	var delas []smtpfalso.Recebida
	for _, r := range smtp.Recebidas() {
		if len(r.Para) > 0 && r.Para[0] == para {
			delas = append(delas, r)
		}
	}
	return delas
}

// esperarEmailPara aguarda a primeira mensagem para o endereço.
func esperarEmailPara(t *testing.T, smtp *smtpfalso.Servidor, para string) smtpfalso.Recebida {
	t.Helper()
	fim := time.Now().Add(10 * time.Second)
	for time.Now().Before(fim) {
		if delas := emailsPara(smtp, para); len(delas) > 0 {
			return delas[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("nenhum email chegou para %s", para)
	return smtpfalso.Recebida{}
}

// H1, H2, H3 e H6
func TestConvitePorEmailDeUmaPontaAOutra(t *testing.T) {
	smtp := smtpfalso.Iniciar(t)
	srv := subirServidor(t, smtp)

	ana := novaPessoa(t, srv)
	ana.cadastrar("Ana", emailUnico("ana"))
	status, quadro, _ := ana.pedir(http.MethodPost, "/boards", map[string]string{"titulo": "Lançamento do app"})
	if status != http.StatusCreated {
		t.Fatalf("criar quadro: %d %v", status, quadro)
	}
	boardID := quadro["id"].(string)

	emailDaBia := emailUnico("bia")
	status, convite, cabecalhos := ana.pedir(http.MethodPost, "/boards/"+boardID+"/membros",
		map[string]string{"email": emailDaBia, "papel": "editor"})
	if status != http.StatusCreated {
		t.Fatalf("convidar: %d %v", status, convite)
	}
	if convite["entrega"] != "email" {
		t.Fatalf("entrega = %v, esperado email", convite["entrega"])
	}
	// H3: a resposta carrega o link — e o link é credencial.
	if cc := cabecalhos.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control da resposta do convite = %q, esperado no-store", cc)
	}

	// H1: o worker da montagem de produção entrega sem ninguém o chamar.
	lida := esperarEmailPara(t, smtp, emailDaBia).Ler(t)
	achado := tokenNoLink.FindStringSubmatch(lida.Texto)
	if achado == nil {
		t.Fatalf("o email não traz o link do convite:\n%s", lida.Texto)
	}
	token := achado[1]

	// H2: o link abre o convite sem sessão, e o aceite põe a Bia no quadro.
	bia := novaPessoa(t, srv)
	status, detalhe, _ := bia.pedir(http.MethodGet, "/convites/"+token, nil)
	if status != http.StatusOK || detalhe["quadro"] != "Lançamento do app" {
		t.Fatalf("detalhe do convite pelo link do email: %d %v", status, detalhe)
	}
	bia.cadastrar("Bia", emailDaBia)
	status, aceito, _ := bia.pedir(http.MethodPost, "/convites/"+token+"/aceitar", map[string]string{})
	if status != http.StatusOK || aceito["id"] != boardID {
		t.Fatalf("aceitar: %d %v", status, aceito)
	}

	// H6: na listagem da Bia o quadro é dela para editar, e é da Ana.
	status, lista, _ := bia.pedir(http.MethodGet, "/boards", nil)
	if status != http.StatusOK {
		t.Fatalf("listar quadros: %d %v", status, lista)
	}
	quadros := lista["boards"].([]any)
	if len(quadros) != 1 {
		t.Fatalf("quadros da Bia = %v", quadros)
	}
	q := quadros[0].(map[string]any)
	if q["papel"] != "editor" || q["dono"] != "Ana" {
		t.Fatalf("quadro compartilhado = %v, esperado papel editor e dono Ana", q)
	}
}

// H4
func TestSemSMTPOServidorSobeEOConviteVoltaComLink(t *testing.T) {
	srv := subirServidor(t, nil)

	ana := novaPessoa(t, srv)
	ana.cadastrar("Ana", emailUnico("ana"))
	_, quadro, _ := ana.pedir(http.MethodPost, "/boards", map[string]string{"titulo": "Sem email"})

	status, convite, _ := ana.pedir(http.MethodPost, "/boards/"+quadro["id"].(string)+"/membros",
		map[string]string{"email": emailUnico("bia"), "papel": "leitor"})
	if status != http.StatusCreated {
		t.Fatalf("convidar sem SMTP: %d %v", status, convite)
	}
	if convite["entrega"] != "manual" || !tokenNoLink.MatchString(fmt.Sprint(convite["link"])) {
		t.Fatalf("convite sem SMTP = %v, esperado entrega manual com link", convite)
	}
}

// H5
func TestReenviarCedoDemaisExplicaEmVezDeQuebrar(t *testing.T) {
	smtp := smtpfalso.Iniciar(t)
	srv := subirServidor(t, smtp)

	ana := novaPessoa(t, srv)
	ana.cadastrar("Ana", emailUnico("ana"))
	_, quadro, _ := ana.pedir(http.MethodPost, "/boards", map[string]string{"titulo": "Reenvio"})
	boardID := quadro["id"].(string)
	emailDaBia := emailUnico("bia")
	_, convite, _ := ana.pedir(http.MethodPost, "/boards/"+boardID+"/membros",
		map[string]string{"email": emailDaBia, "papel": "editor"})
	conviteID := convite["convite"].(map[string]any)["id"].(string)
	esperarEmailPara(t, smtp, emailDaBia)

	status, corpo, _ := ana.pedir(http.MethodPost,
		"/boards/"+boardID+"/convites/"+conviteID+"/reenviar", map[string]string{})
	if status != http.StatusTooManyRequests {
		t.Fatalf("reenviar logo depois do envio: %d %v, esperado 429", status, corpo)
	}
	if msg, _ := corpo["erro"].(string); !strings.Contains(msg, "espere") {
		t.Errorf("mensagem do 429 = %q, esperado explicar a espera", msg)
	}
	if n := len(emailsPara(smtp, emailDaBia)); n != 1 {
		t.Fatalf("%d emails para a Bia, esperado só o do convite", n)
	}
}

var tokenDeConfirmacao = regexp.MustCompile(`http://localhost:5173/confirmar-email/([A-Za-z0-9_-]+)`)

// H7
func TestConvitesNaContaSoDepoisDeConfirmarOEmail(t *testing.T) {
	smtp := smtpfalso.Iniciar(t)
	srv := subirServidor(t, smtp)

	ana := novaPessoa(t, srv)
	ana.cadastrar("Ana", emailUnico("ana"))
	_, quadro, _ := ana.pedir(http.MethodPost, "/boards", map[string]string{"titulo": "Na conta"})

	emailDaBia := emailUnico("bia")
	bia := novaPessoa(t, srv)
	bia.cadastrar("Bia", emailDaBia)
	ana.pedir(http.MethodPost, "/boards/"+quadro["id"].(string)+"/membros",
		map[string]string{"email": emailDaBia, "papel": "editor"})

	if status, corpo, _ := bia.pedir(http.MethodGet, "/meus-convites", nil); status != http.StatusForbidden {
		t.Fatalf("conta sem email confirmado listou os convites: %d %v", status, corpo)
	}

	// O cadastro mandou a confirmação; o link a confirma sem sessão.
	confirmacao := esperarEmailPara(t, smtp, emailDaBia)
	achado := tokenDeConfirmacao.FindStringSubmatch(confirmacao.Ler(t).Texto)
	if achado == nil {
		t.Fatalf("o primeiro email da Bia não é a confirmação:\n%s", confirmacao.Ler(t).Texto)
	}
	anonimo := novaPessoa(t, srv)
	if status, corpo, _ := anonimo.pedir(http.MethodPost, "/auth/confirmacao/"+achado[1], nil); status != http.StatusNoContent {
		t.Fatalf("confirmar pelo link: %d %v", status, corpo)
	}

	status, corpo, _ := bia.pedir(http.MethodGet, "/meus-convites", nil)
	if status != http.StatusOK {
		t.Fatalf("listar depois de confirmar: %d %v", status, corpo)
	}
	convites := corpo["convites"].([]any)
	if len(convites) != 1 || convites[0].(map[string]any)["quadro"] != "Na conta" {
		t.Fatalf("convites da Bia = %v", convites)
	}
	id := convites[0].(map[string]any)["id"].(string)
	if status, aceito, _ := bia.pedir(http.MethodPost, "/meus-convites/"+id+"/aceitar", map[string]string{}); status != http.StatusOK || aceito["id"] != quadro["id"] {
		t.Fatalf("aceitar pelo painel: %d %v", status, aceito)
	}
}
