package oficina

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotas(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	if _, err := Anotar("  navegador padrão:   Edge "); err != nil {
		t.Fatal(err)
	}
	_, _ = Anotar("navegador padrão: edge") // repetida
	if n := Notas(); len(n) != 1 || n[0].Texto != "navegador padrão: Edge" {
		t.Fatalf("notas: %+v", n)
	}
	if g, _ := Anotar("a chave é sk-abcdefghijklmnopqrstuvwxyz123456"); strings.Contains(g, "abcdefghijkl") {
		t.Errorf("segredo foi parar na memória curta: %s", g)
	}
	for i := 0; i < MaxNotas+5; i++ {
		_, _ = Anotar("fato número " + strings.Repeat("x", i+1))
	}
	if len(Notas()) != MaxNotas {
		t.Errorf("deveria manter só %d notas, tem %d", MaxNotas, len(Notas()))
	}
	if n, _ := Esquecer("fato número"); n != MaxNotas {
		t.Errorf("esquecer apagou %d", n)
	}
	if _, err := Esquecer("a"); err == nil {
		t.Error("trecho curto demais apagaria quase tudo")
	}
}

func TestNomesDeScript(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	for _, ruim := range []string{"", "../fora.sh", "a/b.sh", "x.rb", ".oculto.sh", "nome com espaço.sh", "X.SH", "a.sh\x00", "/etc/passwd"} {
		if _, err := Escrever(ruim, "echo oi"); err == nil {
			t.Errorf("nome %q deveria ser recusado", ruim)
		}
		if _, err := Rodar(context.Background(), ruim); err == nil {
			t.Errorf("rodar %q deveria ser recusado", ruim)
		}
	}
	if _, err := Escrever("ok.sh", ""); err == nil {
		t.Error("script vazio deveria ser recusado")
	}
	// link para fora da oficina não roda nem é lido
	p, _ := Pasta()
	_ = os.Symlink("/bin/ls", filepath.Join(p, "link.sh"))
	if _, err := Rodar(context.Background(), "link.sh"); err == nil {
		t.Error("link simbólico não pode rodar")
	}
	if _, err := Escrever("link.sh", "echo"); err == nil {
		t.Error("não pode escrever por cima de link")
	}
}

func TestScriptRodaNaCaixaFechada(t *testing.T) {
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		t.Skip("sem sandbox-exec")
	}
	casa := t.TempDir()
	t.Setenv("MN_HOME", casa)
	fora := filepath.Join(casa, "fora.txt")
	lar, _ := os.UserHomeDir()
	script := `echo dentro > nota.txt && echo ESCREVEU_DENTRO
echo x > "` + fora + `" 2>/dev/null && echo ESCREVEU_FORA
echo x > "` + lar + `/meia-noite-teste-fora.txt" 2>/dev/null && echo ESCREVEU_NA_HOME
ls "` + lar + `/.ssh" >/dev/null 2>&1 && echo LEU_SSH
cat "` + casa + `/segredo.txt" 2>/dev/null && echo LEU_DADOS
/usr/bin/nc -z -w 2 1.1.1.1 443 >/dev/null 2>&1 && echo REDE_ABERTA
echo fim`
	_ = os.WriteFile(filepath.Join(casa, "segredo.txt"), []byte("x"), 0o600)
	if _, err := Escrever("teste.sh", script); err != nil {
		t.Fatal(err)
	}
	saida, err := Rodar(context.Background(), "teste.sh")
	if err != nil {
		t.Fatalf("erro: %v\n%s", err, saida)
	}
	if !strings.Contains(saida, "ESCREVEU_DENTRO") || !strings.HasSuffix(saida, "fim") {
		t.Errorf("deveria escrever dentro da oficina: %s", saida)
	}
	for _, ruim := range []string{"ESCREVEU_FORA", "ESCREVEU_NA_HOME", "LEU_SSH", "LEU_DADOS", "REDE_ABERTA"} {
		if strings.Contains(saida, ruim) {
			t.Errorf("a caixa vazou: %s", ruim)
		}
	}
	if _, err := os.Stat(fora); err == nil {
		t.Error("arquivo criado fora da oficina")
	}
	if got := Listar(); len(got) != 1 || got[0] != "teste.sh" {
		t.Errorf("listar: %v", got)
	}
	if _, err := Escrever("erro.sh", "exit 3"); err != nil {
		t.Fatal(err)
	}
	if _, err := Rodar(context.Background(), "erro.sh"); err == nil {
		t.Error("script com erro deveria devolver erro")
	}
}
