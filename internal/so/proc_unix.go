//go:build !windows

package so

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// Vivo diz se o processo existe.
func Vivo(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Destacar faz o processo sobreviver a quem o iniciou.
func Destacar(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// GrupoProprio põe o processo num grupo dele: dá para encerrar ele e os filhos.
func GrupoProprio(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// Encerrar pede com educação; devolve se o pedido pôde ser entregue.
func Encerrar(pid int) bool { return pid > 1 && syscall.Kill(pid, syscall.SIGTERM) == nil }

// Matar encerra à força.
func Matar(pid int) {
	if pid > 1 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// EncerrarGrupo e MatarGrupo valem para o processo e todos os filhos.
func EncerrarGrupo(pid int) {
	if pid > 1 {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
}

func MatarGrupo(pid int) {
	if pid > 1 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

// NomeDoProcesso devolve a linha de comando do processo (vazio se não existir).
func NomeDoProcesso(pid int) string {
	b, _ := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	return strings.TrimSpace(string(b))
}
