# meia-noite

**Um assistente de voz, em português, que comanda as IAs de programação do seu computador.**

Você diz **"meia-noite"**, faz o pedido em voz alta, e ele escolhe a IA certa (Claude Code, Codex, Cursor, Antigravity, OpenCode), manda trabalhar no projeto certo, acompanha e conta o resultado falando. É a ideia do "Jarvis", só que para quem programa com várias IAs ao mesmo tempo.

![Os seis estados do orbe](arte/orbe-estados.png)

> **Alpha 1.** Projeto novo, nascido do uso de uma pessoa só. Funciona no dia a dia no macOS; Linux e Windows compilam e passam nos testes automáticos, mas ainda não foram usados de verdade. O que falta está listado no fim, sem enfeite. Toda ajuda é bem-vinda.

## A ideia

Quem programa com IA hoje tem três, quatro, cinco ferramentas instaladas, cada uma numa janela, cada uma boa numa coisa. O trabalho virou abrir a certa, na pasta certa, escrever o pedido, esperar, voltar para ver.

O meia-noite é o **coordenador** disso. Ele não programa: ele manda quem programa.

- Você fala: *"meia-noite, manda o Codex resumir o projeto da loja"*.
- Ele confirma em um segundo, dispara a IA sem abrir tela, e você continua o que estava fazendo.
- Quando ela termina, ele te conta o essencial em voz alta.

Três decisões guiam o projeto:

1. **O cérebro é seu.** O meia-noite não vem com modelo embutido nem com conta de ninguém. Você cadastra a chave de uma API sua — qualquer uma que converse com ferramentas — ou aponta para um modelo rodando no próprio computador.
2. **Sua voz não sai da máquina.** A transcrição é local. O que vai para fora é só o texto do pedido (para o cérebro) e, se você escolher uma voz na nuvem, o texto da resposta.
3. **Travas em código, não em promessa do modelo.** Lista fechada de IAs, pastas permitidas, leitura por padrão, confirmação falada para alterar arquivo, limites diários. Um modelo enganado continua preso às mesmas travas.

## O que ele faz

