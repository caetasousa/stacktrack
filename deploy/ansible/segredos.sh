#!/usr/bin/env bash
# Gera, UMA vez, os segredos de produção do stacktrack e os deixa cifrados em
# segredos/producao.yml.
#
#   make infra-segredos
#
# A senha do Postgres nasce aqui, aleatória: ninguém precisa digitá-la nem
# lê-la. Ela vira as credenciais do initdb na primeira subida da stack e passa a
# existir em dois lugares apenas — este vault e o .env que o playbook escreve no
# servidor. É o que faz o `.env` deixar de ser um arquivo que alguém escreve à
# mão, que era o passo manual que sobrava no roteiro de produção.
#
# Para RECRIAR os segredos, apague o arquivo cifrado à mão — o script se recusa a
# sobrescrever. E note que recriar DEPOIS que a stack subiu não troca a senha do
# papel no Postgres: o initdb já a gravou no volume, e mudá-la exige ALTER ROLE.
# A role `stacktrack` tem um assert que recusa essa divergência em vez de deixar
# a API falhar a conexão no próximo deploy.

set -euo pipefail

# Os caminhos do ansible.cfg são relativos a deploy/ansible/.
cd "$(dirname "$0")"

# FORA de group_vars/: lá o Ansible decifraria este arquivo ao montar as
# variáveis de qualquer playbook do grupo — e até um `--syntax-check` passaria a
# exigir a senha. Quem precisa dele é o `vars_files` de provisionar.yml.
vault=segredos/producao.yml
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
	echo "  GUARDE UMA CÓPIA FORA DESTA MÁQUINA: sem ela o vault não abre mais."
fi

# As credenciais SMTP do provedor de email (Brevo: SMTP & API → SMTP). Vazias,
# o envio fica desligado e os convites saem só com o link manual — dá para
# preencher depois com `ansible-vault edit`.
#
# O `|| true` cobre a execução sem terminal (um pipe, um script): `read`
# devolve 1 no EOF e o `set -e` abortaria o script antes de gravar coisa alguma.
printf 'usuário SMTP do provedor de email (vazio desliga o envio): '
read -r smtp_usuario || true
smtp_usuario=${smtp_usuario:-}
printf 'senha SMTP do provedor: '
read -r -s smtp_senha || true
smtp_senha=${smtp_senha:-}
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
#   cd deploy/ansible
#   ansible-vault edit --vault-password-file .senha-vault segredos/producao.yml
#
# POSTGRES_DB, USER e PASSWORD são gravados pelo initdb no volume do Postgres na
# primeira subida da stack. Mudá-los aqui depois NÃO muda o papel no banco: só
# quebra a conexão da API. A role \`stacktrack\` tem um assert que recusa isso.
vault_postgres_db: 'stacktrack'
vault_postgres_user: 'stacktrack'
vault_postgres_password: '$(openssl rand -hex 24)'

# O envio de email dos convites. A chave cifra a fila de emails (AES-256): ela
# nasce aqui, aleatória, e ninguém precisa conhecê-la.
vault_smtp_usuario: '$smtp_usuario'
vault_smtp_senha: '$smtp_senha'
vault_email_chave: '$(openssl rand -base64 32)'
YAML

mkdir -p "$(dirname "$vault")"
# A senha vai no comando: o ansible.cfg não a declara, senão todo comando neste
# diretório passaria a exigi-la — inclusive o `make infra-validar`, que roda
# sem ela.
ansible-vault encrypt --vault-password-file "$senha_vault" --output "$vault" "$temporario"
echo "→ segredos cifrados em deploy/ansible/$vault"
echo
echo "Próximo passo:  make infra-check"
