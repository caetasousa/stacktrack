SHELL := /bin/bash

.PHONY: run test test-backend test-frontend

# Sobe a stack inteira: Postgres, migrations (Flyway), API, frontend e Mailpit.
# Roda em primeiro plano — os logs dos cinco serviços saem juntos e Ctrl+C
# encerra tudo (o que também exercita o desligamento gracioso da API).
#
# As dependências deste alvo são arquivos, não outros alvos: o make só as
# prepara quando faltam, então `make run` é seguro de repetir.
#
# O 130 tratado no fim é 128+SIGINT — o Ctrl+C que encerra a stack de propósito.
# Sem isso o make fecha toda sessão normal de desenvolvimento com um "Error 130"
# vermelho, que parece defeito e não é.
run: .env frontend/.env frontend/node_modules
	@echo "frontend  http://localhost:5173"
	@echo "api       http://localhost:8080"
	@echo "mailpit   http://localhost:8025"
	@echo
	@docker compose up; codigo=$$?; \
	if [ $$codigo -eq 130 ]; then exit 0; fi; \
	exit $$codigo


# Senha aleatória em vez do placeholder do exemplo: este .env vira as
# credenciais reais do Postgres na primeira subida, e o volume guarda a senha
# do initdb — trocá-la depois exige derrubar o banco com `docker compose down -v`.
.env:
	@senha=$$(openssl rand -hex 16); \
	test -n "$$senha" || { echo "openssl não encontrado — copie .env.example para .env à mão"; exit 1; }; \
	sed "s/troque-esta-senha/$$senha/" .env.example > .env; \
	echo "→ .env criado a partir do exemplo, com POSTGRES_PASSWORD aleatória"

frontend/.env:
	@cp frontend/.env.example frontend/.env
	@echo "→ frontend/.env criado a partir do exemplo"

# Instalar no host ANTES do compose não é preferência: o container `web` monta
# volumes próprios em node_modules e .svelte-kit (a instalação dele é Alpine/musl,
# a sua é glibc), e o Docker cria o ponto de montagem como ROOT quando o
# diretório ainda não existe — depois todo `npm install` seu falha com EACCES.
# Criando aqui primeiro, os dois nascem seus e o Docker só monta por cima.
frontend/node_modules: frontend/package.json frontend/package-lock.json
	@cd frontend && npm install
	@mkdir -p frontend/.svelte-kit
	@touch frontend/node_modules

# Roda os testes rápidos de backend e frontend (sem Docker, sem browsers).
# É o alvo padrão para checar o projeto antes de commitar.
test: test-backend test-frontend

# Testes rápidos do backend (domínio, usecases, handlers — em memória).
test-backend:
	@$(MAKE) -C backend test-fast

# Testes unitários do frontend (Vitest).
test-frontend:
	@cd frontend && npm run test:unit
