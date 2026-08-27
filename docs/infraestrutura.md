# 🧱 Infraestrutura como código

A **aplicação** descrita em Ansible, em `deploy/ansible/`. O que antes era um
roteiro de comandos para copiar à mão em `producao.md` agora é um playbook que
se pode rodar quantas vezes quiser.

O **host** — Docker, rede `borda`, firewall, hardening de SSH e o Traefik da
borda que termina o TLS — é do projeto
[`loadbalancer`](https://github.com/caetasousa/loadbalancer), repo à parte. Este
repositório pressupõe esse host provisionado e descreve só o que roda em cima:
o usuário de deploy, o `.env`, o compose (com as labels do Traefik), o backup,
o cron e os papéis do banco.

A esteira que leva o commit até o ar continua sendo o GitHub Actions
([entrega-continua.md](entrega-continua.md)); o Ansible cuida da máquina, não da
versão que roda nela.

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
| `preparar-host.yml` | quando a **identidade de deploy** muda | `stacktrack` + `sudo` | o usuário `stacktrack`, o `sudoers` restrito, a chave do operador, a chave da esteira, o wrapper `stacktrack-release` |
| `provisionar.yml` | sempre que quiser | `stacktrack`, **sem sudo** | diretórios, `.env`, compose, `backup.sh`, cron, papéis do banco, stack na `borda` |

A divisória não é estética. `provisionar.yml` não precisa de privilégio nenhum:
tudo mora no home do `stacktrack` e o acesso ao Docker vem do grupo. Só o outro
exige privilégio — e é justamente o que o CI não consegue fazer, nem deve: a
chave da esteira está presa a um comando forçado que executa três verbos de
release.

O `preparar-host.yml` entra como `stacktrack` e sobe por `sudo`, em vez de logar
como root: o `loadbalancer` aplica `PermitRootLogin no`, e um playbook que só
soubesse entrar como root deixaria de rodar. Numa máquina em que `stacktrack`
ainda não consiga elevar, entre por outra conta com sudo:

```bash
cd deploy/ansible
ansible-playbook preparar-host.yml -e ansible_user=<conta-com-sudo> --ask-become-pass --diff
```

`-e ansible_user=...`, e não `-u ...`: o `-u` troca só o usuário da conexão e
deixa a variável valendo `stacktrack`, o que mantém o `become` pedindo a senha
da conta errada. O `-e` tem a precedência mais alta e resolve os dois de uma vez.

**Por que o sudo do `stacktrack` é sem senha.** A conta nasce com a senha
bloqueada (`passwd -S` responde `L`), que é o certo para conta de serviço —
senha de conta de serviço é senha que alguém guarda num arquivo. Sem senha,
`sudo` com senha nunca funciona, e o playbook ficaria preso ao root para sempre.

E não é privilégio novo: `stacktrack` está no grupo `docker`, o que nesta
máquina já equivale a root (`docker run -v /:/host` monta o disco inteiro). O
sudoers só torna explícito o que o grupo concede. O usuário da **esteira** é
outro assunto: o acesso dele é restrito a um arquivo, e é ali que o privilégio
mínimo importa.

Rodar `preparar-host.yml` duas vezes seguidas deve terminar em `changed=0` — que
é a prova de que ele descreve a máquina, em vez de só saber construí-la.

### 🏠 A casa do stacktrack

```
/home/stacktrack/   .env · docker-compose.prod.yml · scripts/ · backups/
/opt/loadbalancer/  a borda (projeto à parte) — Traefik + socket-proxy
```

O roteamento de `stacktrack.caetasousa.tech` (split `/api` → API, resto → front,
cabeçalhos, TLS) está nas **labels do Traefik no `docker-compose.prod.yml`** — o
Traefik as lê pelo Docker, nada é depositado na borda. O contrato está em
[producao.md](producao.md#o-que-o-loadbalancer-roteia-para-o-stacktrack).

### 🔑 Uma conta, e uma chave que não vale o que a conta vale

A esteira **não tem conta própria**: ela entra no `stacktrack`, e o que a limita
é a chave, não o usuário.

```
authorized_keys do stacktrack
├── chave do operador                                    → shell normal
└── restrict,command="/usr/local/bin/stacktrack-release" → três verbos
```

`command=` faz o sshd executar aquele programa e ignorar o que o cliente pediu;
o pedido vai para `SSH_ORIGINAL_COMMAND`, como dado. `restrict` desliga PTY,
encaminhamento de porta, de agente e X11.

**O que se abriu mão, para não haver conta separada.** Se um dia essa linha
perder as opções, a chave da esteira passa a dar shell numa conta do grupo
`docker` — que nesta máquina é root. Uma conta separada faria o mesmo erro
terminar num shell sem poder nenhum: falharia fechado, e esta escolha falha
aberta. A mitigação é o `authorized_keys` ser escrito pelo playbook, e não à
mão.

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
make infra-preparar               # o que exige privilégio (pede a senha do sudo)
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

## 🤝 A fronteira com o `loadbalancer`

O VPS é compartilhado com a borda Traefik e com os outros sites dela. O playbook
do stacktrack nunca escreve fora do que é dele:

| | Como |
|---|---|
| **roteamento** | nas labels do Traefik em `docker-compose.prod.yml`; nenhum arquivo de config depositado na borda, nenhum reload disparado |
| **crontab** | `ansible.builtin.cron` com `name:` gerencia só o bloco marcado — as outras linhas ficam intactas, por desenho e não por sorte |
| **rede `borda`** | criada pelo `loadbalancer`; o compose só a declara `external`, **nunca** a remove |
| **Docker / firewall / hardening** | do `loadbalancer` — este repo pressupõe tudo isso pronto |

O crontab tem **um dono só**: o Ansible. A esteira apenas confere que a entrada
existe e falha se não existir. Enquanto os dois escreviam, brigavam: o
`grep -v 'stacktrack/scripts/backup.sh'` do CI removia a linha do job sem
remover o comentário `#Ansible:`, o marcador ficava órfão, e o `infra-check`
seguinte acusava `changed` — reescrevia, e o deploy seguinte reescrevia de
volta. Um `changed=0` que nunca estabiliza não prova nada.

---

## ♻️ Recomeçar do zero

O procedimento que apagou a stack para o playbook poder reconstruí-la. Note que
`rm -rf ~/stacktrack` **sozinho não apaga os dados** — o banco e os anexos moram
nos volumes `stacktrack_postgres_data` e `stacktrack_anexos`, e sem o `-v` eles
sobrevivem. A stack nova subiria com uma senha que não bate com a do `initdb`.

```bash
cd /home/stacktrack
docker compose -f docker-compose.prod.yml down -v   # o -v é o que apaga os volumes
sudo rm -rf /home/stacktrack
crontab -u stacktrack -l | grep -v stacktrack | crontab -u stacktrack -
```

⚠️ **Isto apaga contas, quadros, comentários e anexos.** Tire um backup antes
(`~/stacktrack/scripts/backup.sh`) e leve o arquivo para fora do VPS — a cópia
remota ainda não existe, ver [producao.md](producao.md).

Três comandos que parecem faxina e derrubam a borda e os vizinhos:

- `docker network rm borda` — o Traefik da borda perde a rota para todos os apps
- `docker system prune -a` — apaga as imagens deles junto
- `rm -rf ~/backups` — pode conter cópias de outros serviços do VPS

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
5. `curl -sI https://<outro-site-da-borda>` — os vizinhos continuam no ar
6. `crontab -u stacktrack -l` — a linha do backup do stacktrack

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

- **O host e a borda** — Docker, rede `borda`, firewall, hardening, o Traefik e
  o TLS são do projeto [`loadbalancer`](https://github.com/caetasousa/loadbalancer).
- **O roteamento** está nas labels do `docker-compose.prod.yml`; o contrato está em
  [producao.md](producao.md#o-que-o-loadbalancer-roteia-para-o-stacktrack).
- **Criar a máquina** — é Terraform, não Ansible.
- **Fuso do host** — o `date` do servidor continua como o provedor entregou; o
  que depende dele (o cron do backup) imprime os dois no `estado`.
- **DNS** — o registro `A` de `stacktrack.caetasousa.tech` é externo, ver
  [producao.md](producao.md) §1.
- **Cópia de backup para fora do VPS** — pendência antiga, entra junto com a
  chave de escrita no bucket.

---

## 📚 Para estudar

📚 [Ansible — documentação](https://docs.ansible.com/ansible/latest/) · [playbooks](https://docs.ansible.com/ansible/latest/playbook_guide/index.html) · [roles](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_reuse_roles.html)
📚 [ansible-vault](https://docs.ansible.com/ansible/latest/vault_guide/index.html) — como o segredo cifrado vive no repositório
📝 [Idempotência em Ansible](https://docs.ansible.com/ansible/latest/reference_appendices/glossary.html#term-Idempotency) — por que `changed=0` na segunda execução é o teste que importa
📚 [community.docker](https://docs.ansible.com/ansible/latest/collections/community/docker/) — `docker_compose_v2` chama a CLI e dispensa o SDK Python no servidor
