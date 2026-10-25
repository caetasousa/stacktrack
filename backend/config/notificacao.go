// Configuração das notificações: a espera do email, o vigia de prazos e os
// tetos do stream de tempo real (SSE).

package config

import (
	"time"

	"stacktrack/internal/domain/notificacao"
)

// EsperaDoEmailDeNotificacao é quanto o email de uma notificação espera antes
// de sair (env NOTIFICACAO_ESPERA_EMAIL_SEG, em segundos).
//
// É a janela em que ler no sistema cancela o email: quem está com o stacktrack
// aberto vê o sino e não recebe o mesmo aviso de novo na caixa de entrada.
// Zero manda na hora — é o que o desenvolvimento usa, para o email chegar ao
// Mailpit sem esperar.
func EsperaDoEmailDeNotificacao() time.Duration {
	return time.Duration(intDoAmbiente("NOTIFICACAO_ESPERA_EMAIL_SEG",
		int(notificacao.EsperaDoEmail/time.Second))) * time.Second
}

// IntervaloDoVigiaDePrazos é de quanto em quanto tempo o vigia procura prazos
// chegando (env PRAZOS_INTERVALO_SEG, em segundos).
//
// Dez minutos: o aviso é da véspera, e dez minutos de atraso nele não mudam
// nada. O vigia também é acordado na hora quando um prazo é marcado ou alguém
// passa a responder por um card — o intervalo só cobre a passagem do tempo.
func IntervaloDoVigiaDePrazos() time.Duration {
	return time.Duration(intDoAmbiente("PRAZOS_INTERVALO_SEG", 600)) * time.Second
}

// SSEConexoesPorConta é quantos streams de notificação uma conta pode ter
// abertos ao mesmo tempo (env SSE_CONEXOES_POR_CONTA; 0 desliga).
//
// Dez, o dobro do WebSocket: o stream fica aberto em TODA página logada, e não
// só no quadro, então cada aba conta uma vez. Um stream custa uma goroutine e
// nenhuma fila — bem menos que uma conexão de quadro.
func SSEConexoesPorConta() int {
	return intDoAmbiente("SSE_CONEXOES_POR_CONTA", 10)
}

// SSEConexoesSimultaneas é o teto global de streams do processo
// (env SSE_CONEXOES_SIMULTANEAS; 0 desliga).
func SSEConexoesSimultaneas() int {
	return intDoAmbiente("SSE_CONEXOES_SIMULTANEAS", 200)
}

// SSEAberturasPorMinuto é o teto de aberturas do stream por IP
// (env SSE_ABERTURAS_POR_MINUTO; 0 desliga).
//
// Mais folgado que o handshake do WebSocket: cada página carregada de novo (F5,
// link aberto noutra aba) abre um stream, e um escritório inteiro sai pelo
// mesmo IP.
func SSEAberturasPorMinuto() int {
	return intDoAmbiente("SSE_ABERTURAS_POR_MINUTO", 60)
}
