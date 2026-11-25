package nucleo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type Modo string

const (
	Leitura Modo = "leitura"
	Edicao  Modo = "edicao"
)

// Parcial é o que se extrai da saída da CLI enquanto ela roda.
type Parcial struct {
	Texto     string
	Erro      string
	Sessao    string
	Passos    int
	CustoUSD  float64
	Bloqueada string // ação negada por falta de permissão
}

// ParamsArgs é tudo que uma IA precisa para montar a linha de comando.
type ParamsArgs struct {
	Modo       Modo
	Pasta      string
	Pedido     string
	Modelo     string
	ArqFinal   string // onde a CLI grava a última mensagem (quando suporta)
	LimiteSegs int
}

// IA descreve uma CLI de agente: binário fixo, argumentos fixos por modo e
// como ler a saída. Nada aqui vem de texto livre do usuário além do pedido,
// que entra sempre como UM argumento.
type IA struct {
	Nome      string
	Descricao string
	Bin       string
	Args      func(p ParamsArgs) ([]string, error)
	Ler       func(linha []byte, r *Parcial)
}

// Catalogo é a lista fechada. Quem não está aqui não roda.
var Catalogo = map[string]IA{
	"cursor": {
		Nome: "cursor", Descricao: "Cursor Agent", Bin: "agent",
		Args: func(p ParamsArgs) ([]string, error) {
			if p.Modo == Edicao {
				// Sem tela, o Cursor só aplica edições com --force, que libera
				// também comandos. Fica para a etapa com confirmação na tela.
				return nil, errors.New("o Cursor só está liberado em modo leitura nesta etapa")
			}
			a := []string{"-p", "--output-format", "stream-json", "--mode", "plan", "--trust", "--workspace", p.Pasta}
			if p.Modelo != "" {
				a = append(a, "--model", p.Modelo)
			}
			return append(a, p.Pedido), nil
		},
		Ler: lerEstiloClaude,
	},
	"claude": {
		Nome: "claude", Descricao: "Claude Code", Bin: "claude",
		Args: func(p ParamsArgs) ([]string, error) {
			perm := "plan"
			if p.Modo == Edicao {
				perm = "acceptEdits"
			}
			a := []string{"-p", "--output-format", "stream-json", "--verbose", "--permission-mode", perm}
			if p.Modelo != "" {
				a = append(a, "--model", p.Modelo)
			}
			return append(a, p.Pedido), nil
		},
		Ler: lerEstiloClaude,
	},
	"codex": {
		Nome: "codex", Descricao: "Codex", Bin: "codex",
		Args: func(p ParamsArgs) ([]string, error) {
			caixa := "read-only"
			if p.Modo == Edicao {
				caixa = "workspace-write"
			}
			a := []string{"exec", "--json", "--sandbox", caixa, "--skip-git-repo-check", "-C", p.Pasta, "-o", p.ArqFinal}
			if p.Modelo != "" {
				a = append(a, "-m", p.Modelo)
			}
			return append(a, p.Pedido), nil
		},
		Ler: lerCodex,
	},
	"agy": {
		Nome: "agy", Descricao: "Antigravity", Bin: "agy",
		Args: func(p ParamsArgs) ([]string, error) {
			modo := "plan"
			if p.Modo == Edicao {
				modo = "accept-edits"
			}
			a := []string{"--output-format", "stream-json", "--mode", modo, "--add-dir", p.Pasta,
				"--print-timeout", fmt.Sprintf("%ds", p.LimiteSegs)}
			if p.Modelo != "" {
				a = append(a, "--model", p.Modelo)
			}
			// No agy o pedido é o valor de -p, então vai por último e colado à opção.
			return append(a, "-p", p.Pedido), nil
		},
		Ler: lerAgy,
	},
	"opencode": {
		Nome: "opencode", Descricao: "OpenCode", Bin: "opencode",
		Args: func(p ParamsArgs) ([]string, error) {
			a := []string{"run", "--format", "json"}
			if p.Modo == Leitura {
				a = append(a, "--agent", "plan")
			}
			if p.Modelo != "" {
				a = append(a, "-m", p.Modelo)
			}
			return append(a, p.Pedido), nil
		},
		Ler: lerOpencode,
	},
}

// Opções que desligam as travas das CLIs. Nunca entram numa linha de comando
// montada pelo meia-noite; há teste que garante isso.
var OpcoesProibidas = []string{
	"--force", "--yolo", "-f",
	"--dangerously-skip-permissions",
	"--dangerously-bypass-approvals-and-sandbox",
	"--dangerously-bypass-hook-trust",
	"--auto",
	"bypassPermissions", "danger-full-access",
}

