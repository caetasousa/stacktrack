# 🧱 Infraestrutura como código

O servidor descrito em Ansible, em `deploy/ansible/`. O que antes era um roteiro
de comandos para copiar à mão em `producao.md` agora é um playbook que se pode
rodar quantas vezes quiser.

A esteira que leva o commit até o ar continua sendo o GitHub Actions
([entrega-continua.md](entrega-continua.md)); o Ansible cuida do **servidor**,
não da versão que roda nele.

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
| `preparar-host.yml` | uma vez por **máquina** | `root` | Docker, plugin do Compose, rede `borda`, usuário `deploy` |
| `provisionar.yml` | sempre que quiser | `deploy`, **sem sudo** | diretórios, `.env`, compose, `backup.sh`, cron, bloco do Caddy, stack no ar |

A divisória não é estética. `provisionar.yml` não precisa de privilégio nenhum:
tudo mora no home do `deploy` e o acesso ao Docker vem do grupo — **o mesmo
poder que a esteira já tem**. Só o outro exige root, e é justamente o que não
pode ser automatizado pelo CI: `VPS_USER` é `deploy`, e um login como `deploy`
não pode criar o `deploy`.

No VPS de hoje o `preparar-host.yml` não tem o que fazer: Docker, Compose, rede
`borda` e o usuário vieram do agendaGo. Rodá-lo deve terminar em `changed=0` —
que é a prova de que ele descreve a máquina, em vez de só saber construí-la.

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

`make infra-segredos` gera `deploy/ansible/group_vars/producao/vault.yml`,
cifrado com `ansible-vault`, contendo o nome do banco, o usuário, a senha
(aleatória, de `openssl rand -hex 24`) e o token do GHCR.

A **senha do vault** vai para `deploy/ansible/.senha-vault`, que está no
`.gitignore`. Ela é o que decifra os segredos de produção:

> **Guarde uma cópia fora desta máquina.** Sem ela o `vault.yml` não abre mais,
> e a senha do banco existe apenas dentro do volume do Postgres — de onde não
> sai.

Para ver ou editar:

```bash
cd deploy/ansible
ansible-vault view group_vars/producao/vault.yml
ansible-vault edit group_vars/producao/vault.yml
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
| **Docker** | instalado só quando falta, **nunca atualizado**: `apt upgrade` do `docker-ce` reinicia o daemon e derruba os containers do vizinho junto |
| **`~/backups`** | comum aos dois; a separação é o prefixo do nome do arquivo |

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

### Por que não há job de CI para o playbook

Um `ansible-lint` ou `--syntax-check` no GitHub Actions precisa **carregar** os
`group_vars` — e um deles é cifrado. Sem a senha do vault o Ansible nem começa:

```
ERROR! The vault password file .senha-vault was not found
```

Ou seja: validar o playbook no CI exige o secret `ANSIBLE_VAULT_PASSWORD`, que é
a mesma escalada de raio de explosão que a migração do job `implantar` para o
Ansible exigiria. As duas coisas andam juntas e ficaram para o mesmo momento —
ver `PLANO.md`, fase 16, etapa B. Até lá a validação é local, pelo
`make infra-check`.

---

## 🧭 O que continua fora

- **Endurecimento do host** — fuso, `unattended-upgrades`, `sshd`, `ufw`. Fase
  própria; misturá-lo com a reconstrução juntaria duas fontes de falha.
- **Criar a máquina** — é Terraform, não Ansible.
- **DNS / DuckDNS** — o registro é externo, ver [producao.md](producao.md) §1.
- **Cópia de backup para fora do VPS** — pendência antiga, entra junto com a
  chave de escrita no bucket.

---

## 📚 Para estudar

📚 [Ansible — documentação](https://docs.ansible.com/ansible/latest/) · [playbooks](https://docs.ansible.com/ansible/latest/playbook_guide/index.html) · [roles](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_reuse_roles.html)
📚 [ansible-vault](https://docs.ansible.com/ansible/latest/vault_guide/index.html) — como o segredo cifrado vive no repositório
📝 [Idempotência em Ansible](https://docs.ansible.com/ansible/latest/reference_appendices/glossary.html#term-Idempotency) — por que `changed=0` na segunda execução é o teste que importa
📚 [community.docker](https://docs.ansible.com/ansible/latest/collections/community/docker/) — `docker_compose_v2` chama a CLI e dispensa o SDK Python no servidor
