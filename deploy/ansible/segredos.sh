#!/usr/bin/env bash
# Gera, UMA vez, os segredos de produção do stacktrack e os deixa cifrados em
# group_vars/producao/vault.yml.
#
#   make infra-segredos
#
# A senha do Postgres nasce aqui, aleatória: ninguém precisa digitá-la nem
# lê-la. Ela vira as credenciais do initdb na primeira subida da stack e passa a
# existir em dois lugares apenas — este vault e o .env que o playbook escreve no
# servidor. É o que faz o `.env` deixar de ser um arquivo que alguém escreve à
# mão, que era o passo manual que sobrava no roteiro de produção.
#
# Para RECRIAR os segredos, apague o vault.yml à mão — o script se recusa a
# sobrescrever. E note que recriar DEPOIS que a stack subiu não troca a senha do
# papel no Postgres: o initdb já a gravou no volume, e mudá-la exige ALTER ROLE.
# A role `stacktrack` tem um assert que recusa essa divergência em vez de deixar
# a API falhar a conexão no próximo deploy.

set -euo pipefail

# Os caminhos do ansible.cfg são relativos a deploy/ansible/, e é de lá que o
# ansible-vault lê a senha (vault_password_file).
cd "$(dirname "$0")"

vault=group_vars/producao/vault.yml
senha_vault=.senha-vault

if [ -f "$vault" ]; then
	echo "ERRO: $vault já existe."
	echo "Apague-o à mão se realmente quiser recriar os segredos de produção."
	exit 1
fi

# A senha do vault é o que protege o arquivo cifrado ao lado dela; por isso mora
# fora do git (ver .gitignore).
if [ ! -f "$senha_vault" ]; then
	openssl rand -base64 32 >"$senha_vault"
	chmod 600 "$senha_vault"
	echo "→ senha do vault criada em deploy/ansible/$senha_vault (fora do git)"
	echo "  GUARDE UMA CÓPIA FORA DESTA MÁQUINA: sem ela o vault.yml não abre mais."
fi

# As imagens do GHCR nascem privadas. Sem token, o servidor só as puxa se os
# pacotes tiverem sido tornados públicos — que é uma escolha válida, e por isso
# o campo aceita vazio.
# O `|| true` cobre a execução sem terminal (um pipe, o CI): `read` devolve 1 no
# EOF e o `set -e` abortaria o script antes de gravar coisa alguma.
printf 'token do GHCR com read:packages (vazio se as imagens forem públicas): '
read -r -s token_ghcr || true
token_ghcr=${token_ghcr:-}
echo

# mktemp com 600 e trap: o arquivo passa alguns milissegundos em claro no disco,
# e nesse intervalo ele contém a senha do banco de produção.
temporario=$(mktemp)
chmod 600 "$temporario"
trap 'rm -f "$temporario"' EXIT

cat >"$temporario" <<YAML
---
# Segredos de produção do stacktrack. Cifrado com ansible-vault — nunca editar
# direto no disco:
#
#   cd deploy/ansible && ansible-vault edit group_vars/producao/vault.yml
#
# POSTGRES_DB, USER e PASSWORD são gravados pelo initdb no volume do Postgres na
# primeira subida da stack. Mudá-los aqui depois NÃO muda o papel no banco: só
# quebra a conexão da API. A role \`stacktrack\` tem um assert que recusa isso.
vault_postgres_db: 'stacktrack'
vault_postgres_user: 'stacktrack'
vault_postgres_password: '$(openssl rand -hex 24)'
vault_ghcr_token: '$token_ghcr'
YAML

ansible-vault encrypt --output "$vault" "$temporario"
echo "→ segredos cifrados em deploy/ansible/$vault"
echo
echo "Próximo passo:  make infra-check"
