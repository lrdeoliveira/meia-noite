// Package voz dá ouvido e fala ao meia-noite.
//
// Ouvir: grava o microfone com ffmpeg e transcreve neste Mac com whisper.cpp,
// que fica carregado num serviço local falando por socket de arquivo (sem porta
// de rede). Falar: gera o áudio pela MiniMax (comando mmx) e toca com afplay.
package voz

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
	"github.com/lrdeoliveira/meia-noite/internal/so"
)

const (
	MaxGravacaoSeg   = 45  // teto de uma fala do usuário
	esperaFalaSeg    = 8   // sem nenhuma voz nesse tempo, desiste
	pausaFimSeg      = 1.0 // silêncio que encerra a fala
	maxFalaCaracters = 500 // teto do texto falado de uma vez
)

func pastaVoz() string   { return filepath.Join(nucleo.Casa(), "voz") }
func soquete() string    { return filepath.Join(nucleo.Casa(), "ouvido.sock") }
func arquivoPID() string { return filepath.Join(nucleo.Casa(), "ouvido.pid") }
func arquivoLog() string { return filepath.Join(nucleo.Casa(), "ouvido.log") }
func temporario(ext string) string {
	return filepath.Join(pastaVoz(), fmt.Sprintf("%d-%d%s", time.Now().UnixNano(), os.Getpid(), ext))
}

// No Windows o transcritor não sabe usar socket de arquivo: fica numa porta
// de 127.0.0.1 (só este computador alcança).
const portaOuvidoWindows = "4791"

func redeOuvido() (rede, endereco string) {
	if runtime.GOOS == "windows" {
		return "tcp", "127.0.0.1:" + portaOuvidoWindows
	}
	return "unix", soquete()
}

func clienteOuvido() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			rede, end := redeOuvido()
			return (&net.Dialer{}).DialContext(ctx, rede, end)
		}},
	}
}

