#!/usr/bin/env bash
# Backup do banco e dos anexos. Roda no VPS, por cron.
#
#   sudo crontab -e
#   17 3 * * * /opt/stacktrack/scripts/backup.sh >> /var/log/stacktrack-backup.log 2>&1
#
# ⚠️ Backup guardado só na máquina que ele protege cobre erro de aplicação e
# migration ruim — NÃO cobre perder o VPS. Enquanto a cópia não sair daqui, o
# risco de perda total continua de pé. O gancho para mandar os arquivos ao
# Supabase Storage está no fim do arquivo, comentado.

set -euo pipefail

RAIZ="${RAIZ:-/opt/stacktrack}"
DESTINO="${DESTINO:-/var/backups/stacktrack}"
# Quantos dias de cópias manter. Além de espaço, é o que define até quando dá
# para voltar: um estrago percebido tarde precisa de uma cópia anterior a ele.
DIAS="${DIAS:-14}"

cd "$RAIZ"
# shellcheck disable=SC1091
source .env.prod

compose() {
	docker compose -f docker-compose.prod.yml --env-file .env.prod "$@"
}

mkdir -p "$DESTINO"
carimbo=$(date +%Y%m%d-%H%M%S)

# --- banco ------------------------------------------------------------------
# --clean --if-exists deixa o dump restaurável sobre um banco que já tem as
# tabelas, sem precisar recriá-lo à mão antes.
echo "[$(date -Is)] dump do postgres"
compose exec -T postgres pg_dump \
	--username "$POSTGRES_USER" \
	--dbname "$POSTGRES_DB" \
	--clean --if-exists \
	| gzip -9 > "$DESTINO/banco-$carimbo.sql.gz"

# --- anexos -----------------------------------------------------------------
# Os anexos são arquivos num volume, não linhas no banco: o pg_dump não os
# inclui. Sem esta parte, restaurar o banco devolveria cards apontando para
# anexos que não existem mais.
#
# A cópia sai por um container avulso montando o mesmo volume: a imagem da API
# roda como read_only e sem shell útil, e o container em produção não deve ser
# perturbado para tirar backup.
echo "[$(date -Is)] tar dos anexos"
docker run --rm \
	-v stacktrack_anexos:/dados:ro \
	-v "$DESTINO:/backup" \
	alpine:3.21 tar -czf "/backup/anexos-$carimbo.tar.gz" -C /dados .

# --- rotação ----------------------------------------------------------------
find "$DESTINO" -name 'banco-*.sql.gz'   -mtime "+$DIAS" -delete
find "$DESTINO" -name 'anexos-*.tar.gz'  -mtime "+$DIAS" -delete

# Um dump que ninguém nunca restaurou é uma esperança, não um backup. Teste a
# restauração num banco descartável de tempos em tempos:
#   gunzip -c banco-XXXX.sql.gz | docker compose -f docker-compose.prod.yml \
#     exec -T postgres psql -U "$POSTGRES_USER" -d stacktrack_teste
echo "[$(date -Is)] pronto — $(ls -1 "$DESTINO" | wc -l) arquivos em $DESTINO"

# --- cópia externa (próximo passo) ------------------------------------------
# O Supabase Storage fala S3. Com a CLI configurada, é uma linha:
#
#   aws s3 cp "$DESTINO/banco-$carimbo.sql.gz" \
#     "s3://$SUPABASE_BUCKET/stacktrack/" --endpoint-url "$SUPABASE_S3_ENDPOINT"
#
# Guarde as credenciais no .env.prod (chmod 600), nunca no git. E use uma chave
# com permissão só de escrita neste bucket: se o VPS for comprometido, quem
# entrar não pode apagar os backups junto.
