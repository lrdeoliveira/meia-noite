package nucleo

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Sessao é o resumo de uma sessão do Claude Code aberta neste Mac
// (não as tarefas que o meia-noite disparou: essas estão em Listar).
type Sessao struct {
	Pasta        string `json:"pasta"`
	UltimaAcao   string `json:"ultima_atividade"`
	MinutosAtras int    `json:"minutos_atras"`
	Pedido       string `json:"ultimo_pedido_do_usuario,omitempty"`
	Resposta     string `json:"ultima_fala_da_ia,omitempty"`
	Ferramenta   string `json:"ultima_ferramenta_usada,omitempty"`
}

// PastaSessoesClaude é onde o Claude Code grava as conversas.
func PastaSessoesClaude() string {
	if v := os.Getenv("MN_CLAUDE_PROJETOS"); v != "" {
		return v
	}
	casa, _ := os.UserHomeDir()
	return filepath.Join(casa, ".claude", "projects")
}

var reSegredo = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_\-]{8,}|gh[pousr]_[A-Za-z0-9]{16,}|xox[abprs]-[A-Za-z0-9\-]{8,}|eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{6,}|AKIA[0-9A-Z]{12,}|[A-Za-z0-9+/_\-]{40,}|(senha|password|token|secret|api[_-]?key)\s*[:=]\s*\S+)`)

// Resumir encurta um trecho e tira o que parece segredo: o resumo sai deste
// Mac (vai para o cérebro), então chave e senha não podem ir junto.
func Resumir(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	s = reSegredo.ReplaceAllString(s, "[oculto]")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// SessoesClaude lista as sessões do Claude Code com atividade recente,
// lendo só o fim de cada arquivo de conversa.
func SessoesClaude(dentroDe time.Duration, max int) []Sessao {
	arquivos, _ := filepath.Glob(filepath.Join(PastaSessoesClaude(), "*", "*.jsonl"))
	type item struct {
		caminho string
		mod     time.Time
	}
	var recentes []item
	for _, a := range arquivos {
		info, err := os.Stat(a)
		if err == nil && time.Since(info.ModTime()) <= dentroDe {
			recentes = append(recentes, item{a, info.ModTime()})
		}
	}
	sort.Slice(recentes, func(i, j int) bool { return recentes[i].mod.After(recentes[j].mod) })
	if len(recentes) > max {
		recentes = recentes[:max]
	}
	out := []Sessao{}
	for _, it := range recentes {
		s := lerFimDaSessao(it.caminho)
		s.MinutosAtras = int(time.Since(it.mod).Minutes())
		s.UltimaAcao = it.mod.Format("15:04")
		out = append(out, s)
	}
	return out
}

func lerFimDaSessao(caminho string) Sessao {
	var s Sessao
	f, err := os.Open(caminho)
	if err != nil {
		return s
	}
	defer f.Close()
	const cauda = 400 << 10
	if info, err := f.Stat(); err == nil && info.Size() > cauda {
		_, _ = f.Seek(-cauda, io.SeekEnd)
	}
	leitor := bufio.NewReaderSize(f, 1<<20)
	for {
		linha, err := leitor.ReadBytes('\n')
		if len(bytes.TrimSpace(linha)) > 0 {
			var o struct {
				Type        string `json:"type"`
				Cwd         string `json:"cwd"`
				IsSidechain bool   `json:"isSidechain"`
				Message     struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal(linha, &o) == nil && !o.IsSidechain {
				if o.Cwd != "" {
					s.Pasta = o.Cwd
				}
				switch o.Type {
				case "user":
					if t := textoDoConteudo(o.Message.Content, "text"); t != "" && !strings.HasPrefix(t, "<") {
						s.Pedido = Resumir(t, 220)
					}
				case "assistant":
					if t := textoDoConteudo(o.Message.Content, "text"); t != "" {
						s.Resposta = Resumir(t, 320)
					}
					if n := textoDoConteudo(o.Message.Content, "tool_use"); n != "" {
						s.Ferramenta = n
					}
				}
			}
		}
		if err != nil {
			return s
		}
	}
}

// textoDoConteudo pega o texto (tipo "text") ou o nome da ferramenta (tipo
// "tool_use") do último bloco desse tipo.
func textoDoConteudo(cru json.RawMessage, tipo string) string {
	if len(cru) == 0 {
		return ""
	}
	if cru[0] == '"' {
		var t string
		if tipo == "text" && json.Unmarshal(cru, &t) == nil {
			return t
		}
		return ""
	}
	var blocos []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		Name string `json:"name"`
	}
	if json.Unmarshal(cru, &blocos) != nil {
		return ""
	}
	achado := ""
	for _, b := range blocos {
		if b.Type != tipo {
			continue
		}
		if tipo == "text" && strings.TrimSpace(b.Text) != "" {
			achado = b.Text
		}
		if tipo == "tool_use" {
			achado = b.Name
		}
	}
	return achado
}
