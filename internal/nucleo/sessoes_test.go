package nucleo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResumirEscondeSegredos(t *testing.T) {
	for _, segredo := range []string{
		"sk-abcdef1234567890abcd", "ghp_abcdefghijklmnop1234", "senha: hunter2-muito-secreta", // gitleaks:allow (segredo de mentira: testa o filtro)
		"API_KEY=valor123", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghij", // gitleaks:allow (segredo de mentira: testa o filtro)
		strings.Repeat("Ab3", 20),
	} {
		got := Resumir("antes "+segredo+" depois", 500)
		if strings.Contains(got, segredo) || !strings.Contains(got, "[oculto]") {
			t.Errorf("segredo vazou: %q -> %q", segredo, got)
		}
	}
	if got := Resumir("texto   comum\ncom quebra", 500); got != "texto comum com quebra" {
		t.Errorf("texto comum mudou: %q", got)
	}
	if got := Resumir(strings.Repeat("a ", 400), 50); len([]rune(got)) != 51 {
		t.Errorf("não cortou: %d", len([]rune(got)))
	}
}

func TestSessoesClaude(t *testing.T) {
	raiz := t.TempDir()
	t.Setenv("MN_CLAUDE_PROJETOS", raiz)
	pasta := filepath.Join(raiz, "-proj")
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		t.Fatal(err)
	}
	linhas := strings.Join([]string{
		`{"type":"user","cwd":"/x/loja","message":{"role":"user","content":"corrige o teste de login com token: abc123segredo"}}`, // gitleaks:allow (segredo de mentira: testa o filtro)
		`{"type":"assistant","cwd":"/x/loja","message":{"role":"assistant","content":[{"type":"thinking","thinking":"..."},{"type":"tool_use","name":"Bash","input":{}}]}}`,
		`{"type":"user","cwd":"/x/loja","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`,
		`{"type":"assistant","cwd":"/x/loja","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"fala de subagente"}]}}`,
		`{"type":"assistant","cwd":"/x/loja","message":{"role":"assistant","content":[{"type":"text","text":"Teste corrigido e passando."}]}}`,
		`linha quebrada {`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(pasta, "a.jsonl"), []byte(linhas), 0o600); err != nil {
		t.Fatal(err)
	}
	velha := filepath.Join(pasta, "velha.jsonl")
	_ = os.WriteFile(velha, []byte(linhas), 0o600)
	ontem := time.Now().Add(-26 * time.Hour)
	_ = os.Chtimes(velha, ontem, ontem)

	ss := SessoesClaude(time.Hour, 5)
	if len(ss) != 1 {
		t.Fatalf("esperava 1 sessão recente, veio %d", len(ss))
	}
	s := ss[0]
	if s.Pasta != "/x/loja" || s.Ferramenta != "Bash" || s.Resposta != "Teste corrigido e passando." {
		t.Errorf("resumo errado: %+v", s)
	}
	if !strings.Contains(s.Pedido, "corrige o teste") || strings.Contains(s.Pedido, "abc123segredo") {
		t.Errorf("pedido deveria vir sem o segredo: %q", s.Pedido)
	}
}
