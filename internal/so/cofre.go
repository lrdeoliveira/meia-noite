package so

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// O cofre guarda a chave do cérebro fora de qualquer arquivo de texto:
// Chaves do macOS, libsecret no Linux (comando secret-tool) e, no Windows,
// um arquivo cifrado pelo próprio Windows para o seu usuário (DPAPI).
// Em todos, quem lê a chave digitada é a ferramenta do sistema, com o que
// você digita escondido.

const (
	servico = "meia-noite"
	rotulo  = "meia-noite: chave do cérebro"
)

func arqWindows(casa string) string { return filepath.Join(casa, "chave-"+ContaPadrao+".dpapi") }

// ContaPadrao é o nome sob o qual a chave do cérebro é guardada.
const ContaPadrao = "cerebro"

// CofreNome diz onde a chave fica, para as mensagens.
func CofreNome() string {
	switch runtime.GOOS {
	case "darwin":
		return "Chaves do macOS"
	case "windows":
		return "cofre do Windows (DPAPI)"
	}
	return "chaveiro do sistema (libsecret)"
}

// GuardarChave pede a chave no terminal e guarda no cofre.
func GuardarChave(casa string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("/usr/bin/security", "add-generic-password", "-U", "-s", servico, "-a", ContaPadrao, "-l", rotulo, "-w")
	case "windows":
		if err := os.MkdirAll(casa, 0o700); err != nil {
			return err
		}
		cmd = exec.Command("powershell", "-NoProfile", "-Command",
			"$s = Read-Host -AsSecureString 'Chave'; $s | ConvertFrom-SecureString | Set-Content -Path $env:MN_ARQ_CHAVE")
		cmd.Env = append(os.Environ(), "MN_ARQ_CHAVE="+arqWindows(casa))
	default:
		bin, err := exec.LookPath("secret-tool")
		if err != nil {
			return errors.New("secret-tool não encontrado (pacote libsecret-tools). Sem ele, use a variável de ambiente MN_CEREBRO_CHAVE")
		}
		cmd = exec.Command(bin, "store", "--label="+rotulo, "service", servico, "account", ContaPadrao)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// LerChave devolve a chave guardada (erro se não houver).
func LerChave(ctx context.Context, casa string) (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", servico, "-a", ContaPadrao, "-w")
	case "windows":
		if _, err := os.Stat(arqWindows(casa)); err != nil {
			return "", errors.New("sem chave guardada")
		}
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
			"$s = Get-Content -Path $env:MN_ARQ_CHAVE | ConvertTo-SecureString; [Runtime.InteropServices.Marshal]::PtrToStringBSTR([Runtime.InteropServices.Marshal]::SecureStringToBSTR($s))")
		cmd.Env = append(os.Environ(), "MN_ARQ_CHAVE="+arqWindows(casa))
	default:
		bin, err := exec.LookPath("secret-tool")
		if err != nil {
			return "", errors.New("sem chave guardada")
		}
		cmd = exec.CommandContext(ctx, bin, "lookup", "service", servico, "account", ContaPadrao)
	}
	b, err := cmd.Output()
	if k := strings.TrimSpace(string(b)); err == nil && k != "" {
		return k, nil
	}
	return "", errors.New("sem chave guardada")
}

// ApagarChave remove a chave do cofre.
func ApagarChave(casa string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("/usr/bin/security", "delete-generic-password", "-s", servico, "-a", ContaPadrao).Run()
	case "windows":
		return os.Remove(arqWindows(casa))
	}
	bin, err := exec.LookPath("secret-tool")
	if err != nil {
		return err
	}
	return exec.Command(bin, "clear", "service", servico, "account", ContaPadrao).Run()
}
