// Package so junta o que muda de um sistema operacional para outro:
// processos, microfone, som, navegador e o cofre onde a chave fica guardada.
package so

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var reDispositivo = regexp.MustCompile(`^[\p{L}\p{N} ()\[\]._:,@#&+/\\{}-]{1,200}$`)

// Microfone devolve os argumentos de entrada do ffmpeg para o microfone.
// No macOS o dispositivo é um número (0 = padrão). No Linux é o nome da fonte
// do PulseAudio/PipeWire ("default" ou "0" = padrão). No Windows é o nome do
// dispositivo como aparece em: ffmpeg -list_devices true -f dshow -i dummy
func Microfone(dispositivo string) ([]string, error) {
	d := strings.TrimSpace(dispositivo)
	switch runtime.GOOS {
	case "darwin":
		if _, err := strconv.Atoi(d); err != nil {
			return nil, errors.New("microfone inválido no config (no macOS é um número; 0 = padrão)")
		}
		return []string{"-f", "avfoundation", "-i", ":" + d}, nil
	case "windows":
		if d == "" || d == "0" || !reDispositivo.MatchString(d) || strings.HasPrefix(d, "-") {
			return nil, errors.New(`no Windows, ponha o nome do microfone em voz.microfone no config.json. Para ver os nomes: ffmpeg -list_devices true -f dshow -i dummy`)
		}
		return []string{"-f", "dshow", "-i", "audio=" + d}, nil
	default:
		if d == "" || d == "0" {
			d = "default"
		}
		if !reDispositivo.MatchString(d) || strings.HasPrefix(d, "-") {
			return nil, errors.New("microfone inválido no config")
		}
		return []string{"-f", "pulse", "-i", d}, nil
	}
}

var reNavegador = regexp.MustCompile(`^[\p{L}\p{N} ._-]{1,60}$`)

// AbrirNavegador abre o endereço no navegador padrão (ou, no macOS, no
// aplicativo indicado em "navegador" no config).
func AbrirNavegador(endereco, navegador string) error {
	switch runtime.GOOS {
	case "darwin":
		if navegador != "" && reNavegador.MatchString(navegador) {
			return exec.Command("/usr/bin/open", "-a", navegador, endereco).Run()
		}
		return exec.Command("/usr/bin/open", endereco).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", endereco).Run()
	default:
		return exec.Command("xdg-open", endereco).Start()
	}
}

// Tocar toca um arquivo de áudio até o fim ou até o contexto ser cancelado.
func Tocar(ctx context.Context, arquivo string) error {
	if runtime.GOOS == "darwin" {
		return exec.CommandContext(ctx, "/usr/bin/afplay", arquivo).Run()
	}
	bin, err := exec.LookPath("ffplay")
	if err != nil {
		return errors.New("ffplay não encontrado (vem junto com o ffmpeg)")
	}
	return exec.CommandContext(ctx, bin, "-nodisp", "-autoexit", "-loglevel", "quiet", arquivo).Run()
}

// Aviso toca um som curto (ele ouviu o nome e está esperando o pedido).
func Aviso() {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("/usr/bin/afplay", "/System/Library/Sounds/Tink.aiff").Run()
		return
	}
	if bin, err := exec.LookPath("ffplay"); err == nil {
		_ = exec.Command(bin, "-nodisp", "-autoexit", "-loglevel", "quiet", "-f", "lavfi", "-i", "sine=frequency=1200:duration=0.12").Run()
	}
}

var reVoz = regexp.MustCompile(`^[\p{L}\p{N} ()._+-]{1,60}$`)

// FalarLocal fala com a voz do próprio sistema, sem conta e sem internet:
// `say` no macOS, espeak-ng no Linux, a fala do Windows via PowerShell.
// O texto vai sempre pela entrada padrão, nunca como argumento.
func FalarLocal(ctx context.Context, texto, voz string) error {
	if !reVoz.MatchString(voz) {
		voz = ""
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if voz == "" {
			voz = "Luciana"
		}
		cmd = exec.CommandContext(ctx, "/usr/bin/say", "-v", voz)
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
			"Add-Type -AssemblyName System.Speech; $s = New-Object System.Speech.Synthesis.SpeechSynthesizer; $s.Speak([Console]::In.ReadToEnd())")
	default:
		bin, err := exec.LookPath("espeak-ng")
		if err != nil {
			return errors.New("espeak-ng não encontrado (instale-o, ou use outro motor de voz)")
		}
		if voz == "" {
			voz = "pt-br"
		}
		cmd = exec.CommandContext(ctx, bin, "-v", voz, "--stdin")
	}
	cmd.Stdin = strings.NewReader(texto)
	return cmd.Run()
}
