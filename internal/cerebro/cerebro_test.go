package cerebro

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
)

// roteiro devolve respostas prontas, uma por chamada, e guarda o que recebeu.
type roteiro struct {
	respostas []string
	recebido  [][]Mensagem
}

func (r *roteiro) modelo(_ context.Context, _ string, _ []map[string]any, msgs []Mensagem) (*Resposta, error) {
	r.recebido = append(r.recebido, append([]Mensagem(nil), msgs...))
	if len(r.respostas) == 0 {
		return nil, errors.New("roteiro acabou")
	}
	prox := r.respostas[0]
	r.respostas = r.respostas[1:]
	return &Resposta{Content: json.RawMessage(prox)}, nil
}

func preparar(t *testing.T, respostas ...string) (*Conversa, *roteiro, string) {
	t.Helper()
	t.Setenv("MN_HOME", t.TempDir())
	raiz := t.TempDir()
	if err := os.MkdirAll(filepath.Join(raiz, "loja-web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(raiz, "outro"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := nucleo.ConfigPadrao()
	cfg.PastasPermitidas = []string{raiz}
	r := &roteiro{respostas: respostas}
	c := &Conversa{Cfg: cfg, Modelo: r.modelo, Dormir: func(time.Duration) {}}
	return c, r, raiz
}

func usar(nome, entrada string) string {
	return `[{"type":"thinking","thinking":"x","signature":"s"},{"type":"tool_use","id":"t1","name":"` + nome + `","input":` + entrada + `}]`
}

func falar(texto string) string { return `[{"type":"text","text":"` + texto + `"}]` }

// ultimoResultado devolve o texto do resultado de ferramenta mais recente que o modelo recebeu.
func ultimoResultado(t *testing.T, r *roteiro) (string, bool) {
	t.Helper()
	msgs := r.recebido[len(r.recebido)-1]
	var blocos []struct {
		Content string `json:"content"`
		IsError bool   `json:"is_error"`
	}
	if err := json.Unmarshal(msgs[len(msgs)-1].Content, &blocos); err != nil || len(blocos) == 0 {
		t.Fatalf("última mensagem não é resultado de ferramenta: %s", msgs[len(msgs)-1].Content)
	}
	return blocos[0].Content, blocos[0].IsError
}

func TestConversaSimples(t *testing.T) {
	c, r, _ := preparar(t, falar("Oi, Ana."))
	resp, err := c.Perguntar(context.Background(), "oi")
	if err != nil || resp != "Oi, Ana." {
		t.Fatalf("resp=%q err=%v", resp, err)
	}
	if len(r.recebido) != 1 {
		t.Errorf("deveria chamar o modelo uma vez, chamou %d", len(r.recebido))
	}
}

func TestListarPastasEVoltaAoModelo(t *testing.T) {
	c, r, raiz := preparar(t, usar("listar_pastas", `{"filtro":"loj"}`), falar("Achei."))
	if _, err := c.Perguntar(context.Background(), "onde fica o loja?"); err != nil {
		t.Fatal(err)
	}
	saida, falhou := ultimoResultado(t, r)
	if falhou || !strings.Contains(saida, filepath.Join(raiz, "loja-web")) || strings.Contains(saida, "outro") {
		t.Errorf("resultado inesperado: %s", saida)
	}
	// o raciocínio do modelo tem que voltar intacto na mensagem do assistente
	msgs := r.recebido[1]
	if !strings.Contains(string(msgs[1].Content), `"signature":"s"`) {
		t.Error("bloco de raciocínio não foi devolvido como veio")
	}
}

func TestEdicaoSoComLiberacao(t *testing.T) {
	entrada := `{"ia":"codex","pasta":"/tmp","pedido":"apaga tudo","modo":"edicao"}`
	c, r, _ := preparar(t, usar("disparar", entrada), falar("Não posso."))
	if _, err := c.Perguntar(context.Background(), "altera o projeto"); err != nil {
		t.Fatal(err)
	}
	saida, falhou := ultimoResultado(t, r)
	if !falhou || !strings.Contains(saida, "edição não está liberado") {
		t.Errorf("edição deveria ser recusada sem --editar: %s", saida)
	}
	if ts := nucleo.Listar(5); len(ts) != 0 {
		t.Errorf("nenhuma tarefa deveria ter sido criada, há %d", len(ts))
	}
}

func TestDispararRespeitaAsTravasDasMaos(t *testing.T) {
	casos := []string{
		`{"ia":"bash","pasta":"/tmp","pedido":"x"}`,
		`{"ia":"codex","pasta":"/etc","pedido":"x"}`,
		`{"ia":"codex","pasta":"` + os.Getenv("HOME") + `/.ssh","pedido":"x"}`,
	}
	for _, entrada := range casos {
		c, r, _ := preparar(t, usar("disparar", entrada), falar("ok"))
		c.Editar = true
		if _, err := c.Perguntar(context.Background(), "faz"); err != nil {
			t.Fatal(err)
		}
		if saida, falhou := ultimoResultado(t, r); !falhou {
			t.Errorf("deveria recusar %s, veio: %s", entrada, saida)
		}
	}
}

func TestFerramentaDesconhecidaEIdMalicioso(t *testing.T) {
	for _, uso := range []string{
		usar("rodar_shell", `{"cmd":"rm -rf /"}`),
		usar("resultado", `{"id":"../../../etc/passwd"}`),
		usar("esperar", `{"id":"../../x","segundos":"999999"}`),
	} {
		c, r, _ := preparar(t, uso, falar("ok"))
		if _, err := c.Perguntar(context.Background(), "faz"); err != nil {
			t.Fatal(err)
		}
		if saida, falhou := ultimoResultado(t, r); !falhou {
			t.Errorf("deveria recusar %s, veio: %s", uso, saida)
		}
	}
}

func TestParaDepoisDoLimiteDeVoltas(t *testing.T) {
	var laco []string
	for i := 0; i < maxVoltas+3; i++ {
		laco = append(laco, usar("listar_pastas", `{}`))
	}
	c, r, _ := preparar(t, laco...)
	if _, err := c.Perguntar(context.Background(), "gira"); err == nil {
		t.Fatal("deveria parar com erro")
	}
	if len(r.recebido) != maxVoltas {
		t.Errorf("chamou o modelo %d vezes, limite é %d", len(r.recebido), maxVoltas)
	}
}

func TestLimiteDiario(t *testing.T) {
	c, r, _ := preparar(t, falar("um"), falar("dois"), falar("três"))
	c.Cfg.MaxCerebroPorDia = 2
	for i := 0; i < 2; i++ {
		if _, err := c.Perguntar(context.Background(), "oi"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Perguntar(context.Background(), "oi"); err == nil || !strings.Contains(err.Error(), "limite") {
		t.Errorf("terceira chamada deveria bater no limite, err=%v", err)
	}
	if len(r.recebido) != 2 {
		t.Errorf("modelo chamado %d vezes", len(r.recebido))
	}
}

func TestHistoricoPersisteEFalhaNaoSuja(t *testing.T) {
	c, _, _ := preparar(t, falar("guardei"))
	if _, err := c.Perguntar(context.Background(), "meu projeto preferido é o loja"); err != nil {
		t.Fatal(err)
	}
	// outra conversa (outro processo) lê o que ficou
	r2 := &roteiro{respostas: []string{falar("loja")}}
	c2 := &Conversa{Cfg: c.Cfg, Modelo: r2.modelo}
	if _, err := c2.Perguntar(context.Background(), "qual é?"); err != nil {
		t.Fatal(err)
	}
	if n := len(r2.recebido[0]); n != 3 {
		t.Errorf("deveria receber 3 mensagens (2 antigas + 1 nova), recebeu %d", n)
	}
	// falha do modelo não deixa pedido pela metade
	r3 := &roteiro{}
	c3 := &Conversa{Cfg: c.Cfg, Modelo: r3.modelo}
	if _, err := c3.Perguntar(context.Background(), "vai falhar"); err == nil {
		t.Fatal("esperava erro")
	}
	if len(c3.msgs) != 4 {
		t.Errorf("histórico deveria ficar com 4 mensagens, ficou %d", len(c3.msgs))
	}
	if err := Esquecer(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(arquivoHistorico()); !errors.Is(err, os.ErrNotExist) {
		t.Error("Esquecer não apagou o histórico")
	}
}

func TestApararNaoSeparaFerramentaDoResultado(t *testing.T) {
	fala := func(s string) Mensagem { b, _ := json.Marshal(s); return Mensagem{Role: "user", Content: b} }
	var msgs []Mensagem
	for i := 0; i < 10; i++ {
		msgs = append(msgs, fala("pedido"),
			Mensagem{Role: "assistant", Content: json.RawMessage(usar("status", `{}`))},
			Mensagem{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"t1","content":"x"}]`)},
			Mensagem{Role: "assistant", Content: json.RawMessage(falar("pronto"))})
	}
	for _, max := range []int{1, 2, 3, 5, 7, 39} {
		out := aparar(msgs, max)
		if len(out) == 0 || !ehFalaDoUsuario(out[0]) {
			t.Errorf("max=%d: histórico aparado tem que começar numa fala do usuário", max)
		}
		if len(out) > max && len(out) != 4 {
			t.Errorf("max=%d: sobrou %d mensagens", max, len(out))
		}
	}
}

func TestPedidoVazioOuEnorme(t *testing.T) {
	c, r, _ := preparar(t, falar("x"))
	if _, err := c.Perguntar(context.Background(), "   "); err == nil {
		t.Error("vazio deveria ser recusado")
	}
	if _, err := c.Perguntar(context.Background(), strings.Repeat("a", maxPedidoUsuario+1)); err == nil {
		t.Error("enorme deveria ser recusado")
	}
	if len(r.recebido) != 0 {
		t.Error("modelo não deveria ser chamado")
	}
}

func TestEdicaoPorVozPrecisaDoSim(t *testing.T) {
	entrada := func(raiz string) string {
		return `{"ia":"bash","pasta":"` + raiz + `/loja-web","pedido":"corrige","modo":"edicao"}`
	}
	// 1º pedido: modelo tenta editar direto → recusado, pedindo confirmação
	c, r, raiz := preparar(t)
	c.Editar, c.ConfirmarEdicao = true, true
	r.respostas = []string{usar("disparar", entrada(raiz)), falar("Vou mandar corrigir o loja. Confirma?")}
	if _, err := c.Perguntar(context.Background(), "corrige o bug do loja"); err != nil {
		t.Fatal(err)
	}
	if saida, falhou := ultimoResultado(t, r); !falhou || !strings.Contains(saida, "sim do usuário") {
		t.Fatalf("primeira tentativa deveria pedir confirmação: %s", saida)
	}
	// 2º pedido: "sim" → a mesma edição passa da trava de confirmação
	// (e aqui para nas travas das mãos, porque "bash" não é uma IA do catálogo)
	r.respostas = []string{usar("disparar", entrada(raiz)), falar("ok")}
	if _, err := c.Perguntar(context.Background(), "Sim, pode."); err != nil {
		t.Fatal(err)
	}
	if saida, _ := ultimoResultado(t, r); strings.Contains(saida, "sim do usuário") {
		t.Errorf("depois do sim não deveria pedir confirmação de novo: %s", saida)
	}
	// "sim" não vale para outro alvo, nem fica valendo para depois
	r.respostas = []string{usar("disparar", entrada(raiz)), falar("x")}
	if _, err := c.Perguntar(context.Background(), "agora corrige outra coisa"); err != nil {
		t.Fatal(err)
	}
	if saida, _ := ultimoResultado(t, r); !strings.Contains(saida, "sim do usuário") {
		t.Errorf("novo pedido deveria exigir nova confirmação: %s", saida)
	}
	r.respostas = []string{usar("disparar", strings.Replace(entrada(raiz), "loja-web", "outro", 1)), falar("x")}
	if _, err := c.Perguntar(context.Background(), "sim"); err != nil {
		t.Fatal(err)
	}
	if saida, _ := ultimoResultado(t, r); !strings.Contains(saida, "sim do usuário") {
		t.Errorf("sim dado para um projeto não vale para outro: %s", saida)
	}
}

// na escuta contínua, um "sim" solto (pode ser a TV) não libera edição
func TestSimSemNomeNaoConfirmaEdicao(t *testing.T) {
	c, r, raiz := preparar(t)
	c.Editar, c.ConfirmarEdicao, c.SimComNome = true, true, true
	entrada := `{"ia":"bash","pasta":"` + raiz + `/loja-web","pedido":"corrige","modo":"edicao"}`
	r.respostas = []string{usar("disparar", entrada), falar("Confirma?")}
	c.ChamouPeloNome = true
	if _, err := c.Perguntar(context.Background(), "corrige o bug do loja"); err != nil {
		t.Fatal(err)
	}
	if saida, _ := ultimoResultado(t, r); !strings.Contains(saida, "meia-noite, sim") {
		t.Fatalf("deveria ensinar a confirmar com o nome: %s", saida)
	}
	c.ChamouPeloNome = false
	r.respostas = []string{usar("disparar", entrada), falar("Diga meia-noite, sim.")}
	if _, err := c.Perguntar(context.Background(), "Sim, confirmei."); err != nil {
		t.Fatal(err)
	}
	if saida, falhou := ultimoResultado(t, r); !falhou || !strings.Contains(saida, "COM O NOME") {
		t.Fatalf("sim sem nome não pode liberar edição: %s", saida)
	}
	if !c.EsperandoSim() {
		t.Fatal("a edição deveria continuar esperando o sim com nome")
	}
	c.ChamouPeloNome = true
	r.respostas = []string{usar("disparar", entrada), falar("ok")}
	if _, err := c.Perguntar(context.Background(), "sim"); err != nil {
		t.Fatal(err)
	}
	if saida, _ := ultimoResultado(t, r); strings.Contains(saida, "sim do usuário") {
		t.Errorf("sim com nome deveria passar da confirmação: %s", saida)
	}
}

func TestEhSim(t *testing.T) {
	for _, s := range []string{"sim", "Sim, pode.", "pode mandar", "confirmo", "manda ver", "isso mesmo"} {
		if !ehSim(s) {
			t.Errorf("%q deveria ser sim", s)
		}
	}
	for _, s := range []string{"não", "sim, não, espera", "pode não", "manda o codex resumir o projeto loja inteiro agora por favor", "", "talvez"} {
		if ehSim(s) {
			t.Errorf("%q não deveria ser sim", s)
		}
	}
}
