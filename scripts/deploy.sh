#!/usr/bin/env bash
# Publica e implanta em produção o commit atual, num comando só:
#
#   make deploy                      # verifica, publica as imagens e implanta
#   PULAR_VERIFICAR=1 make deploy    # o mesmo, sem o `make verificar`
#
# É o roteiro de docs/entrega-continua.md, na ordem: pré-voo, `make verificar`,
# login no GHCR, build e push das três imagens com a tag no SHA, `make
# infra-apply` (idempotente: sem mudança, não mexe em nada), o release pela
# chave de deploy e a conferência de que a versão nova está no ar.
#
# Não faz `git push`: o commit precisa já estar no origin/main — produção nunca
# roda um commit que não está no GitHub.
set -euo pipefail
cd "$(dirname "$0")/.."

repo=ghcr.io/caetasousa
usuario_ghcr=caetasousa
chave_release=~/.ssh/stacktrack_release
release=(ssh -o BatchMode=yes -o ConnectTimeout=15 -i "$chave_release" -p 2222 stacktrack@127.0.0.1)
dominio=$(awk '/^dominio:/ {print $2}' deploy/ansible/group_vars/producao/vars.yml)

passo() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
falha() {
	printf '\033[31merro:\033[0m %s\n' "$*" >&2
	exit 1
}

# --- pré-voo: falha antes de qualquer coisa sair desta máquina --------------
passo "pré-voo"
[ -z "$(git status --porcelain)" ] || falha "há alterações não commitadas — produção só recebe o que está commitado"
sha=$(git rev-parse HEAD)
git fetch -q origin
git merge-base --is-ancestor "$sha" origin/main ||
	falha "o commit ${sha:0:7} não está no origin/main — faça o push antes do deploy"
[ -f "$chave_release" ] || falha "chave de deploy ausente em $chave_release"
[ -f deploy/ansible/.senha-vault ] || falha "senha do vault ausente em deploy/ansible/.senha-vault"
echo "commit ${sha:0:7}: $(git log -1 --format=%s)"

if [ "${PULAR_VERIFICAR:-}" != 1 ]; then
	passo "make verificar"
	make --no-print-directory verificar
fi

# --- login no GHCR ------------------------------------------------------------
# Usa o login que já existir; senão o GHCR_TOKEN do ambiente; senão o token do
# GitHub que o git guarda para o push — precisa de `write:packages`. Quando é
# este script que entra, ele também sai no fim: o token não fica gravado no
# ~/.docker/config.json.
if ! grep -q '"ghcr.io"' ~/.docker/config.json 2>/dev/null; then
	passo "login no GHCR"
	token=${GHCR_TOKEN:-$(printf 'protocol=https\nhost=github.com\n\n' |
		GIT_TERMINAL_PROMPT=0 git credential fill 2>/dev/null | sed -n 's/^password=//p')}
	[ -n "$token" ] || falha "sem credencial para o GHCR: exporte GHCR_TOKEN (write:packages) ou rode docker login ghcr.io"
	printf '%s' "$token" | docker login ghcr.io -u "$usuario_ghcr" --password-stdin >/dev/null
	unset token
	trap 'docker logout ghcr.io >/dev/null 2>&1 || true' EXIT
fi

# --- imagens ------------------------------------------------------------------
passo "build das imagens ${sha:0:7}"
docker build -t "$repo/stacktrack-api:$sha" -t "$repo/stacktrack-api:latest" \
	-f backend/Dockerfile.prod backend
docker build -t "$repo/stacktrack-migrations:$sha" -t "$repo/stacktrack-migrations:latest" \
	-f backend/Dockerfile.migrations backend
docker build -t "$repo/stacktrack-web:$sha" -t "$repo/stacktrack-web:latest" \
	-f frontend/Dockerfile.prod --build-arg PUBLIC_API_URL=/api frontend

passo "publicação no GHCR"
# Tag por tag, e não `--all-tags`: este republicaria qualquer tag velha que
# estivesse no disco desta máquina.
for imagem in stacktrack-api stacktrack-migrations stacktrack-web; do
	docker push -q "$repo/$imagem:$sha"
	docker push -q "$repo/$imagem:latest"
done

# --- servidor -----------------------------------------------------------------
passo "configuração do servidor (make infra-apply)"
make --no-print-directory infra-apply

passo "release ${sha:0:7}"
"${release[@]}" "stacktrack-release release $sha"

passo "conferência"
"${release[@]}" "stacktrack-release estado" | grep -q "stacktrack-api:$sha" ||
	falha "a API no ar não é a ${sha:0:7} — veja: make estado"
curl -fsS --max-time 15 "https://$dominio/api/health" >/dev/null ||
	falha "https://$dominio/api/health não respondeu"
printf '\n\033[32mno ar:\033[0m %s em https://%s\n' "${sha:0:7}" "$dominio"
