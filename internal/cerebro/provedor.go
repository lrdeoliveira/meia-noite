package cerebro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
	"github.com/lrdeoliveira/meia-noite/internal/so"
)

// O cérebro pode ser qualquer API de conversa com ferramentas:
//
//	tipo "openai"    — formato /chat/completions (a maioria dos provedores e
//	                   dos servidores locais, como Ollama e LM Studio)
//	tipo "anthropic" — formato /v1/messages
//	tipo "mmx"       — comando mmx já logado (não precisa de chave aqui)
//
// A chave nunca fica no config: vem de variável de ambiente, do 1Password
// (referência op://) ou do cofre do sistema (cadastre com `mn chave`).

var (
	chaveMu sync.Mutex
	chave   string
)

func ehLocal(u *url.URL) bool {
	h := u.Hostname()
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// validarURL exige https, a não ser para um modelo rodando neste Mac.
func validarURL(cru string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(cru, "/"))
	if err != nil || u.Host == "" {
		return nil, errors.New("endereço do cérebro inválido no config (cerebro.url)")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && ehLocal(u)) {
		return nil, errors.New("endereço do cérebro precisa ser https (http só para 127.0.0.1/localhost)")
	}
	return u, nil
}

// lerChave procura a chave do cérebro: variável de ambiente, 1Password, Chaves do macOS.
func lerChave(ctx context.Context, c nucleo.Cerebro) (string, error) {
	chaveMu.Lock()
	defer chaveMu.Unlock()
	if chave != "" {
		return chave, nil
	}
	env := c.ChaveEnv
	if env == "" {
		env = "MN_CEREBRO_CHAVE"
	}
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		chave = v
		return chave, nil
	}
	ctx, cancelar := context.WithTimeout(ctx, 20*time.Second)
	defer cancelar()
	if strings.HasPrefix(c.RefChave, "op://") {
		if bin, err := exec.LookPath("op"); err == nil {
			if b, err := exec.CommandContext(ctx, bin, "read", c.RefChave).Output(); err == nil {
				if chave = strings.TrimSpace(string(b)); chave != "" {
					return chave, nil
				}
			}
		}
		return "", errors.New("não consegui ler a chave do cérebro no 1Password (cerebro.ref_chave)")
	}
	if k, err := so.LerChave(ctx, nucleo.Casa()); err == nil {
		chave = k
		return chave, nil
	}
	return "", errors.New("o cérebro não tem chave cadastrada. Cadastre com: mn chave   (ou defina a variável " + env + ")")
}

// EscolherModelo monta a chamada ao cérebro conforme o config.
func EscolherModelo(cfg nucleo.Config) Modelo {
	c := cfg.Cerebro
	modelo := c.Modelo
	if modelo == "" {
		modelo = cfg.Modelos["cerebro"] // nome antigo do campo
	}
	switch c.Tipo {
	case "mmx":
		return modeloMMX(modelo)
	case "openai":
		return modeloHTTP(c, modelo, chamarOpenAI)
	case "anthropic":
		return modeloHTTP(c, modelo, chamarAnthropic)
	}
	return func(context.Context, string, []map[string]any, []Mensagem) (*Resposta, error) {
		return nil, errors.New(`o cérebro não está configurado. No config.json, preencha "cerebro" (tipo "openai", "anthropic" ou "mmx", url e modelo) e cadastre a chave com: mn chave`)
	}
}

type chamada func(ctx context.Context, base *url.URL, chave, modelo, sistema string, ferr []map[string]any, msgs []Mensagem) (*Resposta, error)

func modeloHTTP(c nucleo.Cerebro, modelo string, chamar chamada) Modelo {
	return func(ctx context.Context, sistema string, ferr []map[string]any, msgs []Mensagem) (*Resposta, error) {
		base, err := validarURL(c.URL)
		if err != nil {
			return nil, err
		}
		if modelo == "" {
			return nil, errors.New("falta o nome do modelo no config (cerebro.modelo)")
		}
		k, err := lerChave(ctx, c)
		if err != nil && !ehLocal(base) { // modelo local costuma não pedir chave
			return nil, err
		}
		ctx, cancelar := context.WithTimeout(ctx, (tempoChamadaSeg+5)*time.Second)
		defer cancelar()
		return chamar(ctx, base, k, modelo, sistema, ferr, msgs)
	}
}

func postar(ctx context.Context, endereco string, cabecalhos map[string]string, corpo any, saida any) error {
	b, _ := json.Marshal(corpo)
	req, err := http.NewRequestWithContext(ctx, "POST", endereco, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cabecalhos {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return errors.New("o cérebro demorou demais para responder")
		}
		return errors.New("não consegui falar com o cérebro: " + nucleo.Resumir(err.Error(), 160))
	}
	defer resp.Body.Close()
	cru, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		chaveMu.Lock()
		chave = ""
		chaveMu.Unlock()
		return errors.New("o cérebro recusou a chave (confira com: mn chave)")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("o cérebro respondeu com erro %d: %s", resp.StatusCode, nucleo.Resumir(string(cru), 200))
	}
	if json.Unmarshal(cru, saida) != nil {
		return errors.New("o cérebro devolveu uma resposta que não consegui ler")
	}
	return nil
}

