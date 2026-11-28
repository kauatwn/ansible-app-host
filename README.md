# Automated Linux Host Provisioning & Hardening with Ansible

[![Continuous Integration](https://github.com/kauatwn/ansible-app-host/actions/workflows/ci.yml/badge.svg)](https://github.com/kauatwn/ansible-app-host/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Ansible](https://img.shields.io/badge/Ansible-IaC-EE0000?style=flat&logo=ansible)](https://www.ansible.com/)
[![Fedora](https://img.shields.io/badge/Fedora-44-51A2DA?style=flat&logo=fedora)](https://fedoraproject.org/)
[![Debian](https://img.shields.io/badge/Debian-13-A81D33?style=flat&logo=debian)](https://www.debian.org/)
[![Caddy](https://img.shields.io/badge/Reverse_Proxy-Caddy-1F88C0?style=flat&logo=caddy)](https://caddyserver.com/)
[![Molecule](https://img.shields.io/badge/Testing-Molecule-6B4FBB?style=flat&logo=ansible)](https://ansible.readthedocs.io/projects/molecule/)

Infraestrutura como Código (**IaC**) para provisionamento, *hardening* e deploy de uma aplicação web em **Go** sobre servidores Linux (**Fedora 44** como plataforma principal e **Debian 13** validado em matriz multi-distro), utilizando **Ansible**. O projeto explora recursos nativos do sistema operacional — como `systemd`, `firewalld` e **SELinux** — sem depender de *runtime* de containers em produção.

## Visão Geral

Este repositório funciona como um laboratório prático de administração de sistemas Linux (*SysAdmin*), segurança defensiva e automação declarativa. Em vez de encapsular a aplicação em um container, o playbook configura diretamente o host: cria um usuário de sistema sem privilégios (`appuser`), executa o binário estático em Go via `systemd` restrito a `127.0.0.1:8080` e expõe o serviço na porta `80` através do proxy reverso **Caddy**.

**Principais componentes:**

- **Ansible:** provisionamento modular, idempotente (`changed=0`) e compatível com as famílias `RedHat` e `Debian`.
- **Fedora 44 & Debian 13:** Fedora como alvo principal e Debian 13 validado continuamente na suíte de testes.
- **firewalld, SSH drop-in & SELinux:** controles de rede, autenticação e confinamento de processos no host.
- **systemd:** gerenciamento do ciclo de vida e *sandboxing* da aplicação Go.
- **Caddy:** proxy reverso desacoplando a borda HTTP (`:80`) do processo interno (`:8080`).
- **Molecule & GitHub Actions:** testes automatizados em containers efêmeros com `systemd` e pipeline de CI.

## Arquitetura

```text
               +-------------------------------------------------------+
               |                  GitHub Repository                    |
               +---------------------------+---------------------------+
                                           |
                                 (Push / PR to main)
                                           |
                                           v
               +-------------------------------------------------------+
               |             GitHub Actions (ci.yml)                   |
               |  1. Go Lint, Unit Tests (-race) & govulncheck         |
               |  2. Ansible Playbook Syntax Check                     |
               |  3. Static Binary Build (linux/amd64 Artifact)        |
               |  4. Molecule Multi-Distro Matrix (Fedora 44 & Debian) |
               +---------------------------+---------------------------+
                                           |
                        (Idempotence: changed=0 & E2E OK)
                                           |
                                           v
+-----------------------------------------------------------------------------------+
|                   Target Linux Host (Fedora 44 / Debian 13)                       |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  |  firewalld (Perímetro: permite apenas 22/tcp e 80/tcp; bloqueia 8080/tcp)    |  |
|  +-----------------------+---------------------------------+-------------------+  |
|                          | :22                             | :80                  |
|                          v                                 v                      |
|  +-----------------------------------+   +-------------------------------------+  |
|  |  sshd / ssh Daemon                |   |  Caddy Reverse Proxy                |  |
|  |  - /etc/ssh/sshd_config.d/        |   |  - Escuta borda externa (:80)       |  |
|  |  - PermitRootLogin no             |   |  - SELinux: httpd_can_network_connect|  |
|  |  - PasswordAuthentication no      |   +------------------+------------------+  |
|  +-----------------------------------+                      | (RedHat)            |
|                                                             | HTTP (Loopback)     |
|                                                             v 127.0.0.1:8080      |
|                                          +-------------------------------------+  |
|                                          |  api.service (systemd Unit)         |  |
|                                          |  - Usuário: appuser (/sbin/nologin) |  |
|                                          |  - SELinux: bin_t (RedHat)          |  |
|                                          |  - Sandboxing: ProtectSystem=strict |  |
|                                          +-------------------------------------+  |
+-----------------------------------------------------------------------------------+
```

## Decisões Técnicas

### 1. Automação Modular e Multi-Distro (Ansible)

A configuração está dividida em três roles independentes, coordenadas por `ansible/playbooks/site.yml`:

- `security`: firewall, *hardening* do SSH e criação do usuário `appuser`.
- `app`: deploy do binário Go, contexto SELinux (quando aplicável) e unit `api.service`.
- `proxy`: instalação e configuração do **Caddy** como proxy reverso.

Para evitar duplicação de tarefas entre distribuições, parâmetros agnósticos ficam em `defaults/main.yml`, enquanto diferenças de cada família (nomes de pacotes, daemons `sshd` vs. `ssh`, grupos `wheel` vs. `sudo` e booleanos SELinux) ficam isoladas em `vars/RedHat.yml` e `vars/Debian.yml`, carregadas dinamicamente via `include_vars` e instaladas com o módulo genérico `ansible.builtin.package`.

- **Resolução do binário:** a variável `app_binary_src` localiza `bin/api` tanto em execuções diretas via CLI quanto no contexto isolado do Molecule.
- **Idempotência:** todas as tarefas e *handlers* convergem para `changed=0` a partir da segunda execução.

> [!NOTE]
> **Bootstrap sem Python via módulo `raw`:** Imagens minimalistas de cloud frequentemente não trazem `python3` pré-instalado, impedindo a fase `gather_facts` do Ansible. Por isso, `site.yml` inicia com `gather_facts: false` e executa uma tarefa `ansible.builtin.raw` em `pre_tasks` com *guard clauses* POSIX para detectar e instalar `python3` via `dnf` (RedHat) ou `apt-get` (Debian) antes de rodar os módulos declarativos.

### 2. Segurança do Host

| Camada | Configuração Aplicada |
| --- | --- |
| **SSH** | Arquivo *drop-in* `/etc/ssh/sshd_config.d/00-hardening.conf` bloqueando login `root` e autenticação por senha, sem alterar o arquivo principal do pacote |
| **Rede (`firewalld`)** | Libera apenas `22/tcp` (SSH) e `80/tcp` (HTTP) na zona pública |
| **Isolamento de Porta** | Binário Go escuta apenas em `127.0.0.1:8080`, acessível somente pelo Caddy |
| **Usuário de Serviço** | Conta de sistema `appuser` sem shell interativo (`/usr/sbin/nologin`) |
| **Sandboxing (`systemd`)** | Diretivas `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true` e `PrivateTmp=true` na unit `api.service` |
| **SELinux (`RedHat`)** | Contexto persistente `bin_t` em `/usr/local/bin/api` (`sefcontext` + `restorecon`) e booleano `httpd_can_network_connect` para o Caddy |

### 3. Testes Automatizados (Molecule & CI)

O cenário `ansible/molecule/default/` utiliza o driver `delegated` com playbooks Ansible nativos e `community.docker` para testar **Fedora 44** (`Dockerfile`) e **Debian 13** (`Dockerfile.debian`) em paralelo.

Cada container inicia `/usr/sbin/init` (`systemd` como PID 1) e define um usuário comum `ansible` com permissão `sudo` em `/etc/sudoers.d/ansible` (`0440`), validando a escalada de privilégios (`become: true`) usada em produção. Na etapa `verify.yml`, o Molecule audita os serviços ativos (`sshd`/`ssh`, `firewalld`, `api`, `caddy`), o arquivo de *hardening* SSH e executa uma requisição real `GET http://localhost/`.

No GitHub Actions (`.github/workflows/ci.yml`), os jobs de lint/testes Go (`-race` + `govulncheck`), validação de sintaxe Ansible e build do binário rodam em paralelo antes de acionar a matriz do Molecule.

> [!WARNING]
> **Trade-off de privilégios nos containers de teste:** Executar o `systemd` como PID 1 e manipular regras de rede (`nftables`) do `firewalld` dentro de um container exige `privileged: true`, `cgroupns_mode: host` e montagem de `/sys/fs/cgroup`. Essas opções são aplicadas **exclusivamente aos containers efêmeros de teste** no `create.yml` e nunca em produção.

## Estrutura do Repositório

```text
.
├── .github/
│   └── workflows/
│       └── ci.yml                  # Pipeline CI: testes Go, lint, build e Molecule multi-distro
├── cmd/
│   └── api/                        # API HTTP minimalista em Go (slog + graceful shutdown)
├── ansible/
│   ├── ansible.cfg                 # Configurações de SSH, pipelining e execução não-interativa
│   ├── requirements.yml            # Coleções Galaxy (ansible.posix, community.general, community.docker)
│   ├── inventory/
│   │   └── hosts.example.ini       # Modelo de inventário com faixas de documentação RFC 5737
│   ├── playbooks/
│   │   └── site.yml                # Playbook principal (bootstrap Python + orquestração das roles)
│   ├── roles/
│   │   ├── security/               # Firewall, drop-in SSH, appuser e vars/{RedHat,Debian}.yml
│   │   ├── app/                    # Deploy do binário Go, contexto SELinux e unit api.service
│   │   └── proxy/                  # Instalação/configuração do Caddy e vars/{RedHat,Debian}.yml
│   └── molecule/
│       └── default/
│           ├── Dockerfile          # Imagem de teste Fedora 44 com systemd (PID 1)
│           ├── Dockerfile.debian   # Imagem de teste Debian 13 com systemd (PID 1)
│           ├── molecule.yml        # Matriz multi-distro e sequência de 11 etapas
│           ├── create.yml          # Build das imagens e subida dos containers com cgroups v2
│           ├── prepare.yml         # Espera pelo boot completo do systemd
│           ├── converge.yml        # Executa playbooks/site.yml nos containers de teste
│           ├── verify.yml          # Asserções de serviços, hardening SSH e teste HTTP E2E (:80)
│           ├── cleanup.yml         # Limpeza de arquivos temporários
│           └── destroy.yml         # Encerramento gracioso (SIGRTMIN+3) e remoção dos containers
├── go.mod
└── README.md
```

## Como Executar e Validar

### Pré-requisitos

- [Go 1.27+](https://go.dev/dl/)
- [Python 3.15+](https://www.python.org/)
- [Ansible](https://docs.ansible.com/) & [Ansible Molecule](https://docs.ansible.com/projects/molecule/)
- [Docker Engine](https://docs.docker.com/get-docker/) em execução

### 1. Compilar o Binário Estático

```bash
mkdir -p bin
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
```

*(Para hosts ARM64, como VMs no Apple Silicon via OrbStack, altere `GOARCH=arm64`).*

### 2. Executar a Suíte de Testes (Molecule)

```bash
cd ansible
molecule test
```

> [!TIP]
> **Iteração rápida durante o desenvolvimento:** Evite recriar os containers a cada ajuste nas roles usando os comandos granulares do Molecule:
>
> - `molecule converge`: aplica apenas as alterações do `site.yml` nos containers ativos.
> - `molecule verify`: roda somente as asserções do `verify.yml`.
> - `molecule login -h fedora-test` (ou `-h debian-test`): abre um shell interativo na instância desejada.
> - `molecule destroy`: remove os containers ao encerrar os testes.

### 3. Provisionar um Servidor Linux (VM / Bare-Metal)

Configure o inventário local (`hosts.ini`) a partir do modelo e execute a sequência de pré-validação antes de aplicar alterações no host:

```bash
cd ansible
cp inventory/hosts.example.ini inventory/hosts.ini
# Edite inventory/hosts.ini informando o IP/hostname e o usuário SSH com sudo do servidor alvo

# 1. Instalar as coleções necessárias do Ansible Galaxy
ansible-galaxy collection install -r requirements.yml

# 2. Verificar a sintaxe do playbook e templates
ansible-playbook -i inventory/hosts.ini playbooks/site.yml --syntax-check

# 3. Validar conectividade SSH com o servidor alvo
ansible -i inventory/hosts.ini web -m ansible.builtin.ping

# 4. Simular alterações em modo dry-run antes de aplicar
ansible-playbook -i inventory/hosts.ini playbooks/site.yml --check --diff

# 5. Executar o provisionamento completo
ansible-playbook -i inventory/hosts.ini playbooks/site.yml
```

> [!NOTE]
> Em um servidor recém-criado antes do primeiro provisionamento, o modo `--check` não simula o bootstrap via módulo `raw` nem unidades de serviços de pacotes ainda não instalados.
>
> Caso a VM utilize um usuário padrão diferente do configurado no inventário na primeira execução, sobrescreva via `-e "ansible_user=<user>"`. Exemplo:
>
> ```bash
> ansible-playbook -i inventory/hosts.ini playbooks/site.yml -e "ansible_user=<user>"
> ```

### 4. Validar o Servidor Pós-Provisionamento

Após a execução do playbook, valide o estado dos serviços, a resposta HTTP pelo proxy reverso e o isolamento da unit:

```bash
# Conferir o status e habilitação no boot dos serviços gerenciados
systemctl status api caddy firewalld

# Validar o fluxo completo através do proxy reverso (porta 80)
curl -i http://<IP_OU_HOSTNAME_DO_SERVIDOR>/

# Auditar as diretivas de sandboxing aplicadas à unit da aplicação
sudo systemd-analyze security api.service
```

| Verificação | Resultado Esperado |
| --- | --- |
| **Serviços (`systemctl status`)** | Units `api`, `caddy` e `firewalld` como `active (running)` e `enabled` |
| **Endpoint HTTP (`:80`)** | Status `200 OK` e payload JSON retornado pela API via Caddy |
| **Sandboxing (`systemd-analyze`)** | Diretivas de isolamento ativas (`ProtectSystem=strict`, `NoNewPrivileges`, etc.) |
