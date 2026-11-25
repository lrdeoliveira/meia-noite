// Package mcp expõe as mãos do meia-noite como servidor MCP por stdio
// (um JSON por linha). Não abre porta de rede.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
)

type pedido struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type resposta struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *erroRPC        `json:"error,omitempty"`
}

type erroRPC struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type esquema map[string]any

func texto(desc string) esquema { return esquema{"type": "string", "description": desc} }

func objeto(props esquema, obrig ...string) esquema {
	if obrig == nil {
		obrig = []string{}
	}
	return esquema{"type": "object", "properties": props, "required": obrig, "additionalProperties": false}
}

var ferramentas = []esquema{
	{
		"name":        "listar_ias",
		"description": "Lista as IAs de código que o meia-noite pode disparar neste Mac, se estão instaladas e se aceitam modo edição.",
		"inputSchema": objeto(esquema{}),
	},
	{
		"name": "disparar",
		"description": "Manda uma IA de código trabalhar numa pasta. Volta na hora com o id da tarefa; a IA segue em segundo plano. " +
			"Use modo 'leitura' para analisar sem alterar nada (padrão) e 'edicao' só quando o usuário pediu mudança em arquivos. " +
			"Acompanhe com 'status' e leia a resposta com 'resultado'.",
		"inputSchema": objeto(esquema{
			"ia":     esquema{"type": "string", "enum": []string{"agy", "claude", "codex", "cursor", "opencode"}, "description": "Qual IA disparar."},
			"pasta":  texto("Caminho absoluto da pasta de trabalho. Precisa estar na lista de pastas permitidas."),
			"pedido": texto("O que a IA deve fazer, em texto corrido. Seja específico e curto."),
			"modo":   esquema{"type": "string", "enum": []string{"leitura", "edicao"}, "description": "leitura (padrão) ou edicao."},
		}, "ia", "pasta", "pedido"),
	},
	{
		"name":        "status",
		"description": "Mostra o estado de uma tarefa (com id) ou das tarefas mais recentes (sem id). Estados: rodando, terminou, erro, cancelada, bloqueada, tempo_esgotado.",
		"inputSchema": objeto(esquema{"id": texto("Id da tarefa. Vazio lista as 10 mais recentes.")}),
	},
	{
		"name":        "resultado",
		"description": "Devolve a resposta final de uma tarefa encerrada, ou o erro. Se ainda estiver rodando, diz isso.",
		"inputSchema": objeto(esquema{"id": texto("Id da tarefa.")}, "id"),
	},
	{
		"name":        "cancelar",
		"description": "Cancela uma tarefa que está rodando. Com id 'todas', cancela todas as tarefas em andamento.",
		"inputSchema": objeto(esquema{"id": texto("Id da tarefa, ou 'todas'.")}, "id"),
	},
}