- **Atende pelo nome.** "Meia-noite, que horas são?" ou só "meia-noite" e depois o pedido.
- **Dispara IAs de código**, cada uma com o login que você já tem, e acompanha: rodando, terminou, erro, cancelada, bloqueada, tempo esgotado.
- **Conta o que o seu Claude Code está fazendo**, lendo o fim das conversas dele neste computador.
- **Lembra em duas camadas**: uma memória curta local, instantânea, para fatos simples; e o [Nexusyn](https://nexusyn.ai) como memória longa, compartilhada com os seus outros agentes.
- **Tem uma oficina**: uma pasta própria onde escreve pequenos scripts para tarefas repetitivas, que só rodam numa caixa fechada e com o seu "sim".
- **Mostra um rosto**: um orbe 3D no navegador que muda quando ele ouve, pensa, fala ou tem IAs trabalhando.
- **Serve outras IAs**: tudo isso também é um servidor MCP.

## Sistemas

| | macOS (Apple Silicon e Intel) | Linux | Windows |
|---|---|---|---|
| Estado | **usado de verdade todo dia** | compila, testes automáticos passam; **não testado em uso real** | compila; **não testado em uso real** |
| Disparar IAs, conversa por texto, MCP, orbe | sim | sim | sim |
| Microfone | automático | PulseAudio/PipeWire | precisa do nome do dispositivo no config |
| Voz sem conta | `say` | `espeak-ng` | fala do Windows |
| Cofre da chave (`mn chave`) | Chaves do macOS | libsecret (`secret-tool`) | DPAPI |
| Rodar scripts da oficina | sim (caixa fechada) | ainda não | ainda não |

Se você usa Linux ou Windows, o relato do que funcionou e do que quebrou é a contribuição mais valiosa agora.

## Do que precisa

| Para | Precisa |
|---|---|
| Rodar | o binário da [página de versões](https://github.com/lrdeoliveira/meia-noite/releases), ou Go 1.27+ para compilar |
| Disparar IAs | ao menos uma instalada e logada: `claude`, `codex`, `agent` (Cursor), `agy`, `opencode` |
| **Conversar** | **uma chave de API sua para servir de cérebro** (ou um modelo local) |
| Ouvir | `ffmpeg` e `whisper-server` (do [whisper.cpp](https://github.com/ggerganov/whisper.cpp)) + dois modelos |
| Falar | nada (voz do sistema) — ou `mmx`, a CLI da MiniMax, para uma voz mais natural |
| Memória longa | uma conta no [Nexusyn](https://nexusyn.ai) (opcional) |

## Instalação

Baixe o arquivo do seu sistema na [página de versões](https://github.com/lrdeoliveira/meia-noite/releases), descompacte e ponha o `mn` no seu PATH. Ou compile:

```bash
git clone https://github.com/lrdeoliveira/meia-noite && cd meia-noite
go build -o bin/mn ./cmd/mn
```

Depois:

```bash
mn config        # cria o config.json e mostra onde ele está
```

### 1. Diga onde as IAs podem trabalhar

No `config.json`, preencha `pastas_permitidas`. Vem vazio de propósito: sem isso ele não dispara nada. Um modelo completo está em [`config.exemplo.json`](config.exemplo.json).

### 2. Cadastre o cérebro (obrigatório): a sua API

O meia-noite **usa uma API de modelo de linguagem como cérebro, e a chave é sua**. Serve qualquer API que converse com ferramentas (*tool calling*).

No `config.json`:

```json
"cerebro": { "tipo": "openai", "url": "https://api.openai.com/v1", "modelo": "nome-do-modelo" }
```

| `tipo` | Formato | Serve para |
|---|---|---|
| `openai` | `POST {url}/chat/completions` | a maioria dos provedores (OpenAI, Groq, OpenRouter, Mistral, DeepSeek, Gemini no modo compatível…) e servidores locais (Ollama, LM Studio) |
| `anthropic` | `POST {url}/v1/messages` | Anthropic e provedores compatíveis |
| `mmx` | comando `mmx` já logado | quem usa a CLI da MiniMax; não precisa de chave aqui |

Guarde a chave:

```bash
mn chave         # cola a chave; ela vai para o cofre do sistema, não para um arquivo
```

Alternativas: uma variável de ambiente (`"chave_env": "NOME_DA_VARIAVEL"`, padrão `MN_CEREBRO_CHAVE`) ou uma referência do 1Password (`"ref_chave": "op://…"`). **A chave nunca vai para o `config.json`.**

Modelo local, sem chave e sem nada sair do computador:

```json
"cerebro": { "tipo": "openai", "url": "http://127.0.0.1:11434/v1", "modelo": "qwen3" }
```

O endereço precisa ser `https`; `http` só é aceito para `127.0.0.1`/`localhost`. Você paga o uso da sua API: um pedido simples gasta de uma a três chamadas, e há um teto de 400 chamadas por dia (`max_cerebro_por_dia`). Modelos pequenos erram mais na escolha de ferramenta: prefira um que seja bom em *tool calling*.

### 3. Ligue a memória: Nexusyn (recomendado)

Sem memória longa, o meia-noite só sabe o que está na conversa e nas notas curtas. Com o [Nexusyn](https://nexusyn.ai) ele responde sobre os seus projetos, decisões e pendências — e essa memória é a mesma que os seus outros agentes (Claude Code, Cursor, Codex…) leem e escrevem, então o que um aprende, todos sabem.

1. Crie a conta em [nexusyn.ai](https://nexusyn.ai) e gere um token de acesso.
2. Entregue o token de um destes jeitos:

```bash
export MN_NEXUSYN_TOKEN="seu-token"          # variável de ambiente
```

```json
"nexusyn": { "url": "https://api.nexusyn.ai/v1/mcp", "ref_chave": "op://Cofre/item/credential" }
```

O meia-noite só **lê** o Nexusyn (ferramenta `lembrar`). Cada consulta leva alguns segundos, e por isso existe a memória curta:

| | Memória curta | Nexusyn |
|---|---|---|
| Onde fica | arquivo neste computador | nuvem, compartilhada entre agentes |
| Velocidade | instantânea (já está no contexto) | 6 a 9 s por consulta |
| Guarda | até 40 fatos de uma frase | o histórico inteiro dos projetos |
| Exemplos | "prefere respostas curtas", "o projeto loja é o loja-web" | "o que decidimos sobre o billing?", "o que falta no projeto X?" |

Ele mesmo anota na memória curta o que vale ter à mão ("meia-noite, anota que…") e apaga quando você diz que mudou.

### 4. Voz

Para ouvir, instale `ffmpeg` e o `whisper-server` (macOS: `brew install whisper-cpp ffmpeg`) e baixe os dois modelos para a pasta `modelos/` dentro da pasta de dados que o `mn config` mostrou:

```bash
curl -LO https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin   # 574 MB
curl -LO https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin          # 0,9 MB
```

Para falar, a voz do sistema já funciona (`"motor": "sistema"`). Com a CLI `mmx` logada ele usa vozes mais naturais (`mn vozes` lista, `mn vozes usar <nome>` troca); `tom` (-12 a 12) e `velocidade` (0.5 a 2) afinam.

No Windows, descubra o nome do microfone com `ffmpeg -list_devices true -f dshow -i dummy` e ponha em `voz.microfone`.

## Uso

```bash
mn rosto        # abre o orbe no navegador e liga a escuta
```

- **"Meia-noite."** → toca um aviso, o orbe vira `ouvindo`, e ele espera o pedido por 10 s. Se você não disser nada, ele avisa que não ouviu.
- **"Meia-noite, que horas são?"** → atende direto.
- Ele espera você terminar: só considera a frase encerrada depois de 1,2 s de silêncio.
- **Enter** (ou clique) na página → o mesmo que chamar. Enter enquanto ele fala interrompe.
- **Alterar arquivos** → ele diz o que vai mandar fazer e espera **"meia-noite, sim"**.

Pelo terminal, sem voz:

```bash
mn conversar "manda o codex resumir o projeto loja"
mn conversar                 # conversa contínua ("sair" encerra, "nova" esquece)
mn conversar --editar        # deixa ele disparar IAs que alteram arquivos
```

Direto nas mãos, sem cérebro:

```bash
mn ias                                              # quais IAs estão instaladas
mn disparar codex ~/projetos/loja "analisa os riscos"            # modo leitura
mn disparar claude ~/projetos/loja "corrige o teste Y" --editar
mn status | mn status <id> | mn resultado <id> | mn cancelar <id> | mn parar-tudo
```

Voz:

```bash
mn voz                    # conversa falada no terminal (Enter para falar)
mn falar "bom dia"        # só fala
mn ouvir                  # só ouve e mostra o que entendeu
mn vozes                  # lista as vozes em português (motor mmx)
mn ouvido parar           # desliga o transcritor (libera ~700 MB)
```

## Como funciona

```
 microfone ─► recorte de falas ─► transcrição local ─► começa com "meia-noite"?
                                   (whisper.cpp)              │ não: descarta
                                                              ▼ sim
            fala ◄── resposta ◄── cérebro (sua API + ferramentas) ──► mãos ──► CLIs das IAs
           (voz)                   │        │         │                           │
                                   │        │         └──► oficina (scripts)      │
                     memória curta ┘        └──► Nexusyn (memória longa)   estado em arquivos
                                                                                  │
                                                 orbe no navegador ◄──────────────┘
```

- **Mãos** (`internal/nucleo`): cada tarefa tem um processo vigia que roda a IA num grupo próprio e grava estado e saída em arquivos. Não há daemon.
- **Cérebro** (`internal/cerebro`): um laço modelo ⇄ ferramentas, com adaptadores para o formato OpenAI, o formato Anthropic e a CLI `mmx` (`provedor.go`). Guarda as últimas 40 mensagens.
- **Memória** (`internal/memoria`, `internal/oficina`): Nexusyn pelo endpoint MCP, só leitura; notas curtas num arquivo local.
- **Oficina** (`internal/oficina`): scripts `.sh` e `.py` numa pasta própria. No macOS rodam dentro do `sandbox-exec`: sem rede, gravando só na oficina, sem ler pastas de chaves nem os dados do próprio meia-noite, 60 s de limite.
- **Voz** (`internal/voz`): `ffmpeg` captura o microfone; um recortador com limiar que acompanha o ruído do ambiente separa as falas; o `whisper-server` (com filtro Silero) transcreve por um socket de arquivo, sem porta de rede (no Windows, uma porta em `127.0.0.1`).
- **Rosto** (`internal/rosto`): página WebGL2 sem biblioteca, embutida no binário, servida só em `127.0.0.1`.
- **Sistema** (`internal/so`): o que muda entre macOS, Linux e Windows.
- **MCP** (`internal/mcp`): `mn mcp` por stdio, com `listar_ias`, `disparar`, `status`, `resultado`, `cancelar`.

```json
{ "mcpServers": { "meia-noite": { "command": "/caminho/para/mn", "args": ["mcp"] } } }
```

### Como cada IA é chamada

| IA | Comando por baixo | Leitura | Edição |
|---|---|---|---|
| `cursor` | `agent -p --output-format stream-json --trust --workspace` | `--mode plan` | não liberada (exigiria `--force`) |
| `claude` | `claude -p --output-format stream-json --verbose` | `--permission-mode plan` | `--permission-mode acceptEdits` |
| `codex` | `codex exec --json --skip-git-repo-check -C -o` | `--sandbox read-only` | `--sandbox workspace-write` |
| `agy` | `agy --output-format stream-json --add-dir --print-timeout -p` | `--mode plan` | `--mode accept-edits` |
| `opencode` | `opencode run --format json -m <modelo>` | `--agent plan` | agente padrão |

### Estados do orbe

| Estado | Quando | Como fica |
|---|---|---|
| `parado` | esperando a chamada | azul em cima, laranja embaixo, giro lento |
| `ouvindo` | esperando ou gravando o pedido | azul-claro e lilás |
| `pensando` | transcrevendo ou decidindo | roxo, anéis rápidos |
| `falando` | tocando a resposta | laranja pulsando |
| `trabalhando` | há IA rodando | um ponto de luz em órbita por tarefa |
| `erro` | algo falhou há pouco | vermelho |

## Segurança

Um assistente que ouve o ambiente e dispara programas precisa de travas que não dependam do modelo se comportar.

- **Lista fechada de IAs.** O nome vira um binário fixo com argumentos fixos. Nunca texto livre, nunca shell.
- **Pastas permitidas.** Só roda dentro de `pastas_permitidas` (links resolvidos, comparação por componente). `.ssh`, `.gnupg`, `.aws`, `.kube`, `Keychains` são negadas sempre. Lista vazia significa "nenhuma pasta".
- **Leitura por padrão.** No terminal, edição só com `--editar`. Pela voz, só depois de ele dizer o que vai fazer e ouvir **"meia-noite, sim"** — um "sim" solto não vale, porque pode ter vindo da TV ou de outra pessoa na sala. A confirmação vale para aquela IA e aquele projeto, naquele momento. `"editar": "nao"` proíbe de vez.
- **Scripts em caixa fechada.** O que ele escreve na oficina só roda com confirmação e, no macOS, sem rede e sem poder gravar fora da própria pasta. Onde não há caixa fechada, ele não roda.
- **Nunca desliga as travas das CLIs.** `--force`, `--yolo`, `--dangerously-*`, `--auto` não entram em nenhuma linha de comando; há teste que garante.
- **Limites.** 3 tarefas simultâneas, 40 por dia, 30 minutos por tarefa, 8 chamadas ao modelo por pedido, 400 por dia. Zero no config não vira "sem limite".
- **Rede mínima.** Só o rosto abre porta, e só em `127.0.0.1`: recusa `Host` estranho (DNS rebinding), manda CSP `default-src 'self'`, e a única ação (`POST /tocar`, o Enter) não tem parâmetros e exige a origem da própria página.
- **Texto das IAs é dado, não ordem.** O que volta das tarefas entra na conversa do modelo, que é instruído a não obedecer; e as travas acima valem mesmo se ele for enganado.
- **Segredos.** Nenhuma chave em arquivo de texto: a do cérebro fica no cofre do sistema (ou variável de ambiente, ou 1Password) e só é enviada por `https`. O que vai para o modelo e para a memória curta passa por um filtro que troca por `[oculto]` o que parece chave, token ou senha.
- **Microfone.** Com a escuta por chamada ligada, o microfone fica aberto enquanto `mn rosto` roda. Conversa que não chama pelo meia-noite é transcrita localmente e descartada: não vai ao modelo nem é gravada. `"escuta": "enter"` desliga.

Um limite conhecido: depois de chamado, ele aceita o pedido de **leitura** de qualquer voz que ouvir. Não há reconhecimento de quem está falando.

## Dados

Tudo fica numa pasta só (`mn config` mostra qual): `~/.meia-noite` por padrão, ou a da variável `MN_HOME`, ou o caminho escrito em `~/.config/meia-noite/casa`.

- `config.json` — suas escolhas.
- `tarefas/<id>/` — estado, resposta e saída crua de cada tarefa.
- `conversa.json` — a conversa em curso (`mn conversar --nova` apaga).
- `memoria-curta.json` — as notas curtas.
- `oficina/` — os scripts dele.
- `modelos/` — modelos de transcrição.

## Desempenho medido (macOS, Apple M5, 16 GB)

| Etapa | Tempo |
|---|---|
| Transcrever uma frase (serviço no ar) | ~1 s |
| Cérebro decidir, por API direta | 1,5 a 3 s |
| Primeiro som da resposta | ~1 s depois da decisão |
| Consulta ao Nexusyn | 6 a 9 s a mais (ele avisa que está consultando) |
| A IA disparada em si | de 10 s a minutos; ele não fica esperando calado |

## O que falta

- **Linux e Windows de verdade.** Compilam; ninguém usou ainda. Microfone, voz e cofre nesses sistemas foram escritos sem máquina para testar.
- **Palavra de chamada dedicada.** Hoje ele transcreve tudo o que ouve e procura "meia-noite" no texto. Funciona, mas usa a GPU sempre que há alguém falando por perto.
- **Interromper falando.** Enquanto ele fala, o microfone é ignorado (senão ele se ouviria). Para cortar a fala, só o Enter. Falta cancelamento de eco.
- **Reconhecer quem fala.** Qualquer voz vale para pedidos de leitura.
- **Cérebro testado em poucos provedores.** Os formatos OpenAI e Anthropic foram testados de verdade contra um provedor só (nos dois formatos) e por testes automáticos; cada API tem suas manias. Sem resposta em fluxo (*streaming*) ainda.
- **Voz natural só com a MiniMax.** Falta porta para outros serviços de voz e para voz neural local.
- **Caixa fechada só no macOS.** No Linux dá para fazer com `bwrap`; no Windows ainda não sei.
- **Só enxerga o Claude Code** entre as IAs que você abre por conta própria. As demais só aparecem quando foi ele quem disparou.
- **"Claude Code" às vezes sai "Cloud Code"** na transcrição.
- **Cursor só em leitura**; Antigravity em leitura fica `bloqueada` quando tenta rodar comando que pede permissão.
- **Sem fila**: acima do limite de tarefas, o disparo é recusado.
- **Não inicia com o computador**; você roda `mn rosto`.
- **Sem testes de ponta a ponta com áudio.**

## Contribuir

Veja [CONTRIBUTING.md](CONTRIBUTING.md). Em resumo: `gofmt`, `go vet`, `go test`, português, e nenhuma trava de segurança sai sem teste.

## Licença

[MIT](LICENSE).
