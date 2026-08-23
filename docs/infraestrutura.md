# 🧱 Infraestrutura como código

O servidor descrito em Ansible, em `deploy/ansible/`. O que antes era um roteiro
de comandos para copiar à mão em `producao.md` agora é um playbook que se pode
rodar quantas vezes quiser.

A esteira que leva o commit até o ar continua sendo o GitHub Actions
([entrega-continua.md](entrega-continua.md)); o Ansible cuida do **servidor**,
não da versão que roda nele.

Este documento é **operacional** — como rodar, o que fazer quando dá errado. Se
o que você quer é entender *como o Ansible funciona* (o modelo sem agente, o que
viaja pelo SSH, por que `changed=0` é o teste que importa), o aprofundamento está
em [tecnologias.md](tecnologias.md#ansible--a-infraestrutura-por-dentro).

---

## 🎯 O problema que isto resolve

O `.env` de produção — com a senha do Postgres — nascia de um heredoc copiado à
mão. Era o único passo que impedia o servidor de ser remontado sozinho, e o
único lugar onde a senha existia. Perder o arquivo era perder o banco.

Hoje ele é **consequência de um comando**. Ninguém digita a senha, e ninguém
precisa saber qual é.

---

## 🗺️ Dois playbooks, dois ciclos de vida

| | Quando roda | Como conecta | O que faz |
|---|---|---|---|
| `preparar-host.yml` | quando o que é do **host** muda | `root` | Docker, rede `borda`, usuário `deploy`, acesso da esteira, hardening |
| `provisionar.yml` | sempre que quiser | `deploy`, **sem sudo** | diretórios, `.env`, compose, `backup.sh`, cron, bloco do Caddy, papéis do banco, stack no ar |

A divisória não é estética. `provisionar.yml` não precisa de privilégio nenhum:
tudo mora no home do `deploy` e o acesso ao Docker vem do grupo. Só o outro
exige root — e é justamente o que o CI não consegue fazer, nem deve: a chave da
esteira está presa a um comando forçado que executa três verbos de release.

O `preparar-host.yml` deixou de ser "dia zero". Ele é quem mantém o usuário da
esteira, o wrapper, o `sudo` restrito e o hardening do host, então volta a rodar
quando qualquer um desses muda. Rodá-lo duas vezes seguidas deve terminar em
`changed=0` — que é a prova de que ele descreve a máquina, em vez de só saber
construí-la.

### ⚠️ Ele aperta o caminho de conserto

O hardening desliga a senha do SSH e sobe o firewall. As duas coisas cortam a
via por onde se conserta um erro delas, então a role recusa aplicar em vez de
trancar a porta:

- **SSH** — só desliga a senha depois de conferir que `deploy` tem pelo menos
  uma chave em `authorized_keys`. A configuração vai num drop-in em
  `/etc/ssh/sshd_config.d/`, validada por `sshd -t` antes de existir, e o
  handler faz `reload` (não `restart`), que não derruba a sessão em uso.
- **firewall** — antes de habilitar, lista o que escuta em endereço público e
  **falha** se encontrar porta fora de `ufw_portas`. É assim que "conferir o que
  o agendaGo expõe" deixou de ser um item de checklist e virou uma falha de
  playbook, com a lista de portas na mensagem.

Ainda assim: tenha o console do provedor testado antes de rodar.

E note o alcance. O hardening é a única parte do repositório que mexe no HOST
INTEIRO — SSH, firewall e atualização automática valem para o agendaGo também,
não só para o stacktrack. Por isso ele tem tag própria e pode ficar para depois:

```bash
cd deploy/ansible
# tudo menos o hardening
ansible-playbook preparar-host.yml -u root --ask-pass --skip-tags hardening
# só ele, quando for a hora
ansible-playbook preparar-host.yml -u root --ask-pass --tags hardening
```

O que ele NÃO faz continua valendo: não abre o `Caddyfile` do vizinho, não
reescreve o crontab, não remove a rede `borda` nem `~/backups`. E o `apt-mark
hold` do Docker protege os dois — é o que impede um `apt upgrade` de reiniciar o
daemon e derrubar os containers do agendaGo junto.

### 🔑 Dois usuários, dois poderes

| | `deploy` | `stacktrack-deploy` |
|---|---|---|
| Quem usa | o operador, e o `provisionar.yml` | a esteira |
| Shell | sim | sim, mas a chave tem `restrict` + comando forçado |
| Grupo `docker` | sim — equivale a root nesta máquina | **não** |
| Alcança o Docker | direto | só pelo wrapper, por um `sudo` restrito a ele |
| Chave | a mesma do agendaGo, do operador | exclusiva do stacktrack |

O `deploy` é compartilhado com o agendaGo e continua como está — apertá-lo
mexeria no vizinho. O que mudou é que ele deixou de ser o caminho da esteira.

O wrapper (`/usr/local/bin/stacktrack-release`, root:root 0755) entende
`release <sha>`, `backup` e `estado`, e recusa o resto. Ele lê
`SSH_ORIGINAL_COMMAND` com `read -ra`, nunca `eval`: a linha do cliente é dado,
não código. `scripts/testa-wrapper-de-release.sh` executa o script renderizado
contra um `docker` de mentira e exige recusa para shell, encadeamento,
substituição de comando e SHA malformado — e roda no CI.

---

## 🚀 Uso

```bash
sudo apt install ansible          # Ubuntu 24.04: o PEP 668 bloqueia o pip

make infra-segredos               # UMA vez: gera e cifra a senha do banco
make infra-preparar               # uma vez por MÁQUINA nova (pede a senha de root)
make infra-check                  # mostra o que mudaria, sem tocar em nada
make infra-apply                  # aplica
```

`make infra-check` é o passo que não se pula. Ele imprime o diff de cada arquivo
e a lista do que mudaria; ler isso antes de aplicar é o que separa
infraestrutura como código de um script que ninguém revisa.

---

## 🔐 Os segredos

`make infra-segredos` gera `deploy/ansible/segredos/producao.yml`, cifrado com
`ansible-vault`, contendo o nome do banco, o usuário, a senha (aleatória, de
`openssl rand -hex 24`) e o token do GHCR.

**Ele fica fora de `group_vars/` de propósito.** Lá dentro, o Ansible o
decifraria ao montar as variáveis de qualquer playbook que apontasse para o
grupo `producao` — inclusive o `preparar-host.yml`, que não usa segredo nenhum,
e inclusive um `--syntax-check`, que passava a exigir a senha. Quem o carrega
hoje é uma task `include_vars` no `provisionar.yml`: ela só abre o arquivo
quando a play roda de verdade. É o que permite ao CI validar o playbook sem
receber a chave dos segredos de produção.

Pela mesma razão o `ansible.cfg` **não** declara `vault_password_file`: ali ela
valeria para todo comando ansible do diretório, e um arquivo de senha ausente
derruba o comando na partida. Quem precisa dela a passa explicitamente — os
alvos `infra-*` do Makefile já o fazem.

A **senha do vault** vai para `deploy/ansible/.senha-vault`, que está no
`.gitignore`. Ela é o que decifra os segredos de produção:

> **Guarde uma cópia fora desta máquina.** Sem ela o vault não abre mais,
> e a senha do banco existe apenas dentro do volume do Postgres — de onde não
> sai.

Para ver ou editar:

```bash
cd deploy/ansible
ansible-vault view --vault-password-file .senha-vault segredos/producao.yml
ansible-vault edit --vault-password-file .senha-vault segredos/producao.yml
```

### O token do GHCR pode ficar vazio

As imagens nascem privadas, mas podem ser tornadas públicas em
Packages → Package settings. Hoje as três são públicas, então o vault tem
`vault_ghcr_token: ''` e a task de `docker login` é pulada. Se um dia elas
voltarem a ser privadas, basta um token com `read:packages` no vault.

### Trocar a senha do banco não é editar o vault

`POSTGRES_DB`, `POSTGRES_USER` e `POSTGRES_PASSWORD` são gravados pelo `initdb`
**no volume do Postgres**, na primeira subida da stack. Mudá-los no vault depois
não muda o papel dentro do banco: só faz o `.env` mentir, e a API para de
conectar no deploy seguinte.

A role tem um `assert` que recusa exatamente isso, com a instrução no texto do
erro. Trocar a senha de verdade exige `ALTER ROLE` no banco **e** o vault
atualizado, nessa ordem.

---

## 🤝 A fronteira com o agendaGo

O VPS é compartilhado, e o playbook nunca escreve fora do que é dele:

| | Como |
|---|---|
| **Caddy** | deposita em `~/caddy/sites/stacktrack.caddy`; nunca abre o `Caddyfile` do vizinho, que é dono das portas 80/443 |
| **crontab** | `ansible.builtin.cron` com `name:` gerencia só o bloco marcado — a linha do agendaGo fica intacta, por desenho e não por sorte |
| **rede `borda`** | garantida se faltar, **nunca** removida |
| **Docker** | instalado só quando falta e **travado com `apt-mark hold`**: `apt upgrade` do `docker-ce` reinicia o daemon e derruba os containers do vizinho junto. Atualizar exige a tag `docker-upgrade`, que destrava, atualiza e trava de novo |
| **`unattended-upgrades`** | só o repositório de segurança, sem reboot automático, com os pacotes do Docker na lista negra |
| **`~/backups`** | comum aos dois; a separação é o prefixo do nome do arquivo |

O crontab tem **um dono só**: o Ansible. A esteira apenas confere que a entrada
existe e falha se não existir. Enquanto os dois escreviam, brigavam: o
`grep -v 'stacktrack/scripts/backup.sh'` do CI removia a linha do job sem
remover o comentário `#Ansible:`, o marcador ficava órfão, e o `infra-check`
seguinte acusava `changed` — reescrevia, e o deploy seguinte reescrevia de
volta. Um `changed=0` que nunca estabiliza não prova nada.

A variável `BACKUP_CRON` do GitHub deixou de ser lida; o horário agora sai de
`backup_hora` e `backup_minuto` em `group_vars/producao/vars.yml`.

---

## ♻️ Recomeçar do zero

O procedimento que apagou a stack para o playbook poder reconstruí-la. Note que
`rm -rf ~/stacktrack` **sozinho não apaga os dados** — o banco e os anexos moram
nos volumes `stacktrack_postgres_data` e `stacktrack_anexos`, e sem o `-v` eles
sobrevivem. A stack nova subiria com uma senha que não bate com a do `initdb`.

```bash
cd /home/deploy/stacktrack
docker compose -f docker-compose.prod.yml down -v   # o -v é o que apaga os volumes
rm -rf /home/deploy/stacktrack
rm -f  /home/deploy/caddy/sites/stacktrack.caddy
crontab -u deploy -l | grep -v stacktrack | crontab -u deploy -
docker exec $(docker ps --filter label=com.docker.compose.service=caddy -q | head -1) \
  caddy reload --config /etc/caddy/Caddyfile
```

⚠️ **Isto apaga contas, quadros, comentários e anexos.** Tire um backup antes
(`~/stacktrack/scripts/backup.sh`) e leve o arquivo para fora do VPS — a cópia
remota ainda não existe, ver [producao.md](producao.md).

Três comandos que parecem faxina e derrubam o vizinho:

- `docker network rm borda` — o Caddy do agendaGo perde a rota
- `docker system prune -a` — apaga as imagens dele junto
- `rm -rf ~/backups` — é pasta comum aos dois projetos

Não há alvo de `make` para isto de propósito: um `make infra-destruir` seria um
footgun morando no repositório para sempre.

---

## 🧪 Como isto é verificado

O critério de aceite é a **reconstrução**, e ele não cabe num teste automatizado:

1. Teardown acima, incluindo o `down -v`
2. `make infra-apply` — o servidor vazio vira aplicação no ar
3. `make infra-apply` **de novo** → `changed=0`
4. Criar conta, montar um quadro e dar F5 (exercita o cookie `__Host-`, que
   nenhum `curl` alcança)
5. `curl -sI https://<dominio-do-agendago>` — o vizinho continua no ar
6. `crontab -u deploy -l` — as **duas** linhas de backup

### O CI valida o playbook — sem a senha do vault

O job `infra` da esteira roda `--syntax-check` nos dois playbooks e
`ansible-lint` no diretório, e o `publicar-imagens` depende dele: playbook que
não passa não vira release.

Isso não era possível enquanto o arquivo cifrado morava em `group_vars/`. Lá o
Ansible o carregava ao montar as variáveis do grupo, e o comando morria antes de
qualquer análise:

```
ERROR! The vault password file .senha-vault was not found
```

A saída não foi dar a senha ao CI — seria trocar um problema de validação por um
de raio de explosão. Foi tirar o segredo do carregamento automático: ele entra
por `include_vars` no `provisionar.yml`, que só executa numa play de verdade. O
job confere inclusive que `.senha-vault` **não** existe no runner, para o
critério não passar por acidente um dia.

O mesmo alvo roda na sua máquina, antes do push:

```bash
make infra-validar
```

O que continua fora do CI é o que exige o servidor: `--check` e `--diff` contra o
host, e a prova de idempotência (`changed=0` na segunda aplicação). Esses
seguem locais, pelo `make infra-check`.

---

## 🧭 O que continua fora

- **Criar a máquina** — é Terraform, não Ansible.
- **Fuso do host** — o `date` do servidor continua como o provedor entregou; o
  que depende dele (o cron do backup) imprime os dois no `estado`.
- **DNS / DuckDNS** — o registro é externo, ver [producao.md](producao.md) §1.
- **Cópia de backup para fora do VPS** — pendência antiga, entra junto com a
  chave de escrita no bucket.

---

## 📚 Para estudar

📚 [Ansible — documentação](https://docs.ansible.com/ansible/latest/) · [playbooks](https://docs.ansible.com/ansible/latest/playbook_guide/index.html) · [roles](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_reuse_roles.html)
📚 [ansible-vault](https://docs.ansible.com/ansible/latest/vault_guide/index.html) — como o segredo cifrado vive no repositório
📝 [Idempotência em Ansible](https://docs.ansible.com/ansible/latest/reference_appendices/glossary.html#term-Idempotency) — por que `changed=0` na segunda execução é o teste que importa
📚 [community.docker](https://docs.ansible.com/ansible/latest/collections/community/docker/) — `docker_compose_v2` chama a CLI e dispensa o SDK Python no servidor
