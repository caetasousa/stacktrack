#!/usr/bin/env bash
# Backup do banco e dos anexos de produção. Roda por cron na VPS, como o
# usuário `stacktrack`:
#
#   20 3 * * * /home/stacktrack/scripts/backup.sh >> /home/stacktrack/backups/backup.log 2>&1
#
# Todo nome de arquivo é prefixado por `stacktrack-`, e todos os globs abaixo
# são escopados por esse prefixo: se um dia outra coisa gravar em `~/backups`,
# a rotação daqui não a enxerga nem a apaga.
#
# São dois artefatos: os anexos dos cards são arquivos num volume, e o pg_dump
# não os inclui. Restaurar só o banco devolveria cards apontando para anexos que
# não existem mais.

set -euo pipefail

# A stack mora na RAIZ do home do usuário do projeto — /home/stacktrack —, e não
# num subdiretório: a conta já é do stacktrack. Antes era $HOME/stacktrack,
# quando o dono era o `deploy`, que hospedava os dois projetos.
PASTA_STACK="${PASTA_STACK:-$HOME}"
PASTA_BACKUP="${PASTA_BACKUP:-$HOME/backups}"
RETENCAO_DIAS="${RETENCAO_DIAS:-7}"
ARQUIVO_COMPOSE="${ARQUIVO_COMPOSE:-$PASTA_STACK/docker-compose.prod.yml}"
# Nome do volume de anexos: o compose o prefixa com o `name:` do projeto.
VOLUME_ANEXOS="${VOLUME_ANEXOS:-stacktrack_anexos}"
# URL_HEARTBEAT (opcional): endereço pingado ao fim de uma execução bem
# sucedida, para um monitor externo alertar quando o ping parar de chegar.

log() { printf '%s  %s\n' "$(date '+%F %T')" "$*"; }

# O Compose lê o .env do diretório atual, não do diretório do arquivo de
# compose: sem este cd, as variáveis chegariam vazias.
cd "$PASTA_STACK"

set -a
# shellcheck disable=SC1091
. ./.env
set +a

compose() { docker compose -f "$ARQUIVO_COMPOSE" "$@"; }

# campo_manifesto lê um campo do manifesto de um backup anterior. sed em vez de
# jq porque a VPS não tem jq instalado, e o JSON aqui é gerado por este próprio
# script — formato fixo, sem aninhamento.
campo_manifesto() {
	[ -f "$1" ] || return 0
	sed -n "s/.*\"$2\": *\"\([^\"]*\)\".*/\1/p" "$1"
}

temporario=$(mktemp -d)
trap 'rm -rf "$temporario"' EXIT
bruto="$temporario/dump.sql"

mkdir -p "$PASTA_BACKUP"
log "iniciando dump do banco ${POSTGRES_DB}"

compose exec -T postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" >"$bruto"

# pg_dump encerra o arquivo com este marcador. Sem ele, o dump saiu truncado —
# e um backup truncado que parece válido é pior do que backup nenhum.
if ! tail -5 "$bruto" | grep -q "PostgreSQL database dump complete"; then
	log "ERRO: dump incompleto; nada foi gravado"
	exit 1
fi

# Hash do CONTEÚDO, para saber se o banco realmente mudou desde o último backup.
#
# As linhas \restrict/\unrestrict saem antes do cálculo: o pg_dump envolve o
# dump com um token ALEATÓRIO a cada execução (proteção contra injeção de
# meta-comandos do psql). Sem removê-las, dois dumps de um banco idêntico jamais
# teriam o mesmo hash.
hash=$(grep -v '^\\\(un\)\?restrict ' "$bruto" | sha256sum | cut -d' ' -f1)

# Versão do schema e da imagem que produziram este dump — é o que transforma o
# arquivo num ponto de restauração COMPLETO. Sem isso, descobrir a que versão do
# código um backup pertence exige restaurá-lo e ler o flyway_schema_history, e a
# hora dessa arqueologia costuma ser a hora do incidente.
schema_version=$(compose exec -T postgres psql -tAX -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
	-c "SELECT version FROM flyway_schema_history WHERE success ORDER BY installed_rank DESC LIMIT 1" \
	2>/dev/null | tr -d '[:space:]')
schema_version=${schema_version:-desconhecida}

# A imagem EM EXECUÇÃO é a verdade, não o .env: um deploy pode passar IMAGE_TAG
# por variável de ambiente sem tocar no arquivo, e aí o .env diria `latest` num
# servidor rodando um SHA fixo.
image_tag=desconhecida
container_api=$(compose ps -q api 2>/dev/null | head -1)
if [ -n "$container_api" ]; then
	image_tag=$(docker inspect --format '{{.Config.Image}}' "$container_api" 2>/dev/null | sed 's/.*://')
	image_tag=${image_tag:-desconhecida}
fi

anterior=$(find "$PASTA_BACKUP" -maxdepth 1 -name 'stacktrack-*.sql.gz' -printf '%T@ %p\n' 2>/dev/null |
	sort -rn | head -1 | cut -d' ' -f2-)

# Três condições, não uma. Dado idêntico sob um schema ou uma imagem NOVA merece
# ponto de restauração próprio: é justamente o retrato pré-migration que o
# rollback vai querer, e ele desapareceria se a deduplicação olhasse só o hash.
if [ -n "$anterior" ] &&
	[ "$hash" = "$(cat "$anterior.sha256" 2>/dev/null)" ] &&
	[ "$schema_version" = "$(campo_manifesto "$anterior.json" schema_version)" ] &&
	[ "$image_tag" = "$(campo_manifesto "$anterior.json" image_tag)" ]; then
	# O `touch` não é detalhe: é o que impede a rotação de apagar, dias depois,
	# o único backup existente. Num banco parado por mais tempo que a retenção,
	# sem isto você terminaria com zero backups.
	touch "$anterior" "$anterior.sha256"
	[ -f "$anterior.json" ] && touch "$anterior.json"
	log "sem alteração desde $(basename "$anterior"); nenhum backup novo criado"
