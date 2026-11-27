# Automated Linux Host Provisioning & Hardening with Ansible

[![Continuous Integration](https://github.com/kauatwn/ansible-app-host/actions/workflows/ci.yml/badge.svg)](https://github.com/kauatwn/ansible-app-host/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Ansible](https://img.shields.io/badge/Ansible-IaC-EE0000?style=flat&logo=ansible)](https://www.ansible.com/)
[![Fedora](https://img.shields.io/badge/Fedora-44-51A2DA?style=flat&logo=fedora)](https://fedoraproject.org/)
[![Caddy](https://img.shields.io/badge/Reverse_Proxy-Caddy-1F88C0?style=flat&logo=caddy)](https://caddyserver.com/)
[![Molecule](https://img.shields.io/badge/Testing-Molecule-6B4FBB?style=flat&logo=ansible)](https://ansible.readthedocs.io/projects/molecule/)

Infraestrutura como Código (**IaC**) desenvolvida para automatizar o provisionamento, o *hardening* de sistema operacional e o deploy idempotente de uma aplicação web em **Go** sobre servidores **Fedora 44** sem dependência de *runtime* de containers em produção, integrada a uma suíte de testes automatizados com **Ansible Molecule** e **GitHub Actions**.

## Visão Geral & Objetivo

O objetivo deste repositório é servir como um **laboratório prático** de administração de sistemas Linux (*SysAdmin*), segurança defensiva e automação declarativa em ambientes *bare-metal* ou máquinas virtuais tradicionais. Em vez de utilizar scripts Shell imperativos ou delegar o isolamento de processos, rede e sistema de arquivos a um orquestrador de containers, o projeto aplica controles nativos diretamente no kernel Linux, no **SELinux** e no gerenciador de serviços (`systemd`).

A aplicação em Go atua como um *workload* compilado estaticamente, executado como um daemon nativo sob um usuário sem privilégios (`appuser`) restrito à interface interna (`127.0.0.1:8080`), protegida por regras de `firewalld`, políticas do **SELinux** em modo `Enforcing`, restrições de *sandboxing* no `systemd` e exposta externamente apenas através do proxy reverso **Caddy** na porta `80`.

### Principais Pilares Implementados

- **Automação Modular & Idempotente (Ansible):** Separação estrita entre roles reutilizáveis (`security`, `app`, `proxy`) e playbook orquestrador (*glue code*), garantindo convergência com `changed=0` em execuções subsequentes.
- **Defesa em Profundidade no Host (Fedora 44):** Bloqueio de acesso direto à porta da aplicação via `firewalld`, *hardening* de SSH por arquivos *drop-in* (`sshd_config.d`), contextos explícitos no **SELinux** e isolamento de processo via `systemd`.
- **Proxy Reverso & Privilégio Mínimo (Caddy):** Desacoplamento entre a borda pública privilegiada (`:80`) e o processo da aplicação (`127.0.0.1:8080`), evitando conceder `root` ou *capabilities* de rede ao daemon do serviço.
- **Testes de Integração com Molecule & CI (GitHub Actions):** Validação automatizada em container efêmero executando `/usr/sbin/init` (`systemd` como PID 1) e usuário não-privilegiado (`ansible` com `sudo`), garantindo idempotência e validação E2E via HTTP a cada *push* ou *Pull Request*.

## Arquitetura da Solução

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
               |  4. Molecule Test Suite (Fedora 44 systemd Container) |
               +---------------------------+---------------------------+
                                           |
                        (Idempotence: changed=0 & E2E OK)
                                           |
                                           v
+-----------------------------------------------------------------------------------+
|                        Target Linux Host (Fedora 44)                              |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  |  firewalld (Perímetro: permite apenas 22/tcp e 80/tcp; bloqueia 8080/tcp)    |  |
|  +-----------------------+---------------------------------+-------------------+  |
|                          | :22                             | :80                  |
|                          v                                 v                      |
|  +-----------------------------------+   +-------------------------------------+  |
|  |  sshd Daemon                      |   |  Caddy Reverse Proxy                |  |
|  |  - /etc/ssh/sshd_config.d/        |   |  - Escuta borda externa (:80)       |  |
|  |  - PermitRootLogin no             |   |  - SELinux: httpd_can_network_connect|  |
|  |  - PasswordAuthentication no      |   +------------------+------------------+  |
|  +-----------------------------------+                      |                     |
|                                                             | HTTP (Loopback)     |
|                                                             v 127.0.0.1:8080      |
|                                          +-------------------------------------+  |
|                                          |  api.service (systemd Unit)         |  |
|                                          |  - Usuário: appuser (/sbin/nologin) |  |
|                                          |  - SELinux Context: bin_t           |  |
|                                          |  - Sandboxing: ProtectSystem=strict |  |
|                                          +-------------------------------------+  |
+-----------------------------------------------------------------------------------+
```

## Tecnologias e Decisões de Engenharia

### 1. Automação & Arquitetura Modular (Ansible)

- **Separação entre Roles e Glue Code:** Toda a lógica de configuração é encapsulada em roles coesas (`security`, `app` e `proxy`), enquanto `ansible/playbooks/site.yml` atua exclusivamente como orquestrador declarando grupos alvo, variáveis e ordem de execução.
- **Resolução Dinâmica de Artefatos:** A variável `app_binary_src` resolve automaticamente o caminho do binário estático (`bin/api`) tanto em execuções diretas via CLI (`ansible-playbook`) quanto em execuções orquestradas pelo Molecule dentro de `ansible/molecule/default/`.
- **Idempotência Estrita:** Todas as tarefas, templates Jinja2 e *handlers* são projetados para convergir ao estado desejado sem efeitos colaterais, garantindo `changed=0` em execuções repetidas.

> [!NOTE]
> **Bootstrap sem Python via módulo `raw`:** Imagens minimalistas do Fedora frequentemente não incluem o interpretador `python3` pré-instalado, o que impede a execução da fase padrão `gather_facts` do Ansible. Conforme recomendado pela documentação oficial do Ansible para *bootstrapping*, `site.yml` inicia com `gather_facts: false` e executa `dnf install -y python3` usando `ansible.builtin.raw` em `pre_tasks` antes de acionar os módulos declarativos convencionais.

### 2. Hardening de Sistema Operacional & Segurança (Fedora 44)

- **Perímetro de Rede (`firewalld`):** Apenas os serviços `ssh` (porta `22`) e `http` (porta `80`) são expostos na zona pública; o binário estático da API vincula-se exclusivamente à interface de *loopback* (`127.0.0.1:8080`), acessível apenas localmente pelo proxy reverso **Caddy**.
- **Acesso Administrativo via Drop-in (`sshd`):** Em vez de editar `/etc/ssh/sshd_config` diretamente, a role `security` deposita as diretivas em `/etc/ssh/sshd_config.d/00-hardening.conf` (`PermitRootLogin no` e `PasswordAuthentication no`), exigindo chaves Ed25519 e evitando conflitos durante atualizações de pacotes pelo `dnf`.
- **Privilégio Mínimo & Sandboxing (`systemd`):** Criação da conta de sistema `appuser` sem shell interativo (`/usr/sbin/nologin`). A unit `api.service` aplica diretivas de contenção no kernel (`NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=true`, `PrivateTmp=true`, além de restrições de *namespaces* e *syscalls*) para limitar a superfície de ataque do processo.
- **Conformidade com SELinux (`Enforcing`):** Aplicação persistente do contexto `bin_t` ao executável `/usr/local/bin/api` (`sefcontext` + `restorecon`) e habilitação do booleano `httpd_can_network_connect` para autorizar o encaminhamento HTTP do Caddy para a porta local `8080`.

### 3. Testes Automatizados & CI (Molecule & GitHub Actions)

- **Abordagem Ansible-Native (`delegated` driver):** O cenário `ansible/molecule/default/` gerencia o ciclo de vida completo do ambiente de testes utilizando playbooks Ansible nativos e `community.docker`.
- **Paridade Real com VMs Cloud:** O `Dockerfile` de teste constrói uma imagem **Fedora 44** executando `/usr/sbin/init` (`systemd` como PID 1) e cria o usuário comum `ansible` com permissão `sudo` em `/etc/sudoers.d/ansible` (`0440`), garantindo que o `become: true` do `site.yml` seja validado de ponta a ponta.
- **Verificação Declarativa (`verify.yml`):** Audita o status `running` e `enabled` das units (`sshd`, `firewalld`, `api`, `caddy`), inspeciona o arquivo aplicado em `/etc/ssh/sshd_config.d/00-hardening.conf` e executa uma requisição real `GET http://localhost/` validando o fluxo completo cliente -> Caddy -> API Go.
- **Pipeline de CI (`.github/workflows/ci.yml`):** Executa em paralelo os jobs `go-quality`, `ansible-syntax` e `go-build` (*fast-fail*), transfere o binário estático via GitHub Actions Artifact para o job `molecule-test`, coleta logs de diagnóstico (`systemctl` e `journalctl`) em caso de falha e destrói o container efêmero ao término.

> [!WARNING]
> **Trade-off de privilégios no container de testes:** Conforme documentado no guia oficial *Systemd Container* do Molecule, executar o `systemd` como PID 1 e manipular regras de rede (`nftables`) do `firewalld` dentro de um container exige `privileged: true`, `cgroupns_mode: host` e montagem de `/sys/fs/cgroup`. Essas opções são aplicadas **exclusivamente ao container efêmero de teste** no `create.yml` e nunca são utilizadas em workloads de produção.

## Estrutura do Repositório

```text
.
├── .github/
│   └── workflows/
│       └── ci.yml                  # Lint Go, vulncheck, syntax check, build e Molecule CI
├── cmd/
│   └── api/                        # Workload HTTP minimalista em Go (slog + graceful shutdown)
├── ansible/
│   ├── ansible.cfg                 # Otimizações SSH, pipelining e execução não-interativa
│   ├── requirements.yml            # Coleções Galaxy (ansible.posix, community.general, community.docker)
│   ├── inventory/
│   │   └── hosts.example.ini       # Modelo de inventário com faixas de documentação RFC 5737
│   ├── playbooks/
│   │   └── site.yml                # Playbook principal (bootstrap Python + orquestração das roles)
│   ├── roles/
│   │   ├── security/               # Firewall, drop-in SSH hardening e criação do appuser
│   │   ├── app/                    # Deploy do binário Go, contexto SELinux e unit api.service
│   │   └── proxy/                  # Instalação e configuração do Caddy como reverse proxy
│   └── molecule/
│       └── default/
│           ├── Dockerfile          # Imagem Fedora 44 com systemd (PID 1) e usuário sudo ansible
│           ├── molecule.yml        # Configuração do cenário, inventário efêmero e matriz de 11 etapas
│           ├── create.yml          # Build da imagem e inicialização do container com cgroups v2
│           ├── prepare.yml         # Geração de chaves SSH de controle e espera pelo boot do systemd
│           ├── converge.yml        # Importa diretamente playbooks/site.yml (paridade com produção)
│           ├── verify.yml          # Asserções de serviços, hardening SSH e teste HTTP E2E (:80)
│           ├── cleanup.yml         # Limpeza de resíduos intermediários
│           └── destroy.yml         # Encerramento gracioso (SIGRTMIN+3) e remoção do container
├── go.mod                          # Módulo Go 1.27
└── README.md
```

## Como Executar e Validar

### Pré-requisitos

- [Go 1.27+](https://go.dev/dl/)
- [Python 3.15+](https://www.python.org/)
- [Ansible](https://docs.ansible.com/)
- [Ansible Molecule](https://docs.ansible.com/projects/molecule/)
- [Docker Engine](https://docs.docker.com/get-docker/) em execução

### 1. Compilar o Binário Estático da Aplicação

```bash
mkdir -p bin
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
```

*(Caso esteja desenvolvendo em macOS e testando em uma VM ARM64 no OrbStack, ajuste `GOARCH=arm64` ao compilar o binário).*

### 2. Executar a Suíte Completa de Testes (Molecule)

```bash
cd ansible
molecule test
```

A execução percorre automaticamente as 11 fases configuradas:

```text
dependency → cleanup → destroy → syntax → create → prepare → converge → idempotence → verify → cleanup → destroy
```

> [!TIP]
> **Iteração rápida durante o desenvolvimento:** Em vez de destruir e recriar o container Fedora 44 a cada alteração nas roles, utilize os subcomandos granulares do Molecule:
>
> - `molecule converge`: cria o container (se ainda não existir) e aplica apenas as alterações do `site.yml`.
> - `molecule verify`: executa apenas as asserções do `verify.yml` contra o container em execução.
> - `molecule login`: abre um shell interativo dentro do container `fedora-test` para inspecionar logs ou estados do `systemd`.
> - `molecule destroy`: remove o container ao finalizar os testes.

### 3. Validar Antes e Provisionar um Servidor Linux (VM / Bare-Metal)

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

*(Nota: em um servidor recém-criado antes do primeiro provisionamento, o `--check` não simula o módulo `raw` nem unidades de serviços que ainda serão instaladas pelo `dnf`).*

### 4. Auditar Hardening e Validar o Host Pós-Provisionamento

Após a execução do playbook, valide o estado dos daemons, a exposição de portas e as proteções aplicadas no servidor alvo:

```bash
# Conferir o status e habilitação no boot dos serviços gerenciados
systemctl status api caddy firewalld

# Validar o fluxo completo através do proxy reverso (porta 80)
curl -i http://<IP_OU_HOSTNAME_DO_SERVIDOR>/

# Auditar as diretivas de sandboxing aplicadas à unit da aplicação
sudo systemd-analyze security api.service
```

| Verificação | Comando no Servidor Alvo | Resultado Esperado |
| :--- | :--- | :--- |
| **Endpoint HTTP (Borda)** | `curl -i http://localhost/` | Status `200 OK` e payload JSON retornado pela API via Caddy |
| **Portas Expostas** | `sudo ss -tulpn` | Caddy em `*:80`, `sshd` em `*:22` e API restrita a `127.0.0.1:8080` |
| **Sandboxing da Unit** | `sudo systemd-analyze security api.service` | Diretivas de isolamento ativas (`ProtectSystem=strict`, `NoNewPrivileges`, etc.) |
| **Contexto SELinux** | `ls -Z /usr/local/bin/api` | Contexto `system_u:object_r:bin_t:s0` aplicado ao executável |
