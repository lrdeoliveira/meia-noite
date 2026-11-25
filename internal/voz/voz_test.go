package voz

import (
	"strings"
	"testing"
	"time"
)

func TestLimparTranscricao(t *testing.T) {
	casos := map[string]string{
		"  Manda o Codex\n resumir.  ":       "Manda o Codex resumir.",
		"Legendas pela comunidade Amara.org": "",
		" Obrigado por assistir! ":           "",
		"[Música]":                           "",
		" . ":                                "",
		"":                                   "",
		"Oi":                                 "Oi",
	}
	for entrada, esperado := range casos {
		if got := LimparTranscricao(entrada); got != esperado {
			t.Errorf("LimparTranscricao(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}

func TestParaFalar(t *testing.T) {
	got := ParaFalar("O **codex** terminou a tarefa `20261125-190307-1cdd` em /Users/ana/projetos/meia-noite.\n\n# Fim")
	for _, ruim := range []string{"*", "`", "#", "20261125", "/Users/ana"} {
		if strings.Contains(got, ruim) {
			t.Errorf("sobrou %q em %q", ruim, got)
		}
	}
	if !strings.Contains(got, "meia-noite") || !strings.Contains(got, "codex terminou") {
		t.Errorf("perdeu conteúdo: %q", got)
	}
	longo := ParaFalar(strings.Repeat("Frase de teste bem comum. ", 200))
	if n := len([]rune(longo)); n > maxFalaCaracters+40 {
		t.Errorf("texto longo não foi cortado: %d caracteres", n)
	}
	if !strings.HasSuffix(longo, "O resto está escrito na tela.") {
		t.Errorf("faltou avisar do corte: %q", longo[len(longo)-60:])
	}
	if ParaFalar(" ** ") != "" {
		t.Error("só marcação deveria virar vazio")
	}
}

func decidir(t *testing.T, linhas ...string) string {
	t.Helper()
	entrada := make(chan string, len(linhas))
	for _, l := range linhas {
		entrada <- l
	}
	aviso := make(chan string, 1)
	go decidirParada(entrada, aviso)
	select {
	case m := <-aviso:
		return m
	case <-time.After(300 * time.Millisecond):
		return "segue gravando"
	}
}

func TestDecidirParada(t *testing.T) {
	// silêncio inicial, fala, silêncio de novo: acabou
	if m := decidir(t, "[silencedetect @ 0x1] silence_start: 0", "[silencedetect @ 0x1] silence_end: 1.8 | silence_duration: 1.8", "[silencedetect @ 0x1] silence_start: 5.2"); m != "fim" {
		t.Errorf("deveria encerrar, veio %q", m)
	}
	// começou falando direto e se calou
	if m := decidir(t, "[silencedetect @ 0x1] silence_start: 3.1"); m != "fim" {
		t.Errorf("deveria encerrar, veio %q", m)
	}
	// só o silêncio inicial: continua esperando a pessoa falar
	if m := decidir(t, "[silencedetect @ 0x1] silence_start: 0", "size= 12kB time=00:00:01"); m != "segue gravando" {
		t.Errorf("não deveria parar ainda, veio %q", m)
	}
	// ainda falando
	if m := decidir(t, "[silencedetect @ 0x1] silence_start: 0", "[silencedetect @ 0x1] silence_end: 1.2 | silence_duration: 1.2"); m != "segue gravando" {
		t.Errorf("não deveria parar no meio da fala, veio %q", m)
	}
}
