// Package memoria consulta o Nexusyn (nexusyn.ai), uma memória compartilhada
// entre agentes: projetos, decisões, preferências, o que já foi feito.
// Opcional e só leitura.
package memoria

import (
	"bufio"
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
)

// Achado é o que volta de uma consulta.
type Achado struct {
	Resposta string   `json:"resposta"`
	Fontes   []string `json:"fontes,omitempty"`
}

var (
	chaveMu sync.Mutex
	chave   string
)

// Ligada diz se a memória está configurada.
func Ligada(cfg nucleo.Config) bool {
	return cfg.Nexusyn.URL != "" && (cfg.Nexusyn.RefChave != "" || os.Getenv("MN_NEXUSYN_TOKEN") != "")
}

// lerChave busca o token no 1Password (nunca fica em arquivo nem no código)
// e o guarda só na memória deste processo.
func lerChave(ctx context.Context, cfg nucleo.Config) (string, error) {
	chaveMu.Lock()
	defer chaveMu.Unlock()
	if chave != "" {
		return chave, nil
	}
	if v := os.Getenv("MN_NEXUSYN_TOKEN"); v != "" {
		chave = v
		return chave, nil
	}
	ref := cfg.Nexusyn.RefChave
	if !strings.HasPrefix(ref, "op://") {
		return "", errors.New("referência da chave do Nexusyn inválida no config (esperado op://…)")
	}
	bin, err := exec.LookPath("op")
	if err != nil {
		return "", errors.New("1Password CLI (op) não encontrado: é por ele que o meia-noite pega a chave do Nexusyn")
	}
	ctx, cancelar := context.WithTimeout(ctx, 20*time.Second)
	defer cancelar()
	saida, err := exec.CommandContext(ctx, bin, "read", ref).Output()
	if err != nil {
		return "", errors.New("não consegui ler a chave do Nexusyn no 1Password")
	}
	chave = strings.TrimSpace(string(saida))
	if chave == "" {
		return "", errors.New("a chave do Nexusyn veio vazia do 1Password")
	}
	return chave, nil
}

// Buscar pergunta ao Nexusyn e devolve a resposta com os títulos das fontes.
func Buscar(ctx context.Context, cfg nucleo.Config, pergunta string) (*Achado, error) {
	pergunta = strings.TrimSpace(pergunta)
	if pergunta == "" {
		return nil, errors.New("pergunta vazia")
	}
	if r := []rune(pergunta); len(r) > 500 {
		pergunta = string(r[:500])
	}
	u, err := url.Parse(cfg.Nexusyn.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("endereço do Nexusyn inválido no config (precisa ser https)")
	}
	token, err := lerChave(ctx, cfg)
	if err != nil {
		return nil, err
	}
	corpo, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "search_memory", "arguments": map[string]any{"query": pergunta, "limit": 8}},
	})
	ctx, cancelar := context.WithTimeout(ctx, 45*time.Second)
	defer cancelar()
	req, _ := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(corpo))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.New("o Nexusyn não respondeu")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		chaveMu.Lock()
		chave = ""
		chaveMu.Unlock()
		return nil, errors.New("o Nexusyn recusou a chave (confira o item do 1Password indicado em nexusyn.ref_chave)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("o Nexusyn respondeu com erro %d", resp.StatusCode)
	}
	return lerResposta(io.LimitReader(resp.Body, 4<<20))
}

// lerResposta aceita tanto JSON puro quanto o formato de eventos ("data: …").
func lerResposta(r io.Reader) (*Achado, error) {
	type envelope struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	var env envelope
	achou := false
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		linha := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "data:"))
		if !strings.HasPrefix(linha, "{") {
			continue
		}
		var e envelope
		if json.Unmarshal([]byte(linha), &e) == nil && (len(e.Result.Content) > 0 || e.Error != nil) {
			env, achou = e, true
		}
	}
	if !achou {
		return nil, errors.New("resposta do Nexusyn ilegível")
	}
	if env.Error != nil {
		return nil, errors.New("o Nexusyn devolveu erro: " + env.Error.Message)
	}
	texto := env.Result.Content[0].Text
	if env.Result.IsError {
		return nil, errors.New("o Nexusyn devolveu erro: " + nucleo.Resumir(texto, 200))
	}
	var dado struct {
		Answer  string `json:"answer"`
		Sources []struct {
			Title   string `json:"title"`
			Project string `json:"project"`
		} `json:"sources"`
	}
	if json.Unmarshal([]byte(texto), &dado) != nil {
		dado.Answer = texto
	}
	a := &Achado{Resposta: strings.TrimSpace(dado.Answer)}
	if r := []rune(a.Resposta); len(r) > 2500 {
		a.Resposta = string(r[:2500]) + "…"
	}
	for i, f := range dado.Sources {
		if i >= 5 {
			break
		}
		t := f.Title
		if f.Project != "" {
			t += " (" + f.Project + ")"
		}
		a.Fontes = append(a.Fontes, t)
	}
	if a.Resposta == "" {
		a.Resposta = "Não encontrei nada sobre isso na memória."
	}
	return a, nil
}