// ---------- formato anthropic (/v1/messages): é o formato interno, quase direto

func chamarAnthropic(ctx context.Context, base *url.URL, chave, modelo, sistema string, ferr []map[string]any, msgs []Mensagem) (*Resposta, error) {
	corpo := map[string]any{"model": modelo, "max_tokens": 2048, "system": sistema, "tools": ferr, "messages": msgs}
	r := &Resposta{}
	err := postar(ctx, base.String()+"/v1/messages",
		map[string]string{"x-api-key": chave, "Authorization": bearer(chave), "anthropic-version": "2023-06-01"}, corpo, r)
	if err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, errors.New("o cérebro recusou a chamada: " + r.Error.Message)
	}
	if len(r.Content) == 0 {
		return nil, errors.New("o cérebro não respondeu nada")
	}
	return r, nil
}

func bearer(chave string) string {
	if chave == "" {
		return ""
	}
	return "Bearer " + chave
}

// ---------- formato openai (/chat/completions)

type chamadaFerramenta struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// paraOpenAI converte o histórico interno (blocos) para mensagens do formato openai.
func paraOpenAI(sistema string, msgs []Mensagem) []map[string]any {
	out := []map[string]any{{"role": "system", "content": sistema}}
	for _, m := range msgs {
		if len(m.Content) > 0 && m.Content[0] == '"' {
			var t string
			_ = json.Unmarshal(m.Content, &t)
			out = append(out, map[string]any{"role": m.Role, "content": t})
			continue
		}
		var blocos []struct {
			bloco
			ToolUseID string `json:"tool_use_id"`
			Content   any    `json:"content"`
		}
		if json.Unmarshal(m.Content, &blocos) != nil {
			continue
		}
		if m.Role == "assistant" {
			msg := map[string]any{"role": "assistant"}
			var textos []string
			var usos []chamadaFerramenta
			for _, b := range blocos {
				switch b.Type {
				case "text":
					textos = append(textos, b.Text)
				case "tool_use":
					args, _ := json.Marshal(b.Input)
					if b.Input == nil {
						args = []byte("{}")
					}
					u := chamadaFerramenta{ID: b.ID, Type: "function"}
					u.Function.Name, u.Function.Arguments = b.Name, string(args)
					usos = append(usos, u)
				}
			}
			msg["content"] = strings.Join(textos, "\n")
			if len(usos) > 0 {
				msg["tool_calls"] = usos
			}
			out = append(out, msg)
			continue
		}
		for _, b := range blocos {
			if b.Type != "tool_result" {
				continue
			}
			texto, ok := b.Content.(string)
			if !ok {
				cru, _ := json.Marshal(b.Content)
				texto = string(cru)
			}
			out = append(out, map[string]any{"role": "tool", "tool_call_id": b.ToolUseID, "content": texto})
		}
	}
	return out
}

func chamarOpenAI(ctx context.Context, base *url.URL, chave, modelo, sistema string, ferr []map[string]any, msgs []Mensagem) (*Resposta, error) {
	var tools []map[string]any
	for _, f := range ferr {
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": f["name"], "description": f["description"], "parameters": f["input_schema"]}})
	}
	corpo := map[string]any{"model": modelo, "messages": paraOpenAI(sistema, msgs), "tools": tools, "max_tokens": 2048}
	var r struct {
		Choices []struct {
			Message struct {
				Content   *string             `json:"content"`
				ToolCalls []chamadaFerramenta `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := postar(ctx, base.String()+"/chat/completions", map[string]string{"Authorization": bearer(chave)}, corpo, &r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, errors.New("o cérebro recusou a chamada: " + r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return nil, errors.New("o cérebro não respondeu nada")
	}
	m := r.Choices[0].Message
	var blocos []bloco
	if m.Content != nil {
		// alguns modelos deixam o raciocínio no texto entre <think>…</think>
		texto := *m.Content
		if i, j := strings.Index(texto, "<think>"), strings.LastIndex(texto, "</think>"); i >= 0 && j > i {
			texto = texto[:i] + texto[j+len("</think>"):]
		}
		if strings.TrimSpace(texto) != "" {
			blocos = append(blocos, bloco{Type: "text", Text: strings.TrimSpace(texto)})
		}
	}
	for i, u := range m.ToolCalls {
		entrada := map[string]any{}
		_ = json.Unmarshal([]byte(u.Function.Arguments), &entrada)
		id := u.ID
		if id == "" {
			id = fmt.Sprintf("uso_%d", i)
		}
		blocos = append(blocos, bloco{Type: "tool_use", ID: id, Name: u.Function.Name, Input: entrada})
	}
	if len(blocos) == 0 {
		return nil, errors.New("o cérebro não respondeu nada")
	}
	cru, _ := json.Marshal(blocos)
	return &Resposta{Content: cru, StopReason: r.Choices[0].FinishReason}, nil
}
