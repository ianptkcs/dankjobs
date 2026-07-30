# Como criar um job para o djobs

Este documento existe para que uma IA (ou uma pessoa) consiga criar um job
compatível com o djobs manualmente, sem abrir a TUI — por exemplo dentro
de um agente que precisa agendar uma tarefa de terminal para depois. Se você
tem a TUI à mão, é mais simples apertar `n` dentro dela (veja o final deste
documento); o que segue é o formato que ela espera encontrar no disco.

## O que o djobs enxerga

O djobs varre um diretório de jobs (`~/jobs` por padrão, configurável via
`DJOBS_JOBS_DIR`) procurando subdiretórios, e casa cada um por nome com um
par opcional de units systemd `--user` num segundo diretório (
`~/.config/systemd/user` por padrão, configurável via `DJOBS_SYSTEMD_DIR`).
Não há nenhum arquivo de metadado separado — todo o estado é inferido do
sistema de arquivos + do systemd.

Layout esperado por job, em `<jobs-dir>/<nome-do-job>/`:

- `<nome-do-job>.sh` — o script que faz o trabalho de fato.
- `<nome-do-job>-body.txt` (ou qualquer `*body*.txt`, e como fallback
  `*.txt`) — notas/descrição livres, opcionais, mostradas no painel de
  detalhes.
- `<nome-do-job>.log` — opcional. Escrever aqui é o que deixa um registro no
  histórico: a presença desse arquivo (já sem timer) é o que marca um job
  como "concluído" em vez de "removido".

Enquanto o job ainda está agendado, existe também o par de units em
`~/.config/systemd/user/`:

```
~/jobs/minha-tarefa/
  minha-tarefa.sh
  minha-tarefa.log
  minha-tarefa-body.txt

~/.config/systemd/user/
  minha-tarefa.timer
  minha-tarefa.service
```

## Os cinco estados que o djobs infere

- **ativo** — timer existe, unit habilitada.
- **pausado** — timer existe, unit desabilitada (mas o service nunca chegou
  a rodar e falhar).
- **concluído** — timer/service já não existem, e há um `.log`.
- **falha** — timer/service ainda existem, mas o `ActiveState` do service é
  `failed` (ele rodou e saiu com erro).
- **removido** — timer/service já não existem, e não há `.log` (agendamento
  apagado, ou nunca chegou a existir, antes de rodar).

**A consequência prática disso**: o próprio script do job é responsável por
remover as suas units systemd quando termina com sucesso. Se ele não fizer
essa auto-limpeza, o job fica para sempre em "pendentes" mesmo depois de já
ter rodado — não existe nenhum outro jeito do djobs saber que ele
terminou bem.

## Template do script

```bash
#!/usr/bin/env bash
set -euo pipefail

JOB_NAME="minha-tarefa"

# ... o trabalho de fato vai aqui ...

# auto-remove o par de units systemd ao terminar (one-shot, não recorrente)
systemctl --user disable --now "${JOB_NAME}.timer" 2>/dev/null || true
rm -f "$HOME/.config/systemd/user/${JOB_NAME}.timer" "$HOME/.config/systemd/user/${JOB_NAME}.service"
systemctl --user daemon-reload
```

`set -euo pipefail` importa além da segurança de sempre: é o que permite
distinguir "falha" de "removido". Um script que morre no meio do caminho
nunca chega até a linha de auto-limpeza, então deixa as units para trás
exatamente como um job pausado deixaria — e o djobs usa o
`ActiveState` do service pra diferenciar os dois casos.

## As duas units systemd

```ini
# ~/.config/systemd/user/minha-tarefa.service
[Unit]
Description=minha-tarefa

[Service]
Type=oneshot
ExecStart=/home/usuario/jobs/minha-tarefa/minha-tarefa.sh
StandardOutput=append:/home/usuario/jobs/minha-tarefa/minha-tarefa.log
StandardError=append:/home/usuario/jobs/minha-tarefa/minha-tarefa.log
```

```ini
# ~/.config/systemd/user/minha-tarefa.timer
[Unit]
Description=minha-tarefa

[Timer]
OnCalendar=2026-08-05 14:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

`OnCalendar` recebe um timestamp absoluto para um job one-shot — vale a pena
validar antes com `systemd-analyze calendar "<timestamp>"`. `Persistent=true`
é o que faz um disparo perdido (máquina desligada ou suspensa no horário
marcado) rodar assim que a máquina voltar, em vez de ser silenciosamente
pulado como o cron comum faria.

Depois de escrever os dois arquivos:

```bash
systemctl --user daemon-reload
systemctl --user enable --now minha-tarefa.timer
```

Isso depende do "lingering" do `loginctl` estar habilitado para o usuário
(`loginctl show-user "$USER" --property=Linger` deve retornar `yes`), senão
a unit não dispara sem uma sessão de login ativa.

## Ou simplesmente use o djobs

Se a TUI estiver disponível, aperte `n` — ela pede o nome, a data/hora e o(s)
comando(s) a executar, e já escreve o script (com o bloco de auto-limpeza
acima embutido), as duas units e habilita o timer por você.
