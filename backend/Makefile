SHELL := /bin/bash

.PHONY: test test-fast test-tempo-real test-integracao

# Roda todos os testes e mostra, de forma legível, cada caso que passou ou
# falhou — e, quando falha, o motivo (as linhas `arquivo.go:NN:` do teste) e o
# erro de compilação. Falha o alvo (exit != 0) se qualquer teste quebrar, para
# não mascarar erros.
#
# `-race`: a API serve requisições concorrentes, e corrida de dados não quebra o
# teste sem o detector — só aparece em produção sob carga. Da fase 5 em diante,
# quando o hub de WebSocket entra, este alvo deixa de ser opcional.
test:
	@set -o pipefail; go test -race -v ./... 2>&1 \
		| grep -E -- '--- (PASS|FAIL)|^(FAIL|panic:|#)|\.go:[0-9]+:' \
		| sed -E 's/.*--- (PASS|FAIL): /\1 /; s/ \(.*\)//; s@/@ › @; s/_/ /g'

# Roda só os testes rápidos (em memória, sem Docker).
test-fast:
	@go test ./...

# O teste de duas abas da fase 5: exige a API NO AR, porque exercita o
# handshake de verdade — cookie de sessão real, checagem de origem, evento
# atravessando o hub. Fica atrás de build tag para não quebrar o `go test ./...`
# de quem não subiu a stack.
#
#   make run              # noutro terminal, na raiz do repositório
#   make -C backend test-tempo-real
test-tempo-real:
	@set -o pipefail; go test -race -tags tempo_real -v ./test/realtime/ 2>&1 \
		| grep -E -- '--- (PASS|FAIL)|^(FAIL|panic:|#)|\.go:[0-9]+:' \
		| sed -E 's/.*--- (PASS|FAIL): /\1 /; s/ \(.*\)//; s/_/ /g'

# Testes de integração contra um PostgreSQL de verdade, subido pelo
# Testcontainers e derrubado no fim: os casos de uso ligados aos repositórios
# reais (test/repository) e o servidor inteiro por HTTP, com a montagem da
# produção (cmd/api). Exige Docker, e por isso fica atrás de build tag: sem ela
# o `go test ./...` de quem não o tem passaria a falhar.
#
# É a camada que os fakes não alcançam — eles copiam a struct inteira, então um
# campo que o SQL não grava passa por eles sem reclamar. Foi assim que
# cards.prazo e boards.fundo existiram em todas as camadas sem serem gravados.
#
# `pipefail` nos três alvos com filtro: sem ele, o código de saída é o do `sed`,
# e uma suíte vermelha terminava o `make` com sucesso. A esteira chamava o `go
# test` direto e nunca viu isso; à mão, era um verde falso.
test-integracao:
	@set -o pipefail; go test -tags integracao -v ./test/repository/ ./cmd/api/ 2>&1 \
		| grep -E -- '--- (PASS|FAIL|SKIP)|^(FAIL|panic:|#)|\.go:[0-9]+:' \
		| sed -E 's/.*--- (PASS|FAIL|SKIP): /\1 /; s/ \(.*\)//; s/_/ /g'
