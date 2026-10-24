# 🧱 Infraestrutura como código

> **Em uma frase:** o que é do stacktrack no servidor, descrito em Ansible — os
> playbooks, o vault e como recomeçar do zero.

A **aplicação** descrita em Ansible, em `deploy/ansible/`. O que antes era um
roteiro de comandos para copiar à mão em `producao.md` agora é um playbook que
se pode rodar quantas vezes quiser.

O **servidor** é a distro `ubuntu-server` do WSL desta máquina, publicada por
um túnel da Cloudflare. Ele foi montado pelo `site.yml` do studygo
(`~/studygo/ansible`): o sshd na 2222, os pacotes do Docker rootless, o
`cloudflared`. Este repositório pressupõe esse servidor e descreve só o que é
do stacktrack: o usuário de deploy e o Docker rootless dele, o `.env`, o compose
(com a borda nginx dentro), o backup, o cron e os papéis do banco.

Publicar as imagens e implantar são manuais
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
| `preparar-host.yml` | quando a **identidade de deploy** muda | `stacktrack` + `sudo` | o usuário `stacktrack`, o Docker rootless dele, o `sudoers` restrito, a chave do operador, a chave de deploy, o wrapper `stacktrack-release` |
| `provisionar.yml` | sempre que quiser | `stacktrack`, **sem sudo** | diretórios, `.env`, compose, `backup.sh`, cron, papéis do banco, stack no ar |

A divisória não é estética. `provisionar.yml` não precisa de privilégio nenhum:
tudo mora no home do `stacktrack` e o Docker é o rootless dele. Só o outro
exige privilégio. A chave de release — a que você usaria de um script ou de
outra máquina — está presa a um comando forçado que executa três verbos
(`release`, `backup`, `estado`), nenhum que escreva arquivo, e nada de
`preparar-host.yml`.

O `preparar-host.yml` entra como `stacktrack` e sobe por `sudo`, em vez de logar
como root: o sshd do servidor tem `PermitRootLogin no`. Na **primeira vez**, com
o `stacktrack` ainda inexistente, entre por outra conta com sudo — no servidor
do WSL, a do studygo, que o `bootstrap-wsl.sh` dele criou com sudo sem senha:

```bash
make infra-preparar ARGS="-e ansible_user=studygo -e ansible_ssh_private_key_file=~/.ssh/studygo_ci"
# a senha do sudo é pedida: sem senha, basta Enter
```

`-e ansible_user=...`, e não `-u ...`: o `-u` troca só o usuário da conexão e
deixa a variável valendo `stacktrack`, o que mantém o `become` pedindo a senha
da conta errada. O `-e` tem a precedência mais alta e resolve os dois de uma vez.

**Por que o sudo do `stacktrack` é sem senha.** A conta nasce com a senha
bloqueada (`passwd -S` responde `L`), que é o certo para conta de serviço —
senha de conta de serviço é senha que alguém guarda num arquivo. Sem senha,
`sudo` com senha nunca funciona, e o playbook ficaria preso ao root para sempre.

É o mesmo sudo que o servidor dá a cada usuário de projeto, e quem o usa é a
chave do **operador** — você, que já é dono da máquina. A **chave de deploy** é
outro assunto: o acesso dela é restrito a um arquivo, e é ali que o privilégio
mínimo importa.

### 🐳 Um Docker por projeto

O `stacktrack` **não** está no grupo `docker`: o daemon de sistema está
desligado no servidor, e o grupo daria root no dia em que alguém o religasse. A
stack roda no Docker **rootless** da própria conta — daemon, rede, imagens e
volumes próprios, que o studygo não enxerga e vice-versa.

Todas as distros do WSL2 dividem a mesma rede, e um Docker de sistema brigaria
com o da distro de desenvolvimento pelo `docker0` e pelo iptables. O rootless
roda numa rede própria e só publica as portas pedidas.

O `preparar-host.yml` instala o daemon (`dockerd-rootless-setuptool.sh`), liga
o *linger* — sem ele, o daemon morre quando a última sessão do usuário fecha — e
cria o contexto `rootless` como corrente na config do cliente. É o contexto que
faz o shell, o comando forçado e o cron acharem o daemon certo sem exportar
nada. Os módulos `community.docker` não leem contexto: o `provisionar.yml` lhes
passa `DOCKER_HOST` no `environment` da play.

