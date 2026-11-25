package memoria

import (
	"context"
	"strings"
	"testing"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
)

func TestLerRespostaDeEventos(t *testing.T) {
	sse := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"answer\\\":\\\"Microsoft Edge.\\\",\\\"guideline\\\":[{\\\"content\\\":\\\"texto enorme\\\"}],\\\"sources\\\":[{\\\"title\\\":\\\"Navegador\\\",\\\"project\\\":\\\"geral\\\"},{\\\"title\\\":\\\"Outro\\\"}]}\"}]}}\n\n"
	a, err := lerResposta(strings.NewReader(sse))
	if err != nil {
		t.Fatal(err)
	}
	if a.Resposta != "Microsoft Edge." || len(a.Fontes) != 2 || a.Fontes[0] != "Navegador (geral)" {
		t.Errorf("leitura errada: %+v", a)
	}
}

func TestLerRespostaJSONPuroEErros(t *testing.T) {
	a, err := lerResposta(strings.NewReader(`{"result":{"content":[{"text":"{\"answer\":\"\",\"sources\":[]}"}]}}`))
	if err != nil || !strings.Contains(a.Resposta, "Não encontrei") {
		t.Errorf("vazio deveria virar aviso: %+v %v", a, err)
	}
	if _, err := lerResposta(strings.NewReader(`{"error":{"message":"quota"}}`)); err == nil {
		t.Error("erro do servidor deveria virar erro")
	}
	if _, err := lerResposta(strings.NewReader("<html>502</html>")); err == nil {
		t.Error("lixo deveria virar erro")
	}
}

func TestRecusaEnderecoInseguroEPerguntaVazia(t *testing.T) {
	cfg := nucleo.ConfigPadrao()
	if _, err := Buscar(context.Background(), cfg, "  "); err == nil {
		t.Error("pergunta vazia deveria ser recusada")
	}
	for _, u := range []string{"http://api.nexusyn.ai/v1/mcp", "file:///etc/passwd", "", "api.nexusyn.ai"} {
		cfg.Nexusyn.URL = u
		if _, err := Buscar(context.Background(), cfg, "oi"); err == nil || !strings.Contains(err.Error(), "https") {
			t.Errorf("endereço %q deveria ser recusado antes de ler a chave, err=%v", u, err)
		}
	}
}
