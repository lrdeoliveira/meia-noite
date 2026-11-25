package voz

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
	"github.com/lrdeoliveira/meia-noite/internal/so"
)

const (
	taxa         = 16000
	quadro       = 480 // 30 ms
	quadrosIni   = 4   // de 6 quadros acima do limiar = começou a falar
	janelaIni    = 6
	quadrosFim   = 40  // 1,2 s de silêncio = terminou (pausa para pensar não corta a frase)
	quadrosPre   = 10  // 300 ms guardados antes do começo, para não cortar a primeira sílaba
	minFalaQuad  = 10  // menos de 300 ms não é fala
	maxFalaQuad  = 500 // 15 s
	pisoAbsoluto = 220.0
)

var emFala atomic.Bool

// EmFala diz se há alguém falando agora (fala começada e ainda não terminada).
func EmFala() bool { return emFala.Load() }

// Recortador separa um fluxo de áudio em falas. O limiar acompanha o ruído do
// ambiente: ventilador ou TV baixa sobem o piso em vez de virarem "fala" sem fim.
type Recortador struct {
	piso      float64
	recentes  []bool
	pre       [][]int16
	fala      []int16
	falando   bool
	quadFala  int
	silencio  int
	iniciados int
}

func rms(q []int16) float64 {
	var soma float64
	for _, v := range q {
		soma += float64(v) * float64(v)
	}
	return math.Sqrt(soma / float64(len(q)))
}

// Falando diz se há uma fala em andamento.
func (r *Recortador) Falando() bool { return r.falando }

// Alimentar recebe um quadro de 30 ms e devolve uma fala inteira quando ela termina.
func (r *Recortador) Alimentar(q []int16) []int16 {
	nivel := rms(q)
	if r.iniciados < 15 { // primeiro meio segundo calibra o piso
		r.iniciados++
		r.piso = math.Max(r.piso, nivel)
		return nil
	}
	limiar := math.Max(r.piso*2.6, pisoAbsoluto)
	alto := nivel > limiar
	if !r.falando {
		// piso sobe devagar e desce depressa
		if nivel < r.piso {
			r.piso = r.piso*0.9 + nivel*0.1
		} else if !alto {
			r.piso = r.piso*0.995 + nivel*0.005
		}
		r.recentes = append(r.recentes, alto)
		if len(r.recentes) > janelaIni {
			r.recentes = r.recentes[1:]
		}
		r.pre = append(r.pre, append([]int16(nil), q...))
		if len(r.pre) > quadrosPre {
			r.pre = r.pre[1:]
		}
		n := 0
		for _, a := range r.recentes {
			if a {
				n++
			}
		}
		if n >= quadrosIni {
			r.falando, r.quadFala, r.silencio = true, 0, 0
			r.fala = r.fala[:0]
			for _, p := range r.pre {
				r.fala = append(r.fala, p...)
			}
			r.pre, r.recentes = nil, nil
		}
		return nil
	}
	r.fala = append(r.fala, q...)
	r.quadFala++
	if alto {
		r.silencio = 0
	} else {
		r.silencio++
	}
	if r.silencio < quadrosFim && r.quadFala < maxFalaQuad {
		return nil
	}
	r.falando = false
	util := r.quadFala - r.silencio
	if util < minFalaQuad {
		return nil
	}
	return append([]int16(nil), r.fala...)
}

// Escutar mantém o microfone aberto e chama `aoOuvir` a cada fala detectada.
// `surdo` (pode ser nil) faz ignorar o áudio enquanto devolver true — por
// exemplo, enquanto o próprio meia-noite fala, para ele não se ouvir.
func Escutar(ctx context.Context, cfg nucleo.Config, surdo func() bool, aoOuvir func(fala []int16)) error {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("ffmpeg não encontrado (instale o ffmpeg)")
	}
	mic, err := so.Microfone(cfg.Voz.Microfone)
	if err != nil {
		return err
	}
	args := append([]string{"-hide_banner", "-nostats", "-loglevel", "error"}, mic...)
	args = append(args, "-ar", strconv.Itoa(taxa), "-ac", "1", "-f", "s16le", "pipe:1")
	cmd := exec.CommandContext(ctx, bin, args...)
	saida, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Wait()
	leitor := bufio.NewReaderSize(saida, 64<<10)
	bruto := make([]byte, quadro*2)
	q := make([]int16, quadro)
	rec := &Recortador{}
	lidos := 0
	for {
		if _, err := io.ReadFull(leitor, bruto); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if lidos == 0 {
				return errors.New("o microfone não entregou áudio — confira a permissão de Microfone em Ajustes do Sistema")
			}
			return errors.New("o microfone parou de entregar áudio")
		}
		lidos++
		if surdo != nil && surdo() {
			if rec.falando || len(rec.pre) > 0 {
				piso := rec.piso
				rec = &Recortador{piso: piso, iniciados: 15}
			}
			continue
		}
		for i := range q {
			q[i] = int16(binary.LittleEndian.Uint16(bruto[i*2:]))
		}
		fala := rec.Alimentar(q)
		emFala.Store(rec.falando)
		if fala != nil {
			aoOuvir(fala)
		}
	}
}

// GravarWAV grava uma fala num arquivo temporário (o chamador apaga).
func GravarWAV(fala []int16) (string, error) {
	if err := os.MkdirAll(pastaVoz(), 0o700); err != nil {
		return "", err
	}
	caminho := temporario(".wav")
	f, err := os.OpenFile(caminho, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	dados := uint32(len(fala) * 2)
	w := bufio.NewWriter(f)
	w.WriteString("RIFF")
	binary.Write(w, binary.LittleEndian, 36+dados)
	w.WriteString("WAVEfmt ")
	binary.Write(w, binary.LittleEndian, uint32(16))
	binary.Write(w, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(w, binary.LittleEndian, uint16(1)) // mono
	binary.Write(w, binary.LittleEndian, uint32(taxa))
	binary.Write(w, binary.LittleEndian, uint32(taxa*2))
	binary.Write(w, binary.LittleEndian, uint16(2))
	binary.Write(w, binary.LittleEndian, uint16(16))
	w.WriteString("data")
	binary.Write(w, binary.LittleEndian, dados)
	binary.Write(w, binary.LittleEndian, fala)
	return caminho, w.Flush()
}

func simplificar(s string) string {
	troca := strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o", "ú", "u", "ç", "c")
	s = troca.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// formas como "meia-noite" costuma sair na transcrição
var chamadas = []string{"meia noite", "meianoite", "meia noiti", "meia noit", "meia noites", "mea noite"}

// Chamada diz se a frase chama o meia-noite e devolve o que veio depois do
// nome (o pedido, se a pessoa já emendou).
func Chamada(texto string) (bool, string) {
	simples := simplificar(texto)
	for _, c := range chamadas {
		i := strings.Index(simples, c)
		if i < 0 {
			continue
		}
		// o nome tem que estar no começo da frase (até 3 palavras antes: "ei", "oi", "e aí")
		if len(strings.Fields(simples[:i])) > 3 {
			continue
		}
		resto := strings.TrimSpace(simples[i+len(c):])
		// devolve o resto com a grafia original, cortando pelo número de palavras
		orig := strings.Fields(texto)
		n := len(strings.Fields(resto))
		if n == 0 {
			return true, ""
		}
		if n > len(orig) {
			n = len(orig)
		}
		return true, strings.TrimLeft(strings.Join(orig[len(orig)-n:], " "), " ,.!?-")
	}
	return false, ""
}
