#!/usr/bin/env bash
# O que a chave da esteira consegue fazer, e o que ela NÃO consegue.
#
#   make infra-validar        (e o job `infra` da esteira roda o mesmo)
#
# O wrapper `/usr/local/bin/stacktrack-release` é a fronteira: quem tem a chave
# do CI tem exatamente o que este arquivo passa. Um `eval` distraído, um verbo a
# mais, uma validação de SHA frouxa — qualquer um dos três devolve a máquina, e
# a máquina é dividida com o agendaGo.
#
# O teste renderiza o template do Ansible com valores fixos e executa o script
# com um PATH de mentira: `docker`, `sudo`, `crontab` e o backup.sh viram stubs
# que só registram o que foram chamados a fazer. Nada toca em Docker de verdade.

set -euo pipefail

raiz=$(cd "$(dirname "$0")/.." && pwd)
template="$raiz/deploy/ansible/roles/acesso_esteira/templates/stacktrack-release.sh.j2"

trabalho=$(mktemp -d)
trap 'rm -rf "$trabalho"' EXIT

wrapper="$trabalho/bin/stacktrack-release"
mkdir -p "$trabalho/bin" "$trabalho/stack/scripts" "$trabalho/caddy"

# --- renderiza o template ----------------------------------------------------
#
# Substituição direta em vez de Jinja: as variáveis deste template são quatro
# strings simples, e depender do Ansible para rodar o teste transformaria uma
# checagem de dez segundos numa instalação.
# O delimitador do `s` é @ porque o que se procura contém `|` — o filtro
# `| quote` do Jinja.
sed \
	-e "s@{{ usuario_app | quote }}@'$(id -un)'@g" \
	-e "s@{{ pasta_stack | quote }}@'$trabalho/stack'@g" \
	-e "s@{{ pasta_scripts | quote }}@'$trabalho/stack/scripts'@g" \
	-e "s@{{ pasta_caddy_sites | quote }}@'$trabalho/caddy'@g" \
	-e "s@{{ rede_borda | quote }}@'borda'@g" \
	-e "s@{{ rede_borda }}@borda@g" \
	"$template" >"$wrapper"
chmod +x "$wrapper"

if grep -q '{{' "$wrapper"; then
	echo "FALHA: sobrou variável Jinja sem substituir no wrapper renderizado:"
	grep -n '{{' "$wrapper"
	exit 1
fi

# --- o host de mentira -------------------------------------------------------

cat >"$trabalho/bin/docker" <<'STUB'
#!/usr/bin/env bash
echo "docker $*" >>"$REGISTRO"
# `network inspect` precisa responder 0 para o pré-voo passar.
exit 0
STUB

cat >"$trabalho/bin/sudo" <<'STUB'
#!/usr/bin/env bash
echo "sudo $*" >>"$REGISTRO"
exit 0
STUB

cat >"$trabalho/bin/crontab" <<'STUB'
#!/usr/bin/env bash
echo "0 3 * * * /home/deploy/stacktrack/scripts/backup.sh"
STUB

cat >"$trabalho/stack/scripts/backup.sh" <<'STUB'
#!/usr/bin/env bash
echo "backup" >>"$REGISTRO"
STUB

chmod +x "$trabalho/bin/docker" "$trabalho/bin/sudo" "$trabalho/bin/crontab" \
	"$trabalho/stack/scripts/backup.sh"

# O que o pré-voo exige encontrar.
printf 'PROXIES_CONFIAVEIS=172.18.0.0/16\nDB_USER=stacktrack_app\n' >"$trabalho/stack/.env"
touch "$trabalho/stack/docker-compose.prod.yml" "$trabalho/caddy/stacktrack.caddy"

export PATH="$trabalho/bin:$PATH"
export REGISTRO="$trabalho/registro"

falhas=0

# roda executa o wrapper pelo caminho do SSH — comando forçado, que é como a
# esteira chega — e devolve o código de saída.
roda() {
	: >"$REGISTRO"
	SSH_ORIGINAL_COMMAND="$1" "$wrapper" >"$trabalho/saida" 2>&1
}

aceita() {
	local descricao="$1" comando="$2"
	if roda "$comando"; then
		echo "  ok   aceita: $descricao"
	else
		echo "  FALHA: '$comando' devia ser aceito e saiu $?:"
		sed 's/^/         /' "$trabalho/saida"
		falhas=$((falhas + 1))
	fi
}

recusa() {
	local descricao="$1" comando="$2"
	if roda "$comando"; then
		echo "  FALHA: '$comando' devia ser RECUSADO e foi aceito"
		falhas=$((falhas + 1))
	else
		echo "  ok   recusa: $descricao"
	fi
	# A recusa só vale se nada tiver acontecido antes dela.
	if [ -s "$REGISTRO" ] && grep -qE '^(docker (compose )?(pull|up|stop)|backup)' "$REGISTRO"; then
		echo "  FALHA: '$comando' foi recusado DEPOIS de mexer no servidor:"
		sed 's/^/         /' "$REGISTRO"
		falhas=$((falhas + 1))
	fi
}

echo "verbos aceitos"
aceita "release com SHA completo" "release 0123456789abcdef0123456789abcdef01234567"
aceita "release latest" "release latest"
aceita "backup" "backup"
aceita "estado" "estado"
aceita "o nome do wrapper antes do verbo" "stacktrack-release estado"

echo
echo "o que a chave NÃO alcança"
recusa "shell puro" "bash"
recusa "verbo inventado" "destruir"
recusa "comando encadeado por ponto e vírgula" "release latest; rm -rf /"
recusa "comando encadeado por &&" "estado && cat /etc/shadow"
recusa "substituição de comando" 'release $(cat /etc/passwd)'
recusa "crase" 'release `id`'
recusa "redirecionamento" "estado > /etc/passwd"
recusa "SHA com caractere fora do hexadecimal" "release ../../etc/passwd"
recusa "SHA curto demais" "release abc"
recusa "release sem referência" "release"
recusa "argumento a mais" "release latest extra"
recusa "backup com argumento" "backup agora"
recusa "linha vazia" ""

echo
echo "o release faz o que promete"
roda "release 0123456789abcdef0123456789abcdef01234567" || true
for esperado in "backup" "docker compose -f docker-compose.prod.yml pull" \
	"docker compose -f docker-compose.prod.yml stop web api" \
	"docker compose -f docker-compose.prod.yml up -d"; do
	if grep -qF "$esperado" "$REGISTRO"; then
		echo "  ok   $esperado"
	else
		echo "  FALHA: o release não executou: $esperado"
		falhas=$((falhas + 1))
	fi
done

echo
if [ "$falhas" -eq 0 ]; then
	echo "wrapper de release: tudo certo"
else
	echo "wrapper de release: $falhas falha(s)"
	exit 1
fi