// Servir atende pedidos MCP até a entrada fechar.
func Servir(entrada io.Reader, saida io.Writer, versao string) error {
	leitor := bufio.NewReaderSize(entrada, 1<<20)
	escritor := json.NewEncoder(saida)
	for {
		linha, err := leitor.ReadBytes('\n')
		if len(linha) > 1 {
			var p pedido
			if json.Unmarshal(linha, &p) != nil {
				_ = escritor.Encode(resposta{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &erroRPC{-32700, "JSON inválido"}})
			} else if p.ID != nil { // sem id é notificação: não se responde
				r := resposta{JSONRPC: "2.0", ID: p.ID}
				r.Result, r.Error = atender(p, versao)
				_ = escritor.Encode(r)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func atender(p pedido, versao string) (any, *erroRPC) {
	switch p.Method {
	case "initialize":
		var in struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(p.Params, &in)
		if in.ProtocolVersion == "" {
			in.ProtocolVersion = "2025-06-18"
		}
		return esquema{
			"protocolVersion": in.ProtocolVersion,
			"capabilities":    esquema{"tools": esquema{}},
			"serverInfo":      esquema{"name": "meia-noite", "version": versao},
			"instructions":    "Mãos do meia-noite: dispara e acompanha IAs de código já instaladas neste Mac. Padrão é modo leitura.",
		}, nil
	case "ping":
		return esquema{}, nil
	case "tools/list":
		return esquema{"tools": ferramentas}, nil
	case "tools/call":
		var in struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		if err := json.Unmarshal(p.Params, &in); err != nil {
			return nil, &erroRPC{-32602, "parâmetros inválidos: os argumentos devem ser textos"}
		}
		dado, err := Chamar(in.Name, in.Arguments)
		if err != nil {
			return esquema{"content": []esquema{{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		b, _ := json.MarshalIndent(dado, "", "  ")
		return esquema{"content": []esquema{{"type": "text", "text": string(b)}}}, nil
	}
	return nil, &erroRPC{-32601, "método desconhecido: " + p.Method}
}

// Ferramentas devolve a lista de ferramentas, para o cérebro oferecer as mesmas.
func Ferramentas() []map[string]any {
	out := make([]map[string]any, len(ferramentas))
	for i, f := range ferramentas {
		out[i] = f
	}
	return out
}

// Chamar executa uma ferramenta pedida por um cliente MCP.
func Chamar(nome string, a map[string]string) (any, error) { return ChamarDe("mcp", nome, a) }

// ChamarDe executa uma ferramenta registrando quem pediu (mcp, conversa).
func ChamarDe(origem, nome string, a map[string]string) (any, error) {
	switch nome {
	case "listar_ias":
		return nucleo.ListarIAs(), nil
	case "disparar":
		cfg, err := nucleo.CarregarConfig()
		if err != nil {
			return nil, err
		}
		t, err := nucleo.Disparar(cfg, a["ia"], a["pasta"], a["pedido"], a["modo"], origem)
		if err != nil {
			return nil, err
		}
		return esquema{"id": t.ID, "estado": t.Estado, "ia": t.IA, "pasta": t.Pasta, "modo": t.Modo}, nil
	case "status":
		if a["id"] == "" {
			return resumir(nucleo.Listar(10)), nil
		}
		t, err := nucleo.Ler(a["id"])
		if err != nil {
			return nil, err
		}
		return resumir([]*nucleo.Tarefa{t})[0], nil
	case "resultado":
		t, err := nucleo.Ler(a["id"])
		if err != nil {
			return nil, err
		}
		if t.Ativa() {
			return esquema{"id": t.ID, "estado": t.Estado, "aviso": fmt.Sprintf("ainda rodando (%d passos até agora)", t.Passos)}, nil
		}
		return esquema{"id": t.ID, "estado": t.Estado, "ia": t.IA, "resposta": t.Resposta, "erro": t.Erro}, nil
	case "cancelar":
		if a["id"] == "todas" {
			return resumir(nucleo.PararTudo()), nil
		}
		t, err := nucleo.Cancelar(a["id"])
		if err != nil {
			return nil, err
		}
		return resumir([]*nucleo.Tarefa{t})[0], nil
	}
	return nil, fmt.Errorf("ferramenta desconhecida: %s", nome)
}

// resumir tira o que é grande (resposta inteira) da visão de status.
func resumir(ts []*nucleo.Tarefa) []esquema {
	out := []esquema{}
	for _, t := range ts {
		pedido := t.Pedido
		if r := []rune(pedido); len(r) > 120 {
			pedido = string(r[:120]) + "…"
		}
		e := esquema{"id": t.ID, "ia": t.IA, "estado": t.Estado, "modo": t.Modo, "pasta": t.Pasta,
			"pedido": pedido, "passos": t.Passos, "criada": t.Criada.Format("2006-01-02 15:04:05")}
		if t.Erro != "" {
			e["erro"] = t.Erro
		}
		out = append(out, e)
	}
	return out
}
