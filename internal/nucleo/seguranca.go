package nucleo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxPedido = 8000

// Componentes de caminho que nunca são pasta de trabalho, mesmo dentro de uma raiz permitida.
var pastasNegadas = map[string]bool{
	".ssh": true, ".gnupg": true, ".aws": true, ".kube": true,
	"Keychains": true, ".1password": true,
}

var reID = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{4}$`)

// IDValido impede que um id vindo de fora vire caminho arbitrário.
func IDValido(id string) bool { return reID.MatchString(id) }

// ValidarPasta devolve o caminho real (links resolvidos) se ele estiver dentro
// de uma raiz permitida. A comparação é por componente: "/a/Projetos_x" não
// passa por estar "dentro" de "/a/Projetos".
func ValidarPasta(cfg Config, pasta string) (string, error) {
	if strings.TrimSpace(pasta) == "" {
		return "", errors.New("pasta vazia")
	}
	if strings.ContainsRune(pasta, 0) {
		return "", errors.New("pasta inválida")
	}
	abs, err := filepath.Abs(pasta)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("pasta não encontrada: %s", pasta)
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("não é uma pasta: %s", pasta)
	}
	for _, parte := range strings.Split(real, string(filepath.Separator)) {
		if pastasNegadas[parte] {
			return "", fmt.Errorf("pasta negada por segurança: contém %q", parte)
		}
	}
	for _, raiz := range cfg.PastasPermitidas {
		raizReal, err := filepath.EvalSymlinks(raiz)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(raizReal, real)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("pasta fora da lista permitida: %s (edite pastas_permitidas em %s)", real, caminhoConfig())
}

// NormalizarPedido valida o texto que vira argumento da CLI. O pedido nunca
// passa por shell; ainda assim, um texto começando com "-" poderia ser lido
// como opção pela CLI, então ganha um prefixo neutro.
func NormalizarPedido(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("pedido vazio")
	}
	if strings.ContainsRune(p, 0) {
		return "", errors.New("pedido inválido")
	}
	if len(p) > maxPedido {
		return "", fmt.Errorf("pedido longo demais (%d caracteres; máximo %d)", len(p), maxPedido)
	}
	if strings.HasPrefix(p, "-") {
		p = "Pedido: " + p
	}
	return p, nil
}
