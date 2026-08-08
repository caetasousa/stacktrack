SHELL := /bin/bash

.PHONY: test test-fast test-tempo-real test-integracao

# Roda todos os testes e mostra, de forma legível, cada caso que passou ou
# falhou. Falha o alvo (exit != 0) se qualquer teste quebrar, para não mascarar
# erros.
#
# `-race`: a API serve requisições concorrentes, e corrida de dados não quebra o
# teste sem o detector — só aparece em produção sob carga. Da fase 5 em diante,
# quando o hub de WebSocket entra, este alvo deixa de ser opcional.
test:
	@set -o pipefail; go test -race -v ./... 2>&1 \
		| grep -E -- '--- (PASS|FAIL)' \
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
	@go test -race -tags tempo_real -v ./test/realtime/ 2>&1 \
		| grep -E -- '--- (PASS|FAIL)' \
		| sed -E 's/.*--- (PASS|FAIL): /\1 /; s/ \(.*\)//; s/_/ /g'

# Testes de repositório contra um PostgreSQL de verdade, subido pelo
# Testcontainers e derrubado no fim. Exige Docker, e por isso fica atrás de
# build tag: sem ela o `go test ./...` de quem não o tem passaria a falhar.
#
# É a camada que os fakes não alcançam — eles copiam a struct inteira, então um
# campo que o SQL não grava passa por eles sem reclamar. Foi assim que
# cards.prazo e boards.fundo existiram em todas as camadas sem serem gravados.
test-integracao:
	@go test -tags integracao -v ./test/repository/ 2>&1 \
		| grep -E -- '--- (PASS|FAIL|SKIP)' \
		| sed -E 's/.*--- (PASS|FAIL|SKIP): /\1 /; s/ \(.*\)//; s/_/ /g'