// OuvidoNoAr diz se o serviço de transcrição está respondendo.
func OuvidoNoAr() bool {
	rede, end := redeOuvido()
	c, err := net.DialTimeout(rede, end, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// GarantirOuvido sobe o serviço de transcrição se ele não estiver no ar.
// Ele carrega o modelo uma vez (uns 10 s) e depois transcreve em cerca de 1 s.
func GarantirOuvido(cfg nucleo.Config) error {
	if OuvidoNoAr() {
		return nil
	}
	bin, err := exec.LookPath("whisper-server")
	if err != nil {
		return errors.New("whisper-server não encontrado (é do whisper.cpp; no macOS: brew install whisper-cpp)")
	}
	if _, err := os.Stat(cfg.Voz.ModeloOuve); err != nil {
		return fmt.Errorf("modelo de transcrição não encontrado em %s", cfg.Voz.ModeloOuve)
	}
	if err := os.MkdirAll(nucleo.Casa(), 0o700); err != nil {
		return err
	}
	_ = os.Remove(soquete()) // sobra de um serviço que morreu
	log, err := os.OpenFile(arquivoLog(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	args := []string{"-m", cfg.Voz.ModeloOuve, "-l", "pt", "-nt", "--host", soquete()}
	if runtime.GOOS == "windows" {
		args = []string{"-m", cfg.Voz.ModeloOuve, "-l", "pt", "-nt", "--host", "127.0.0.1", "--port", portaOuvidoWindows}
	}
	if _, err := os.Stat(cfg.Voz.ModeloVAD); err == nil {
		args = append(args, "--vad", "-vm", cfg.Voz.ModeloVAD)
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = log, log
	so.Destacar(cmd) // segue vivo depois que este comando acaba
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = os.WriteFile(arquivoPID(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
	_ = cmd.Process.Release()
	for i := 0; i < 90; i++ {
		time.Sleep(time.Second)
		if OuvidoNoAr() {
			if runtime.GOOS == "windows" {
				return nil
			}
			return os.Chmod(soquete(), 0o600)
		}
	}
	return fmt.Errorf("o serviço de transcrição não subiu; veja %s", arquivoLog())
}

// PararOuvido encerra o serviço de transcrição (libera uns 700 MB de memória).
func PararOuvido() (bool, error) {
	b, err := os.ReadFile(arquivoPID())
	if err != nil {
		return false, nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return false, nil
	}
	// só mata se o processo ainda for mesmo o transcritor
	parado := false
	if strings.Contains(so.NomeDoProcesso(pid), "whisper-server") {
		if parado = so.Encerrar(pid); !parado {
			so.Matar(pid)
			parado = true
		}
	}
	_ = os.Remove(arquivoPID())
	_ = os.Remove(soquete())
	return parado, nil
}

// Frases que o modelo inventa quando só há silêncio ou ruído.
var invencoes = []string{"amara.org", "legendas pela comunidade", "legenda adriana zanotto", "obrigado por assistir", "inscreva-se no canal", "[música]", "[musica]", "[silêncio]", "[blank_audio]", "(música)"}

// LimparTranscricao tira espaços e descarta o que é invenção de silêncio.
func LimparTranscricao(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	baixo := strings.ToLower(s)
	for _, inv := range invencoes {
		if strings.Contains(baixo, inv) {
			return ""
		}
	}
	if len([]rune(strings.Trim(s, " .,!?…-"))) < 2 {
		return ""
	}
	return s
}

// Vocabulario monta a dica de nomes que o transcritor recebe: sem ela, nomes
// de ferramenta em inglês ditos no meio do português saem trocados
// ("Claude Code" virava "Crowd Crew").
func Vocabulario(extras []string) string {
	base := "Meia-noite, manda o Codex resumir o projeto. O que o Claude Code está fazendo? Abre o Cursor, o Antigravity e o OpenCode. Nomes"
	for _, e := range extras {
		if len(base) > 600 {
			break
		}
		if e = strings.TrimSpace(e); e != "" && !strings.Contains(strings.ToLower(base), strings.ToLower(e)) {
			base += ", " + e
		}
	}
	return base + "."
}

// Transcrever manda um arquivo de áudio (wav 16 kHz mono) ao serviço local.
func Transcrever(ctx context.Context, arquivo string, extras ...string) (string, error) {
	f, err := os.Open(arquivo)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var corpo bytes.Buffer
	w := multipart.NewWriter(&corpo)
	parte, _ := w.CreateFormFile("file", "fala.wav")
	if _, err := io.Copy(parte, f); err != nil {
		return "", err
	}
	_ = w.WriteField("response_format", "json")
	_ = w.WriteField("language", "pt")
	_ = w.WriteField("prompt", Vocabulario(extras))
	w.Close()
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://ouvido/inference", &corpo)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := clienteOuvido().Do(req)
	if err != nil {
		return "", errors.New("o serviço de transcrição não respondeu: " + err.Error())
	}
	defer resp.Body.Close()
	var r struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return "", errors.New("resposta da transcrição ilegível")
	}
	if r.Error != "" {
		return "", errors.New("transcrição falhou: " + r.Error)
	}
	return LimparTranscricao(r.Text), nil
}

var reSilencio = regexp.MustCompile(`silence_(start|end): ([0-9.]+)`)

// decidirParada lê os avisos de silêncio do ffmpeg e diz quando parar:
// "fim" quando a pessoa falou e depois se calou; "nada" quando não falou.
func decidirParada(linhas <-chan string, aviso chan<- string) {
	falou := false
	limite := time.After(esperaFalaSeg * time.Second)
	for {
		select {
		case l, ok := <-linhas:
			if !ok {
				return
			}
			m := reSilencio.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			t, _ := strconv.ParseFloat(m[2], 64)
			switch {
			case m[1] == "end":
				falou = true
			case t > 0.4: // silêncio que começou depois de algum som = fim da fala
				aviso <- "fim"
				return
			}
		case <-limite:
			if !falou {
				aviso <- "nada"
				return
			}
			limite = nil
		}
	}
}

// Gravar grava o microfone até a pessoa se calar, até `parar` fechar ou até o
// teto de tempo. Devolve o caminho do áudio (o chamador apaga) e se houve voz.
func Gravar(ctx context.Context, cfg nucleo.Config, parar <-chan struct{}) (string, bool, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", false, errors.New("ffmpeg não encontrado (instale o ffmpeg)")
	}
	mic, err := so.Microfone(cfg.Voz.Microfone)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(pastaVoz(), 0o700); err != nil {
		return "", false, err
	}
	saida := temporario(".wav")
	filtro := fmt.Sprintf("silencedetect=noise=%ddB:d=%.1f", cfg.Voz.SilencioDB, pausaFimSeg)
	args := append([]string{"-hide_banner", "-nostats", "-loglevel", "info", "-y"}, mic...)
	args = append(args, "-t", strconv.Itoa(MaxGravacaoSeg), "-af", filtro, "-ar", "16000", "-ac", "1", saida)
	cmd := exec.CommandContext(ctx, bin, args...)
	entrada, _ := cmd.StdinPipe()
	erros, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return "", false, err
	}
	linhas := make(chan string, 64)
	var ultimas []string
	fimLeitura := make(chan struct{})
	go func() {
		defer close(fimLeitura)
		defer close(linhas)
		sc := bufio.NewScanner(erros)
		for sc.Scan() {
			l := sc.Text()
			if ultimas = append(ultimas, l); len(ultimas) > 6 {
				ultimas = ultimas[1:]
			}
			select {
			case linhas <- l:
			default:
			}
		}
	}()
	aviso := make(chan string, 1)
	go decidirParada(linhas, aviso)

	houveVoz := true
	select {
	case motivo := <-aviso:
		houveVoz = motivo == "fim"
	case <-parar:
	case <-fimLeitura: // teto de tempo ou falha
	case <-ctx.Done():
	}
	_, _ = io.WriteString(entrada, "q") // pede ao ffmpeg para fechar o arquivo direito
	entrada.Close()
	<-fimLeitura
	errEspera := cmd.Wait()
	if info, err := os.Stat(saida); err != nil || info.Size() < 2000 {
		os.Remove(saida)
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", false, fmt.Errorf("não consegui gravar o microfone (%v): %s — confira a permissão de Microfone do terminal em Ajustes do Sistema", errEspera, strings.Join(ultimas, " | "))
	}
	return saida, houveVoz, nil
}

var (
	reCodigo  = regexp.MustCompile("`([^`]*)`")
	reMarca   = regexp.MustCompile(`[*_#>]+`)
	reIDTaref = regexp.MustCompile(`\b[0-9]{8}-[0-9]{6}-[0-9a-f]{4}\b`)
	reCaminho = regexp.MustCompile(`(/[\w.\-]+){2,}`)
)

// ParaFalar prepara um texto para ser dito: sem marcação, sem ids e com
// caminhos de pasta reduzidos ao último nome.
func ParaFalar(s string) string {
	s = reCodigo.ReplaceAllString(s, "$1")
	s = reIDTaref.ReplaceAllString(s, "")
	s = reCaminho.ReplaceAllStringFunc(s, func(c string) string { return filepath.Base(c) })
	s = reMarca.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxFalaCaracters {
		corte := string(r[:maxFalaCaracters])
		if i := strings.LastIndexAny(corte, ".!?"); i > maxFalaCaracters/2 {
			corte = corte[:i+1]
		}
		s = corte + " O resto está escrito na tela."
	}
	return s
}

// Sintetizar gera o áudio da fala e devolve o caminho do arquivo (o chamador apaga).
func Sintetizar(ctx context.Context, cfg nucleo.Config, texto string) (string, error) {
	texto = ParaFalar(texto)
	if texto == "" {
		return "", errors.New("nada para falar")
	}
	bin, err := exec.LookPath("mmx")
	if err != nil {
		return "", errors.New("comando mmx não encontrado: a fala precisa dele instalado e logado")
	}
	if err := os.MkdirAll(pastaVoz(), 0o700); err != nil {
		return "", err
	}
	saida := temporario(".mp3")
	ctx, cancelar := context.WithTimeout(ctx, 60*time.Second)
	defer cancelar()
	cmd := exec.CommandContext(ctx, bin, "speech", "synthesize", "--non-interactive", "--quiet",
		"--voice", cfg.Voz.Nome, "--language", "Portuguese", "--text-file", "-", "--out", saida)
	cmd.Args = append(cmd.Args, ajustes(cfg)...)
	cmd.Stdin = strings.NewReader(texto)
	var erros bytes.Buffer
	cmd.Stderr = &erros
	if err := cmd.Run(); err != nil {
		os.Remove(saida)
		det := strings.TrimSpace(erros.String())
		if r := []rune(det); len(r) > 300 {
			det = string(r[len(r)-300:])
		}
		return "", fmt.Errorf("não consegui gerar a fala (%v) %s", err, det)
	}
	if info, err := os.Stat(saida); err != nil || info.Size() < 500 {
		os.Remove(saida)
		return "", errors.New("a fala veio vazia")
	}
	return saida, nil
}

// Falar diz o texto começando a tocar assim que o primeiro pedaço do áudio
// chega (em vez de esperar o áudio inteiro). aoComecar é chamado nesse momento.
// Sem ffplay instalado, cai no caminho antigo: gera o arquivo e toca.
func Falar(ctx context.Context, cfg nucleo.Config, texto string, aoComecar func()) error {
	limpo := ParaFalar(texto)
	if limpo == "" {
		return errors.New("nada para falar")
	}
	gerador, errG := exec.LookPath("mmx")
	if cfg.Voz.Motor == "say" || (cfg.Voz.Motor == "" && errG != nil) {
		return falarComSay(ctx, cfg, limpo, aoComecar)
	}
	if errG != nil {
		return errors.New("comando mmx não encontrado (ou use a voz do sistema: \"motor\": \"sistema\" em voz)")
	}
	tocador, errT := exec.LookPath("ffplay")
	if errT != nil {
		arquivo, err := Sintetizar(ctx, cfg, texto)
		if err != nil {
			return err
		}
		defer os.Remove(arquivo)
		if aoComecar != nil {
			aoComecar()
		}
		return Tocar(ctx, arquivo)
	}
	ctx, cancelar := context.WithCancel(ctx)
	defer cancelar()
	gera := exec.CommandContext(ctx, gerador, "speech", "synthesize", "--non-interactive", "--quiet",
		"--voice", cfg.Voz.Nome, "--language", "Portuguese", "--text-file", "-", "--stream")
	gera.Args = append(gera.Args, ajustes(cfg)...)
	gera.Stdin = strings.NewReader(limpo)
	var erros bytes.Buffer
	gera.Stderr = &erros
	audio, err := gera.StdoutPipe()
	if err != nil {
		return err
	}
	args := []string{"-nodisp", "-autoexit", "-loglevel", "quiet", "-f", "mp3", "-i", "pipe:0"}
	if os.Getenv("MN_MUDO") != "" { // testes sem som
		args = append([]string{"-volume", "0"}, args...)
	}
	toca := exec.CommandContext(ctx, tocador, args...)
	entrada, err := toca.StdinPipe()
	if err != nil {
		return err
	}
	if err := gera.Start(); err != nil {
		return err
	}
	if err := toca.Start(); err != nil {
		cancelar()
		_ = gera.Wait()
		return err
	}
	buf := make([]byte, 16<<10)
	total := 0
	for {
		n, errL := audio.Read(buf)
		if n > 0 {
			if total == 0 && aoComecar != nil {
				aoComecar()
			}
			total += n
			if _, errE := entrada.Write(buf[:n]); errE != nil {
				break
			}
		}
		if errL != nil {
			break
		}
	}
	entrada.Close()
	errGera := gera.Wait()
	errToca := toca.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if total == 0 {
		det := strings.TrimSpace(erros.String())
		if r := []rune(det); len(r) > 300 {
			det = string(r[len(r)-300:])
		}
		return fmt.Errorf("não consegui gerar a fala (%v) %s", errGera, det)
	}
	return errToca
}

// Tocar toca um arquivo de áudio até o fim ou até o contexto ser cancelado.
func Tocar(ctx context.Context, arquivo string) error {
	return so.Tocar(ctx, arquivo)
}

// falarComSay usa a voz do próprio sistema: não precisa de conta nem de internet.
func falarComSay(ctx context.Context, cfg nucleo.Config, texto string, aoComecar func()) error {
	voz := cfg.Voz.Nome
	if strings.HasPrefix(voz, "Portuguese_") {
		voz = ""
	}
	if aoComecar != nil {
		aoComecar()
	}
	if os.Getenv("MN_MUDO") != "" {
		return nil
	}
	if err := so.FalarLocal(ctx, texto, voz); err != nil && ctx.Err() == nil {
		return fmt.Errorf("a voz do sistema falhou (%v)", err)
	}
	return ctx.Err()
}

// Duracao devolve os segundos de um áudio (0 se não der para saber).
func Duracao(arquivo string) float64 {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0
	}
	b, err := exec.Command(bin, "-v", "quiet", "-show_entries", "format=duration", "-of", "csv=p=0", arquivo).Output()
	if err != nil {
		return 0
	}
	d, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return d
}

var reNomeVoz = regexp.MustCompile(`"(Portuguese_[A-Za-z0-9_-]{1,60})"`)

// Vozes lista as vozes em português que a MiniMax oferece.
func Vozes(ctx context.Context) ([]string, error) {
	bin, err := exec.LookPath("mmx")
	if err != nil {
		return nil, errors.New("comando mmx não encontrado")
	}
	ctx, cancelar := context.WithTimeout(ctx, 40*time.Second)
	defer cancelar()
	saida, err := exec.CommandContext(ctx, bin, "speech", "voices", "--non-interactive").Output()
	if err != nil {
		return nil, fmt.Errorf("não consegui listar as vozes: %v", err)
	}
	var nomes []string
	visto := map[string]bool{}
	for _, m := range reNomeVoz.FindAllSubmatch(saida, -1) {
		if n := string(m[1]); !visto[n] {
			visto[n] = true
			nomes = append(nomes, n)
		}
	}
	return nomes, nil
}

// ajustes devolve as opções de tom e velocidade da fala (só o que foi pedido).
func ajustes(cfg nucleo.Config) []string {
	var a []string
	if t := cfg.Voz.Tom; t != 0 && t >= -12 && t <= 12 {
		a = append(a, "--pitch", strconv.Itoa(t))
	}
	if v := cfg.Voz.Velocidade; v >= 0.5 && v <= 2 && v != 1 {
		a = append(a, "--speed", strconv.FormatFloat(v, 'f', 2, 64))
	}
	return a
}
