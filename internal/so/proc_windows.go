//go:build windows

package so

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

const (
	processoDestacado = 0x00000008 // DETACHED_PROCESS
	novoGrupo         = 0x00000200 // CREATE_NEW_PROCESS_GROUP
)

func tasklist(pid int) string {
	b, _ := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").Output()
	return string(b)
}

// Vivo diz se o processo existe.
func Vivo(pid int) bool {
	return pid > 0 && strings.Contains(tasklist(pid), `"`+strconv.Itoa(pid)+`"`)
}

// Destacar faz o processo sobreviver a quem o iniciou.
func Destacar(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: processoDestacado | novoGrupo, HideWindow: true}
}

// GrupoProprio: no Windows os filhos são alcançados pela árvore (taskkill /T).
func GrupoProprio(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: novoGrupo, HideWindow: true}
}

// Encerrar: o Windows não tem pedido educado para processo sem janela;
// devolve false para quem chama ir direto ao encerramento forçado.
func Encerrar(pid int) bool { return false }

func matar(pid int, arvore bool) {
	if pid <= 4 {
		return
	}
	args := []string{"/F", "/PID", strconv.Itoa(pid)}
	if arvore {
		args = append([]string{"/T"}, args...)
	}
	_ = exec.Command("taskkill", args...).Run()
}

func Matar(pid int)         { matar(pid, false) }
func EncerrarGrupo(pid int) { matar(pid, true) }
func MatarGrupo(pid int)    { matar(pid, true) }

// NomeDoProcesso devolve o nome da imagem do processo (vazio se não existir).
func NomeDoProcesso(pid int) string {
	linha := tasklist(pid)
	if !strings.Contains(linha, `"`+strconv.Itoa(pid)+`"`) {
		return ""
	}
	return strings.Trim(strings.SplitN(linha, ",", 2)[0], `" `)
}
