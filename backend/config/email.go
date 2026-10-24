package config

import (
	"crypto/rand"
	"os"
	"strings"
	"time"

	"stacktrack/internal/adapter/security"
)

// SMTP é como a API fala com o provedor de email (env SMTP_* e EMAIL_*).
type SMTP struct {
	Host            string
	Porta           int
	Usuario         string
	Senha           string
	STARTTLS        bool
	Remetente       string
	NomeDoRemetente string
}

// EmailLigado informa se o envio de email está configurado (env SMTP_HOST).
//
// Desligado é um estado legítimo, e não um erro: o convite nasce com o link
// manual e o resto do sistema não muda. Nenhum fluxo de quadro pode exigir
// email para funcionar. A recuperação de senha é a exceção que não tem como
// não ser: sem email ela responde como sempre, mas não manda nada.
func EmailLigado() bool {
	return strings.TrimSpace(os.Getenv("SMTP_HOST")) != ""
}

// ConfiguracaoSMTP lê a configuração do provedor.
//
// SMTP_STARTTLS vale true por padrão: desligá-lo é decisão explícita, que só
// faz sentido contra o Mailpit local — e config.Validar a recusa em produção.
func ConfiguracaoSMTP() SMTP {
	nome := strings.TrimSpace(os.Getenv("EMAIL_REMETENTE_NOME"))
	if nome == "" {
		nome = "stacktrack"
	}
	return SMTP{
		Host:            strings.TrimSpace(os.Getenv("SMTP_HOST")),
		Porta:           intDoAmbiente("SMTP_PORT", 587),
		Usuario:         os.Getenv("SMTP_USER"),
		Senha:           os.Getenv("SMTP_PASSWORD"),
		STARTTLS:        !strings.EqualFold(strings.TrimSpace(os.Getenv("SMTP_STARTTLS")), "false"),
		Remetente:       strings.TrimSpace(os.Getenv("EMAIL_REMETENTE")),
		NomeDoRemetente: nome,
	}
}

// ChaveDoEmail devolve a chave que cifra a mensagem do outbox (env
// EMAIL_CHAVE, base64 de 32 bytes: `openssl rand -base64 32`).
//
// Fora de produção, sem chave, devolve uma EFÊMERA e avisa por `efemera`: o
// desenvolvimento não precisa de vault, e o preço — a fila não sobrevive a um
// reinício — é aceitável ali. Em produção a chave é obrigatória (Validar).
func ChaveDoEmail() (chave []byte, efemera bool, err error) {
	texto := strings.TrimSpace(os.Getenv("EMAIL_CHAVE"))
	if texto == "" && !EhProducao() {
		chave = make([]byte, security.TamanhoDaChave)
		_, err = rand.Read(chave)
		return chave, true, err
	}
	chave, err = security.ChaveDeTexto(texto)
	return chave, false, err
}

// IntervaloDoCarteiro é de quanto em quanto tempo o worker olha a fila de
// email (env EMAIL_INTERVALO_MS). Cinco segundos: o caminho normal é o aviso de
// quem enfileira, e este é a rede que pega o que escapou — a retentativa
// marcada, o processo reiniciado.
func IntervaloDoCarteiro() time.Duration {
	return time.Duration(intDoAmbiente("EMAIL_INTERVALO_MS", 5000)) * time.Millisecond
}

// IntervaloEntreReenvios é o mínimo entre dois emails do mesmo convite (env
// CONVITE_INTERVALO_REENVIO_SEG, padrão 120). Zero desliga, e só a suíte E2E
// precisa disso; em produção Validar exige ao menos 30 segundos.
func IntervaloEntreReenvios() time.Duration {
	return time.Duration(intDoAmbiente("CONVITE_INTERVALO_REENVIO_SEG", 120)) * time.Second
}