else
	# Segundos no nome: sem eles, duas execuções no mesmo minuto (um backup
	# manual logo após o do cron) sobrescreveriam uma à outra. A versão do
	# schema entra no nome para ser legível sem abrir nada.
	arquivo="$PASTA_BACKUP/stacktrack-$(date +%F-%H%M%S)-schema$schema_version.sql.gz"
	# -n omite nome e timestamp do cabeçalho gzip, para o .gz ser reprodutível.
	gzip -9n <"$bruto" >"$arquivo.parcial"
	mv "$arquivo.parcial" "$arquivo"
	printf '%s\n' "$hash" >"$arquivo.sha256"
	cat >"$arquivo.json" <<JSON
{
  "arquivo": "$(basename "$arquivo")",
  "criado_em": "$(date -Is)",
  "schema_version": "$schema_version",
  "image_tag": "$image_tag",
  "sha256": "$hash"
}
JSON
	# O dump contém email e hash de senha de pessoas reais: leitura só do dono.
	chmod 600 "$arquivo" "$arquivo.sha256" "$arquivo.json"
	log "ok: $(du -h "$arquivo" | cut -f1) em $arquivo (schema $schema_version, imagem $image_tag)"
fi

# --- anexos -----------------------------------------------------------------
# Saem por um container avulso montando o volume: a imagem da API roda como
# read_only e sem shell, e o container em produção não deve ser perturbado só
# para tirar backup.
#
# --sort=name e --mtime fixo tornam o tar REPRODUTÍVEL: sem eles, a ordem de
# leitura do diretório mudaria o arquivo a cada execução e a deduplicação nunca
# acertaria. Anexo é imutável depois de enviado, então conteúdo igual tem de
# produzir bytes iguais.
#
# debian-slim, e não alpine: o tar do BusyBox não conhece nenhuma dessas opções.
# E a compressão sai por `gzip -n` num pipe, e não pelo -z do tar — o -z invoca
# o gzip SEM -n, que carimba a data no cabeçalho e faria cada execução gerar
# bytes diferentes para o mesmo conteúdo. As duas coisas quebrariam a
# deduplicação sem quebrar o backup, que é o tipo de defeito que só aparece na
# fatura do armazenamento.
log "empacotando anexos do volume $VOLUME_ANEXOS"
tar_anexos="$temporario/anexos.tar.gz"
docker run --rm -v "$VOLUME_ANEXOS:/dados:ro" debian:stable-slim \
	tar --sort=name --mtime='@0' --owner=0 --group=0 --numeric-owner \
	-cf - -C /dados . | gzip -9n >"$tar_anexos"

hash_anexos=$(sha256sum "$tar_anexos" | cut -d' ' -f1)
anterior_anexos=$(find "$PASTA_BACKUP" -maxdepth 1 -name 'stacktrack-anexos-*.tar.gz' -printf '%T@ %p\n' 2>/dev/null |
	sort -rn | head -1 | cut -d' ' -f2-)

if [ -n "$anterior_anexos" ] && [ "$hash_anexos" = "$(cat "$anterior_anexos.sha256" 2>/dev/null)" ]; then
	touch "$anterior_anexos" "$anterior_anexos.sha256"
	log "anexos sem alteração desde $(basename "$anterior_anexos")"
else
	arquivo_anexos="$PASTA_BACKUP/stacktrack-anexos-$(date +%F-%H%M%S).tar.gz"
	mv "$tar_anexos" "$arquivo_anexos"
	printf '%s\n' "$hash_anexos" >"$arquivo_anexos.sha256"
	chmod 600 "$arquivo_anexos" "$arquivo_anexos.sha256"
	log "ok: $(du -h "$arquivo_anexos" | cut -f1) em $arquivo_anexos"
fi

# --- rotação ----------------------------------------------------------------
# Todos os globs levam o prefixo do projeto: uma rotação sem prefixo apagaria
# qualquer outra coisa que um dia acabe em `~/backups`.
removidos=$(find "$PASTA_BACKUP" -maxdepth 1 \
	\( -name 'stacktrack-*.sql.gz' -o -name 'stacktrack-*.sql.gz.sha256' -o -name 'stacktrack-*.sql.gz.json' \
	-o -name 'stacktrack-anexos-*.tar.gz' -o -name 'stacktrack-anexos-*.tar.gz.sha256' \) \
	-mtime "+$RETENCAO_DIAS" -print -delete | wc -l)
if [ "$removidos" -gt 0 ]; then
	log "rotação: $removidos arquivo(s) com mais de $RETENCAO_DIAS dias removido(s)"
fi

# Heartbeat para um monitor externo (opcional, via URL_HEARTBEAT).
#
# O alerta nasce da AUSÊNCIA do ping, não da presença de um erro: um cron
# desativado, uma VPS desligada e um dump que falhou produzem o mesmo silêncio,
# e todos os três merecem alerta.
#
# Fica na última linha porque, com `set -e`, qualquer falha acima encerra o
# script antes de chegar aqui. Falhar o ping em si não reprova o backup — o
# arquivo já está gravado.
if [ -n "${URL_HEARTBEAT:-}" ]; then
	if curl -fsS --max-time 10 --retry 3 "$URL_HEARTBEAT" >/dev/null 2>&1; then
		log "heartbeat enviado"
	else
		log "AVISO: heartbeat não pôde ser enviado (o backup em si deu certo)"
	fi
fi