Rodar `preparar-host.yml` duas vezes seguidas deve terminar em `changed=0` — que
é a prova de que ele descreve a máquina, em vez de só saber construí-la.

### 🏠 A casa do stacktrack

```
/home/stacktrack/                 .env · docker-compose.prod.yml · scripts/ · backups/
/usr/local/bin/stacktrack-release o wrapper da chave de deploy
```

O roteamento de `stacktrack.d4ffy.com` (split `/api` → API, resto → front,
WebSocket, cabeçalhos) está no serviço **`borda`** do `docker-compose.prod.yml`:
um nginx com a config embutida no próprio compose, publicado em
`127.0.0.1:8580`. O TLS fica com a Cloudflare. O contrato está em
[producao.md](producao.md#a-borda-o-que-o-túnel-alcança).

### 🔑 Uma conta, duas chaves com poderes diferentes

O deploy **não tem conta própria**: ele entra no `stacktrack`, e o que o limita
é a chave, não o usuário.

```
authorized_keys do stacktrack
├── operador-stacktrack (~/.ssh/stacktrack_servidor)     → shell normal
└── restrict,command="/usr/local/bin/stacktrack-release" → três verbos
    release-stacktrack (~/.ssh/stacktrack_release)
```

As duas chaves são **do stacktrack, e de nada mais**. Trocar uma é trocar a
pública no `vars.yml` e pôr a antiga em `chaves_revogadas`, que a role mantém
ausentes do `authorized_keys` — com `exclusive: false`, só trocar o valor deixaria
a antiga lá, valendo. Foi como saíram, em out/2026, as duas da época da esteira:
a de deploy tinha a metade privada nos segredos do repositório no GitHub, e a de
operador era a chave da esteira do agendaGo. A revogação é a última task da role,
depois de as chaves novas estarem instaladas, para não trancar do lado de fora a
execução que faz a troca.

`command=` faz o sshd executar aquele programa e ignorar o que o cliente pediu;
o pedido vai para `SSH_ORIGINAL_COMMAND`, como dado. `restrict` desliga PTY,
encaminhamento de porta, de agente e X11.

**O que se abriu mão, para não haver conta separada.** Se um dia essa linha
perder as opções, a chave de deploy passa a dar shell numa conta com sudo — que
é root. Uma conta separada faria o mesmo erro
terminar num shell sem poder nenhum: falharia fechado, e esta escolha falha
aberta. A mitigação é o `authorized_keys` ser escrito pelo playbook, e não à
mão.

O wrapper (`/usr/local/bin/stacktrack-release`, root:root 0755) entende
`release <sha>`, `backup` e `estado`, e recusa o resto. Nenhum dos três escreve
arquivo: o compose, o `.env` e o `backup.sh` só chegam pelo `make infra-apply`.
Houve um quarto, `sync`, que instalava o compose recebido pelo stdin — era como
a esteira o entregava —, e ele saiu com ela: o compose carrega a configuração da
borda, e reescrevê-lo seria entregar o roteamento do domínio a quem tivesse a
chave de release.

Ele lê `SSH_ORIGINAL_COMMAND` com `read -ra`, nunca `eval`: a linha do cliente é
dado, não código. `scripts/testa-wrapper-de-release.sh` executa o script
renderizado contra um `docker` de mentira e exige recusa para shell,
encadeamento, substituição de comando, SHA malformado e o `sync` — e roda em
`make infra-validar`.

---

## 🚀 Uso

```bash
sudo apt install ansible          # Ubuntu 24.04: o PEP 668 bloqueia o pip

make infra-segredos               # UMA vez: gera e cifra a senha do banco
make infra-preparar               # o que exige privilégio (a 1ª vez por outra conta, ver acima)
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
quando a play roda de verdade. É o que permite validar o playbook (`make
infra-validar`) numa máquina que não tem a senha dos segredos de produção.

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

## 🤝 A fronteira com o servidor

O servidor do WSL é compartilhado com o studygo, e o studygo é quem o montou. O
playbook do stacktrack nunca escreve fora do que é dele:

| | Como |
|---|---|
| **roteamento** | no serviço `borda` do `docker-compose.prod.yml`; nada no nginx nem no `cloudflared` do servidor |
| **túnel** | a rota `stacktrack.d4ffy.com` → `http://localhost:8580` é do painel da Cloudflare, no túnel com token que já existia |
| **porta** | só a 8580, em loopback. A 8480 é do studygo; as distros do WSL dividem a rede, então porta nova é decisão, não detalhe |
| **crontab** | `ansible.builtin.cron` com `name:` gerencia só o bloco marcado — as outras linhas ficam intactas, por desenho e não por sorte |
| **pacotes, sshd, cloudflared** | do `site.yml` do studygo — este repo pressupõe tudo isso pronto, e o `preparar-host.yml` recusa seguir sem o instalador rootless |

O crontab tem **um dono só**: o Ansible. Antes, quando o deploy também mexia
nele (um `crontab -l | grep -v … | crontab -`), os dois brigavam: o `grep -v`
removia a linha do job sem remover o comentário `#Ansible:`, o marcador ficava
órfão, e o `infra-check` seguinte acusava `changed` — reescrevia, e o deploy
seguinte reescrevia de volta. Um `changed=0` que nunca estabiliza não prova
nada. Hoje só o `ansible.builtin.cron` toca no bloco marcado.

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
(`~/scripts/backup.sh`) e leve o arquivo para fora do servidor — a cópia
remota ainda não existe, ver [producao.md](producao.md).

O `sudo rm -rf /home/stacktrack` leva junto o Docker rootless da conta (imagens
e volumes moram em `~/.local/share/docker`); o `make infra-preparar` o recria.
O que **não** fazer no servidor, por parecer faxina: parar o `cloudflared` ou o
nginx de sistema, ou mexer no usuário `studygo` — são do outro projeto.

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
5. `curl -sI https://studygo.d4ffy.com` — o vizinho continua no ar
6. `crontab -u stacktrack -l` — a linha do backup do stacktrack

### Validar o playbook sem a senha do vault

```bash
make infra-validar        # ansible-lint + --syntax-check nos dois playbooks + wrapper
```

Isso roda **sem** decifrar o vault, e é de propósito: enquanto o arquivo cifrado
morava em `group_vars/`, o Ansible o carregava ao montar as variáveis do grupo, e
até um `--syntax-check` morria com `ERROR! The vault password file .senha-vault
was not found`. Hoje o segredo entra por `include_vars` no `provisionar.yml`, que
só executa numa play de verdade — então dá para validar a sintaxe e o lint numa
máquina que não tem a senha.

O que `make infra-validar` não cobre é o que exige o servidor: `--check`/`--diff`
contra o host e a prova de idempotência (`changed=0` na segunda aplicação),
ambos pelo `make infra-check`.

---

## 🧭 O que continua fora

- **O servidor** — a distro `ubuntu-server`, o sshd na 2222, os pacotes do
  Docker rootless e o `cloudflared` são do `site.yml` do studygo.
- **A rota do túnel** — `stacktrack.d4ffy.com` → `http://localhost:8580`, criada
  no painel da Cloudflare (Zero Trust → Networks → Tunnels → Routes). O túnel
  com token é administrado lá, não por arquivo no servidor.
- **Ficar no ar** — o servidor só responde com o PC ligado e a distro de pé; a
  tarefa agendada do Windows que a mantém acordada está em
  `~/studygo/docs/deploy.md`.
- **Fuso do host** — o cron do backup roda no fuso do servidor; o `estado`
  imprime os dois.
- **Cópia de backup para fora do servidor** — pendência antiga, entra junto com
  a chave de escrita no bucket.

---

## 📚 Para estudar

📚 [Ansible — documentação](https://docs.ansible.com/ansible/latest/) · [playbooks](https://docs.ansible.com/ansible/latest/playbook_guide/index.html) · [roles](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_reuse_roles.html)
📚 [ansible-vault](https://docs.ansible.com/ansible/latest/vault_guide/index.html) — como o segredo cifrado vive no repositório
📝 [Idempotência em Ansible](https://docs.ansible.com/ansible/latest/reference_appendices/glossary.html#term-Idempotency) — por que `changed=0` na segunda execução é o teste que importa
📚 [community.docker](https://docs.ansible.com/ansible/latest/collections/community/docker/) — `docker_compose_v2` chama a CLI e dispensa o SDK Python no servidor
