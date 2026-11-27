# Imagem base oficial do Fedora 44 (suporte nativo multi-arquitetura: arm64 e amd64)
FROM quay.io/fedora/fedora:44

# Instala systemd (PID 1), util-linux (/usr/sbin/nologin) e sudo (para validar become: true) em camada única,
# limpando caches e metadados do dnf em seguida para manter a imagem de teste enxuta
RUN dnf -y install systemd util-linux sudo \
    && dnf clean all \
    && rm -rf /var/cache/dnf

# Cria um usuário comum no grupo wheel com sudo sem senha para simular o acesso inicial de uma VM cloud
# e validar se todas as tasks do playbook escalam privilégios corretamente via become: true
RUN useradd -m -G wheel ansible \
    && echo 'ansible ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ansible \
    && chmod 0440 /etc/sudoers.d/ansible

# Define explicitamente o sinal POSIX esperado pelo systemd para encerramento gracioso no molecule destroy
STOPSIGNAL SIGRTMIN+3

# Inicia o systemd como processo principal (PID 1) como root; o Ansible conectará como o usuário 'ansible'
CMD ["/usr/sbin/init"]
