// Package oficina é o canto do próprio meia-noite: uma memória curta (notas
// que ele relê a cada conversa, sem custo de consulta) e uma pasta de scripts
// que ele mesmo escreve. Scripts só rodam numa caixa fechada do macOS: escrita
// apenas dentro da oficina, sem rede e sem acesso às pastas de chaves.
package oficina

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
)

const (
	MaxNotas       = 40
	maxNota        = 240
	maxScript      = 64 << 10
	maxScripts     = 100
	maxSaida       = 4000
	TempoScriptSeg = 60
)

var mu sync.Mutex

// ---------- memória curta

type Nota struct {
	Texto  string `json:"texto"`
	Quando string `json:"quando"`
}

func arqNotas() string { return filepath.Join(nucleo.Casa(), "memoria-curta.json") }

// Notas devolve a memória curta (mais antiga primeiro).
func Notas() []Nota {
	b, err := os.ReadFile(arqNotas())
	if err != nil {
		return nil
	}
	var n []Nota
	_ = json.Unmarshal(b, &n)
	return n
}

func salvarNotas(n []Nota) error {
	if err := os.MkdirAll(nucleo.Casa(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(n, "", "  ")
	tmp := arqNotas() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, arqNotas())
}

// Anotar guarda um fato curto. Segredos viram [oculto]; passou do limite, a mais antiga sai.
func Anotar(texto string) (string, error) {
	texto = strings.Join(strings.Fields(texto), " ")
	if texto == "" {
		return "", errors.New("nota vazia")
	}
	texto = nucleo.Resumir(texto, maxNota)
	mu.Lock()
	defer mu.Unlock()
	notas := Notas()
	for _, n := range notas {
		if strings.EqualFold(n.Texto, texto) {
			return texto, nil
		}
	}
	notas = append(notas, Nota{Texto: texto, Quando: time.Now().Format("2006-01-02")})
	if len(notas) > MaxNotas {
		notas = notas[len(notas)-MaxNotas:]
	}
	return texto, salvarNotas(notas)
}

// Esquecer apaga as notas que contêm o trecho; devolve quantas saíram.
func Esquecer(trecho string) (int, error) {
	trecho = strings.ToLower(strings.TrimSpace(trecho))
	if len(trecho) < 3 {
		return 0, errors.New("diga um trecho da nota com pelo menos 3 letras")
	}
	mu.Lock()
	defer mu.Unlock()
	var fica []Nota
	fora := 0
	for _, n := range Notas() {
		if strings.Contains(strings.ToLower(n.Texto), trecho) {
			fora++
			continue
		}
		fica = append(fica, n)
	}
	if fora == 0 {
		return 0, nil
	}
	return fora, salvarNotas(fica)
}

// ---------- scripts

// Pasta devolve (criando) a pasta dos scripts do meia-noite.
func Pasta() (string, error) {
	p := filepath.Join(nucleo.Casa(), "oficina")
	if err := os.MkdirAll(p, 0o700); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

var reNome = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,48}\.(sh|py)$`)

func caminho(nome string) (string, error) {
	if !reNome.MatchString(nome) {
		return "", errors.New("nome de script inválido: use letras minúsculas, números, - e _, terminando em .sh ou .py (ex.: listar-portas.sh)")
	}
	p, err := Pasta()
	if err != nil {
		return "", err
	}
	return filepath.Join(p, nome), nil
}

// Escrever grava (ou substitui) um script na oficina.
func Escrever(nome, conteudo string) (string, error) {
	arq, err := caminho(nome)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(conteudo) == "" || len(conteudo) > maxScript || strings.ContainsRune(conteudo, 0) {
		return "", errors.New("conteúdo do script vazio, grande demais (máx. 64 KB) ou inválido")
	}
	if info, err := os.Lstat(arq); err == nil && !info.Mode().IsRegular() {
		return "", errors.New("já existe algo com esse nome que não é um arquivo comum")
	} else if err != nil && len(Listar()) >= maxScripts {
		return "", errors.New("a oficina está cheia (100 scripts): apague algum antes")
	}
	return arq, os.WriteFile(arq, []byte(conteudo), 0o600)
}

// Listar devolve os nomes dos scripts.
func Listar() []string {
	p, err := Pasta()
	if err != nil {
		return nil
	}
	itens, _ := os.ReadDir(p)
	var nomes []string
	for _, i := range itens {
		if i.Type().IsRegular() && reNome.MatchString(i.Name()) {
			nomes = append(nomes, i.Name())
		}
	}
	sort.Strings(nomes)
	return nomes
}

// Ler devolve o conteúdo de um script.
func Ler(nome string) (string, error) {
	arq, err := caminho(nome)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(arq)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("script não encontrado: " + nome)
	}
	b, err := os.ReadFile(arq)
	return string(b), err
}

func aspas(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// perfil monta as regras da caixa: tudo o que não for dito aqui segue o padrão
// do sistema, MENOS rede (negada), escrita (só na oficina) e leitura de chaves.
func perfil(pasta, lar string) string {
	var b strings.Builder
	b.WriteString("(version 1)(allow default)(deny network*)(deny file-write*)")
	b.WriteString("(allow file-write* (subpath " + aspas(pasta) + ") (literal \"/dev/null\"))")
	b.WriteString("(deny file-read*")
	for _, p := range []string{".ssh", ".aws", ".gnupg", ".kube", ".docker", ".netrc", ".mmx", ".config", ".claude", ".codex", ".cursor", ".1password", "Library/Keychains", "Library/Application Support", "Library/Cookies", "Library/Group Containers", "Library/Containers"} {
		b.WriteString(" (subpath " + aspas(filepath.Join(lar, p)) + ")")
	}
	b.WriteString(")")
	// os dados do próprio meia-noite (conversa, config) também ficam fora, menos a oficina
	if casa, err := filepath.EvalSymlinks(nucleo.Casa()); err == nil {
		b.WriteString("(deny file-read* (subpath " + aspas(casa) + "))(allow file-read* (subpath " + aspas(pasta) + "))")
	}
	return b.String()
}

// Rodar executa um script da oficina dentro da caixa fechada, com tempo limite.
// Nunca passa por shell: o interpretador é fixo pela extensão.
func Rodar(ctx context.Context, nome string) (string, error) {
	arq, err := caminho(nome)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(arq)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("script não encontrado: " + nome)
	}
	pasta := filepath.Dir(arq)
	lar, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		return "", errors.New("este sistema não tem a caixa fechada do macOS (sandbox-exec); não rodo scripts sem ela")
	}
	interp := []string{"/bin/bash", "--noprofile", "--norc"}
	if strings.HasSuffix(nome, ".py") {
		py, err := exec.LookPath("python3")
		if err != nil {
			return "", errors.New("python3 não encontrado")
		}
		interp = []string{py, "-I"}
	}
	ctx, cancelar := context.WithTimeout(ctx, TempoScriptSeg*time.Second)
	defer cancelar()
	args := append([]string{"-p", perfil(pasta, lar)}, append(interp, arq)...)
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", args...)
	cmd.Dir = pasta
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin", "HOME=" + pasta, "LANG=pt_BR.UTF-8", "TMPDIR=" + pasta}
	var saida bytes.Buffer
	cmd.Stdout, cmd.Stderr = &saida, &saida
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	texto := strings.TrimSpace(saida.String())
	if len(texto) > maxSaida {
		texto = texto[:maxSaida] + "\n[saída cortada]"
	}
	if ctx.Err() != nil {
		return texto, fmt.Errorf("o script passou de %d s e foi interrompido", TempoScriptSeg)
	}
	if err != nil {
		return texto, fmt.Errorf("o script terminou com erro (%v)", err)
	}
	return texto, nil
}
