SHELL := /bin/bash

.PHONY: run test test-backend test-frontend test-e2e

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

# Testes de ponta a ponta: navegador de verdade contra a stack inteira.
#
# Exige `make run` noutro terminal. Não sobe a stack sozinho de propósito — ela
# são quatro serviços com dependências na ordem certa, e duplicar isso na
# configuração do Playwright criaria uma segunda forma de subir o projeto, que
# divergiria da primeira no dia em que alguém mexesse só numa.
test-e2e:
	@cd frontend && npx playwright test


# --- Infraestrutura (Ansible) ------------------------------------------------
#
# O servidor de produção descrito como código: usuário, Docker, diretórios,
# .env, compose, backup, cron e o bloco do Caddy. A ordem de uso é
#
#   make infra-segredos   uma vez, cria e cifra a senha do banco e o token
#   make infra-preparar   uma vez por MÁQUINA: Docker e usuário (exige root)
#   make infra-check      mostra o que MUDARIA, sem tocar em nada
#   make infra-apply      aplica
#
# Detalhes e o procedimento de recomeço estão em docs/infraestrutura.md.

PASTA_ANSIBLE := deploy/ansible

# A senha do vault entra por comando, e não pelo ansible.cfg: declarada lá, ela
# passaria a ser exigida por QUALQUER comando ansible neste diretório — até um
# `--syntax-check` —, e o CI, que valida o playbook sem ter direito aos segredos
# de produção, não conseguiria rodar. Ver o comentário no ansible.cfg.
SENHA_VAULT := --vault-password-file .senha-vault

.PHONY: infra-segredos infra-preparar infra-check infra-apply

# Falha cedo e com instrução, em vez de deixar o Ansible reclamar de um arquivo
# de senha ausente já com a conexão SSH aberta.
CONFERE_VAULT = @test -f $(PASTA_ANSIBLE)/segredos/producao.yml || { \
		echo "segredos ainda não criados — rode antes:  make infra-segredos"; \
		exit 1; }

# Dependência é ARQUIVO, não alvo: o make só reinstala as coleções quando o
# requirements.yml muda, então os alvos abaixo são baratos de repetir.
#
# O Ansible não vem no Ubuntu por padrão, e desde o 24.04 o PEP 668 bloqueia
# `pip install --user` — por isso a mensagem aponta o apt, e não o pip.
$(PASTA_ANSIBLE)/.dependencias: $(PASTA_ANSIBLE)/requirements.yml
	@command -v ansible-playbook >/dev/null || { \
		echo "ansible não encontrado. Instale com:"; \
		echo "    sudo apt install ansible"; \
		exit 1; }
	@cd $(PASTA_ANSIBLE) && ansible-galaxy collection install -r requirements.yml
	@touch $@

infra-segredos: $(PASTA_ANSIBLE)/.dependencias
	@$(PASTA_ANSIBLE)/segredos.sh

# Dia zero da MÁQUINA: instala o Docker e cria o usuário `deploy`. Roda uma vez
# por servidor, e é a única parte que exige root — por isso pede a credencial na
# hora, em vez de guardá-la. No VPS de hoje não tem o que fazer.
#
# Sem $(SENHA_VAULT), e é de propósito: este playbook não lê segredo nenhum, e um
# playbook que roda como ROOT é o último lugar onde vale abrir os segredos por
# hábito.
infra-preparar: $(PASTA_ANSIBLE)/.dependencias
	@cd $(PASTA_ANSIBLE) && ansible-playbook preparar-host.yml -u root --ask-pass --diff

# Passo obrigatório antes do apply. O critério de aceite desta fase é ele sair
# com `changed=0` numa SEGUNDA execução, depois que o servidor já foi montado —
# é o que prova que o playbook descreve o host, e não só o constrói uma vez.
infra-check: $(PASTA_ANSIBLE)/.dependencias
	$(CONFERE_VAULT)
	@cd $(PASTA_ANSIBLE) && ansible-playbook provisionar.yml $(SENHA_VAULT) --check --diff

infra-apply: $(PASTA_ANSIBLE)/.dependencias
	$(CONFERE_VAULT)
	@cd $(PASTA_ANSIBLE) && ansible-playbook provisionar.yml $(SENHA_VAULT) --diff
