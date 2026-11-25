package cerebro

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
)

func historico() []Mensagem {
	return []Mensagem{
		{Role: "user", Content: json.RawMessage(`"resume o projeto loja"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"hmm","signature":"s"},{"type":"text","text":"Vou ver."},{"type":"tool_use","id":"t1","name":"listar_pastas","input":{"filtro":"loja"}}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"t1","content":"{\"pastas\":[\"/p/loja\"]}"}]`)},
	}
}

func TestOpenAIConverteIdaEVolta(t *testing.T) {
	var recebido map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("caminho errado: %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &recebido)
		io.WriteString(w, `{"choices":[{"finish_reason":"tool_calls","message":{"content":"<think>pensando</think>Certo.","tool_calls":[{"id":"c9","type":"function","function":{"name":"disparar","arguments":"{\"ia\":\"codex\",\"pasta\":\"/p/loja\",\"pedido\":\"resuma\"}"}}]}}]}`)
	}))
	defer srv.Close()
	t.Setenv("MN_CEREBRO_CHAVE", "chave-de-teste")
	chave = ""
	cfg := nucleo.ConfigPadrao()
	cfg.Cerebro = nucleo.Cerebro{Tipo: "openai", URL: srv.URL + "/v1/", Modelo: "modelo-x"}
	fs := []map[string]any{{"name": "disparar", "description": "d", "input_schema": map[string]any{"type": "object"}}}
	r, err := EscolherModelo(cfg)(context.Background(), "sistema", fs, historico())
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer chave-de-teste" {
		t.Errorf("chave não foi enviada como Bearer: %q", auth)
	}
	msgs := recebido["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("esperava sistema + 3 mensagens, veio %d: %v", len(msgs), msgs)
	}
	assist := msgs[2].(map[string]any)
	tc := assist["tool_calls"].([]any)[0].(map[string]any)
	if assist["content"] != "Vou ver." || tc["id"] != "t1" || tc["function"].(map[string]any)["arguments"] != `{"filtro":"loja"}` {
		t.Errorf("mensagem do assistente convertida errado: %v", assist)
	}
	fer := msgs[3].(map[string]any)
	if fer["role"] != "tool" || fer["tool_call_id"] != "t1" || !strings.Contains(fer["content"].(string), "/p/loja") {
		t.Errorf("resultado de ferramenta convertido errado: %v", fer)
	}
	if recebido["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["name"] != "disparar" {
		t.Error("ferramentas não foram convertidas")
	}
	var blocos []bloco
	if err := json.Unmarshal(r.Content, &blocos); err != nil || len(blocos) != 2 {
		t.Fatalf("resposta convertida errado: %s", r.Content)
	}
	if blocos[0].Text != "Certo." || blocos[1].Name != "disparar" || blocos[1].ID != "c9" || blocos[1].Input["ia"] != "codex" {
		t.Errorf("blocos errados: %+v", blocos)
	}
}

func TestAnthropicMandaChaveEFormato(t *testing.T) {
	var cab http.Header
	var recebido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cab = r.Header
		if r.URL.Path != "/v1/messages" {
			t.Errorf("caminho errado: %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &recebido)
		io.WriteString(w, `{"content":[{"type":"text","text":"Oi."}],"stop_reason":"end_turn"}`)
	}))
	defer srv.Close()
	t.Setenv("MINHA_CHAVE", "abc")
	chave = ""
	cfg := nucleo.ConfigPadrao()
	cfg.Cerebro = nucleo.Cerebro{Tipo: "anthropic", URL: srv.URL, Modelo: "m", ChaveEnv: "MINHA_CHAVE"}
	r, err := EscolherModelo(cfg)(context.Background(), "sis", nil, historico())
	if err != nil || !strings.Contains(string(r.Content), "Oi.") {
		t.Fatalf("r=%v err=%v", r, err)
	}
	if cab.Get("x-api-key") != "abc" || cab.Get("anthropic-version") == "" {
		t.Errorf("cabeçalhos errados: %v", cab)
	}
	if recebido["system"] != "sis" || len(recebido["messages"].([]any)) != 3 {
		t.Errorf("corpo errado: %v", recebido)
	}
}

func TestCerebroSemConfigOuInseguro(t *testing.T) {
	chave = ""
	t.Setenv("MN_CEREBRO_CHAVE", "x")
	cfg := nucleo.ConfigPadrao()
	if _, err := EscolherModelo(cfg)(context.Background(), "", nil, nil); err == nil || !strings.Contains(err.Error(), "mn chave") {
		t.Errorf("sem config deveria orientar a cadastrar: %v", err)
	}
	for _, u := range []string{"http://api.exemplo.com/v1", "ftp://x", "", "http://10.0.0.5:8080/v1"} {
		cfg.Cerebro = nucleo.Cerebro{Tipo: "openai", URL: u, Modelo: "m"}
		if _, err := EscolherModelo(cfg)(context.Background(), "", nil, nil); err == nil {
			t.Errorf("endereço %q deveria ser recusado (a chave iria em texto aberto)", u)
		}
	}
	cfg.Cerebro = nucleo.Cerebro{Tipo: "openai", URL: "https://api.exemplo.com/v1"}
	if _, err := EscolherModelo(cfg)(context.Background(), "", nil, nil); err == nil || !strings.Contains(err.Error(), "modelo") {
		t.Errorf("sem modelo deveria avisar: %v", err)
	}
}

func TestErroDoProvedorNaoVazaChave(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":"falha interna, token recebido: `+r.Header.Get("Authorization")+`"}`)
	}))
	defer srv.Close()
	t.Setenv("MN_CEREBRO_CHAVE", "sk-supersecreta1234567890abcd")
	chave = ""
	cfg := nucleo.ConfigPadrao()
	cfg.Cerebro = nucleo.Cerebro{Tipo: "openai", URL: srv.URL, Modelo: "m"}
	_, err := EscolherModelo(cfg)(context.Background(), "", nil, historico())
	if err == nil || strings.Contains(err.Error(), "supersecreta") {
		t.Errorf("erro deveria vir sem a chave: %v", err)
	}
}
