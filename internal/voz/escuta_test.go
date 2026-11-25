package voz

import (
	"math"
	"math/rand"
	"os"
	"testing"
)

// som gera n quadros de ruído com a amplitude dada (voz = amplitude alta).
func som(r *rand.Rand, quadros int, amp float64) [][]int16 {
	out := make([][]int16, quadros)
	for i := range out {
		q := make([]int16, quadro)
		for j := range q {
			q[j] = int16((r.Float64()*2 - 1) * amp)
		}
		out[i] = q
	}
	return out
}

func passar(rec *Recortador, quadros [][]int16) (falas [][]int16) {
	for _, q := range quadros {
		if f := rec.Alimentar(q); f != nil {
			falas = append(falas, f)
		}
	}
	return
}

func TestRecortadorSeparaFalas(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	rec := &Recortador{}
	var fluxo [][]int16
	fluxo = append(fluxo, som(r, 40, 120)...)  // ambiente
	fluxo = append(fluxo, som(r, 50, 4000)...) // fala de 1,5 s
	fluxo = append(fluxo, som(r, 60, 120)...)  // pausa longa (1,8 s)
	fluxo = append(fluxo, som(r, 30, 4000)...) // segunda fala
	fluxo = append(fluxo, som(r, 60, 120)...)
	falas := passar(rec, fluxo)
	if len(falas) != 2 {
		t.Fatalf("esperava 2 falas, veio %d", len(falas))
	}
	seg := float64(len(falas[0])) / taxa
	if seg < 1.5 || seg > 3.3 {
		t.Errorf("primeira fala com %.1f s (esperado ~1,5 s + margens)", seg)
	}
}

func TestRecortadorIgnoraEstaloERuidoConstante(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	rec := &Recortador{}
	var fluxo [][]int16
	fluxo = append(fluxo, som(r, 40, 120)...)
	fluxo = append(fluxo, som(r, 3, 6000)...) // estalo de 90 ms
	fluxo = append(fluxo, som(r, 60, 120)...)
	if falas := passar(rec, fluxo); len(falas) != 0 {
		t.Errorf("estalo não é fala, veio %d", len(falas))
	}
	// ambiente barulhento e constante (ventilador): o piso sobe e não vira fala sem fim
	rec = &Recortador{}
	if falas := passar(rec, som(r, 600, 900)); len(falas) != 0 {
		t.Errorf("ruído constante não é fala, veio %d", len(falas))
	}
	// e uma fala por cima desse ruído ainda é percebida
	fluxo = append(som(r, 40, 6000), som(r, 40, 900)...)
	if falas := passar(rec, fluxo); len(falas) != 1 {
		t.Errorf("fala sobre ruído deveria ser percebida, veio %d", len(falas))
	}
}

func TestRecortadorCortaFalaSemFim(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	rec := &Recortador{}
	falas := passar(rec, append(som(r, 30, 120), som(r, maxFalaQuad+200, 5000)...))
	if len(falas) == 0 || float64(len(falas[0]))/taxa > 15.5 {
		t.Errorf("fala sem pausa deveria ser cortada em 15 s")
	}
}

func TestChamada(t *testing.T) {
	casos := []struct {
		texto  string
		chamou bool
		resto  string
	}{
		{"Ei, meia-noite.", true, ""},
		{"Meia noite!", true, ""},
		{"Hey, Meia-Noite, manda o Codex resumir o projeto Loja.", true, "manda o Codex resumir o projeto Loja."},
		{"e aí meianoite que horas são", true, "que horas são"},
		{"Oi meia noite, tudo bem?", true, "tudo bem?"},
		{"Ontem eu fui dormir depois da meia-noite de novo", false, ""},
		{"Manda o codex resumir o projeto", false, ""},
		{"", false, ""},
	}
	for _, c := range casos {
		chamou, resto := Chamada(c.texto)
		if chamou != c.chamou || resto != c.resto {
			t.Errorf("Chamada(%q) = %v, %q; esperado %v, %q", c.texto, chamou, resto, c.chamou, c.resto)
		}
	}
}

func TestGravarWAV(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	fala := make([]int16, taxa) // 1 s
	for i := range fala {
		fala[i] = int16(8000 * math.Sin(float64(i)/20))
	}
	caminho, err := GravarWAV(fala)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(caminho)
	info, _ := os.Stat(caminho)
	if info.Size() != int64(44+len(fala)*2) {
		t.Errorf("tamanho do wav errado: %d", info.Size())
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("áudio deveria ser só do dono: %v", info.Mode().Perm())
	}
}

// uma pausa curta para pensar (0,9 s) não pode partir a frase em duas
func TestRecortadorNaoCortaNaPausaCurta(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	rec := &Recortador{}
	var fluxo [][]int16
	fluxo = append(fluxo, som(r, 40, 120)...)
	fluxo = append(fluxo, som(r, 30, 4000)...)
	fluxo = append(fluxo, som(r, 30, 120)...) // 0,9 s pensando
	fluxo = append(fluxo, som(r, 30, 4000)...)
	fluxo = append(fluxo, som(r, 60, 120)...)
	if falas := passar(rec, fluxo); len(falas) != 1 {
		t.Errorf("pausa curta deveria manter uma fala só, veio %d", len(falas))
	}
}