type EstadoIA struct {
	Nome      string `json:"nome"`
	Descricao string `json:"descricao"`
	Instalada bool   `json:"instalada"`
	Caminho   string `json:"caminho,omitempty"`
	Edita     bool   `json:"aceita_edicao"`
}

// ListarIAs diz quais IAs do catálogo estão instaladas neste Mac.
func ListarIAs() []EstadoIA {
	var out []EstadoIA
	for _, ia := range Catalogo {
		e := EstadoIA{Nome: ia.Nome, Descricao: ia.Descricao}
		if c, err := exec.LookPath(ia.Bin); err == nil {
			e.Instalada, e.Caminho = true, c
		}
		_, err := ia.Args(ParamsArgs{Modo: Edicao, Pasta: "/", Pedido: "x", ArqFinal: "/dev/null", LimiteSegs: 60})
		e.Edita = err == nil
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Nome < out[j].Nome })
	return out
}

// ---- leitores de saída (um JSON por linha) ----

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func obj(m map[string]any, k string) map[string]any {
	o, _ := m[k].(map[string]any)
	return o
}

// Cursor e Claude Code terminam com {"type":"result","result":"...","is_error":bool}.
func lerEstiloClaude(linha []byte, r *Parcial) {
	var m map[string]any
	if json.Unmarshal(linha, &m) != nil {
		return
	}
	if s := str(m, "session_id"); s != "" {
		r.Sessao = s
	}
	switch str(m, "type") {
	case "tool_call":
		if str(m, "subtype") == "started" {
			r.Passos++
		}
	case "assistant":
		if conteudo, ok := obj(m, "message")["content"].([]any); ok {
			for _, c := range conteudo {
				if b, ok := c.(map[string]any); ok && str(b, "type") == "tool_use" {
					r.Passos++
				}
			}
		}
	case "result":
		texto := str(m, "result")
		if erro, _ := m["is_error"].(bool); erro {
			r.Erro = texto
			if r.Erro == "" {
				r.Erro = "a IA terminou com erro (" + str(m, "subtype") + ")"
			}
		} else {
			r.Texto = texto
		}
		if c, ok := m["total_cost_usd"].(float64); ok {
			r.CustoUSD = c
		}
	}
}

func lerCodex(linha []byte, r *Parcial) {
	var m map[string]any
	if json.Unmarshal(linha, &m) != nil {
		return
	}
	item := obj(m, "item")
	switch str(m, "type") {
	case "thread.started":
		r.Sessao = str(m, "thread_id")
	case "item.started":
		if str(item, "type") != "agent_message" {
			r.Passos++
		}
	case "item.completed":
		if str(item, "type") == "agent_message" {
			r.Texto = str(item, "text")
		}
	case "error":
		r.Erro = str(m, "message")
	case "turn.failed":
		r.Erro = str(obj(m, "error"), "message")
	}
}

func lerAgy(linha []byte, r *Parcial) {
	var m map[string]any
	if json.Unmarshal(linha, &m) != nil {
		return
	}
	switch str(m, "event") {
	case "init":
		r.Sessao = str(m, "conversation_id")
	case "step_update":
		s := obj(m, "step_update")
		if str(s, "step_type") == "tool" && str(s, "state") == "ACTIVE" {
			r.Passos++
		}
	case "result":
		res := obj(m, "result")
		r.Texto = str(res, "response")
		if st := str(res, "status"); st != "" && st != "SUCCESS" {
			r.Erro = "o Antigravity terminou com estado " + st
		}
		if negadas, ok := res["denied_actions"].([]any); ok && len(negadas) > 0 && strings.TrimSpace(r.Texto) == "" {
			var nomes []string
			for _, n := range negadas {
				if d, ok := n.(map[string]any); ok {
					nome := str(d, "display_name")
					if nome == "" {
						nome = str(d, "action")
					}
					nomes = append(nomes, nome)
				}
			}
			r.Bloqueada = "o Antigravity precisou de uma permissão que não pode pedir sem tela: " + strings.Join(nomes, ", ")
		}
	}
}

func lerOpencode(linha []byte, r *Parcial) {
	var m map[string]any
	if json.Unmarshal(linha, &m) != nil {
		return
	}
	if s := str(m, "sessionID"); s != "" {
		r.Sessao = s
	}
	parte := obj(m, "part")
	switch str(m, "type") {
	case "tool_use":
		r.Passos++
	case "text":
		// Cada bloco de texto substitui o anterior: o último é a resposta final.
		if t := str(parte, "text"); t != "" {
			r.Texto = t
		}
	case "step_finish":
		if c, ok := parte["cost"].(float64); ok {
			r.CustoUSD += c
		}
	case "error":
		r.Erro = str(obj(m, "error"), "message")
		if r.Erro == "" {
			r.Erro = "o OpenCode devolveu erro"
		}
	}
}
