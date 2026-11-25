// Package rosto é a cara do meia-noite: um orbe em WebGL servido só para este
// Mac (127.0.0.1), que muda conforme o estado do assistente.
package rosto

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/lrdeoliveira/meia-noite/internal/nucleo"
)

//go:embed web/*
var arquivos embed.FS

const PortaPadrao = 4780

// Estados que o orbe sabe desenhar.
var estadosValidos = map[string]bool{
	"parado": true, "ouvindo": true, "pensando": true, "falando": true, "trabalhando": true, "erro": true,
}

type Estado struct {
	Estado  string `json:"estado"`
	Rodando int    `json:"rodando"`
	// Legendas da conversa falada feita pela janela.
	Voce     string `json:"voce,omitempty"`
	Resposta string `json:"resposta,omitempty"`
	// Voz diz se a janela pode iniciar a conversa falada (Enter).
	Voz bool `json:"voz"`
	// Dica é a frase mostrada quando ele está parado (ex.: como chamar).
	Dica string `json:"dica,omitempty"`
}

var (
	legendaMu sync.Mutex
	legenda   struct{ voce, resposta, dica string }
)

// Legendar define o que a janela mostra como última fala e última resposta.
func Legendar(voce, resposta string) {
	legendaMu.Lock()
	legenda.voce, legenda.resposta = cortar(voce, 300), cortar(resposta, 700)
	legendaMu.Unlock()
}

// DefinirDica muda a frase mostrada quando o meia-noite está parado.
func DefinirDica(dica string) {
	legendaMu.Lock()
	legenda.dica = cortar(dica, 80)
	legendaMu.Unlock()
}

func cortar(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

type marcado struct {
	Estado string    `json:"estado"`
	Ate    time.Time `json:"ate"`
}

func arquivoEstado() string { return filepath.Join(nucleo.Casa(), "rosto.json") }

// Definir fixa um estado por alguns segundos. É por aqui que a voz (etapa
// seguinte) vai avisar "ouvindo", "pensando" e "falando".
func Definir(nome string, segundos int) error {
	if !estadosValidos[nome] {
		return fmt.Errorf("estado desconhecido: %q (use parado, ouvindo, pensando, falando, trabalhando ou erro)", nome)
	}
	if segundos <= 0 || segundos > 3600 {
		segundos = 10
	}
	if err := os.MkdirAll(nucleo.Casa(), 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(marcado{Estado: nome, Ate: time.Now().Add(time.Duration(segundos) * time.Second)})
	tmp := arquivoEstado() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, arquivoEstado())
}

// Liberar tira o estado fixado: o orbe volta a seguir as tarefas.
func Liberar() {
	if b, err := json.Marshal(marcado{Estado: "parado", Ate: time.Now()}); err == nil {
		_ = os.WriteFile(arquivoEstado(), b, 0o600)
	}
}

// Atual decide o que o orbe mostra agora: um estado fixado e ainda válido
// ganha; senão, vale o que as tarefas dizem.
func Atual() Estado {
	e := Estado{Estado: "parado"}
	falhaRecente := false
	for _, t := range nucleo.Listar(20) {
		if t.Ativa() {
			e.Rodando++
			continue
		}
		ruim := t.Estado == nucleo.Erro || t.Estado == nucleo.Bloqueada || t.Estado == nucleo.TempoEsgotado
		if ruim && time.Since(t.Terminada) < 6*time.Second {
			falhaRecente = true
		}
	}
	switch {
	case falhaRecente:
		e.Estado = "erro"
	case e.Rodando > 0:
		e.Estado = "trabalhando"
	}
	if b, err := os.ReadFile(arquivoEstado()); err == nil {
		var m marcado
		if json.Unmarshal(b, &m) == nil && estadosValidos[m.Estado] && time.Now().Before(m.Ate) {
			e.Estado = m.Estado
		}
	}
	return e
}

// Manipulador devolve as rotas do rosto. `tocar` é o que acontece quando a
// pessoa aperta Enter na janela (começar a ouvir, encerrar a fala dela ou
// interromper a resposta); nil deixa o rosto só de leitura.
func Manipulador(porta int, tocar func() string) http.Handler {
	web, _ := fs.Sub(arquivos, "web")
	estaticos := http.FileServer(http.FS(web))
	permitidos := map[string]bool{
		"127.0.0.1:" + strconv.Itoa(porta): true,
		"localhost:" + strconv.Itoa(porta): true,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /estado", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		e := Atual()
		e.Voz = tocar != nil
		legendaMu.Lock()
		e.Voce, e.Resposta, e.Dica = legenda.voce, legenda.resposta, legenda.dica
		legendaMu.Unlock()
		_ = json.NewEncoder(w).Encode(e)
	})
	origem := "http://127.0.0.1:" + strconv.Itoa(porta)
	origemNome := "http://localhost:" + strconv.Itoa(porta)
	mux.HandleFunc("POST /tocar", func(w http.ResponseWriter, r *http.Request) {
		if tocar == nil {
			http.Error(w, "voz desligada", http.StatusNotFound)
			return
		}
		// Só a própria janela pode apertar o botão: outro site não consegue
		// mandar este cabeçalho nem esta origem.
		o := r.Header.Get("Origin")
		if (o != origem && o != origemNome) || r.Header.Get("X-Meia-Noite") != "1" {
			http.Error(w, "recusado", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"acao": tocar()})
	})
	mux.Handle("GET /", estaticos)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Host fora da lista = outro site tentando alcançar este serviço por nome (DNS rebinding).
		if !permitidos[r.Host] {
			http.Error(w, "recusado", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

// Escutar abre a porta só em 127.0.0.1.
func Escutar(porta int) (net.Listener, error) {
	if porta < 1024 || porta > 65535 {
		return nil, errors.New("porta inválida (use entre 1024 e 65535)")
	}
	return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(porta)))
}

// Servir atende até o processo ser encerrado.
func Servir(l net.Listener, porta int, tocar func() string) error {
	s := &http.Server{
		Handler:           Manipulador(porta, tocar),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	return s.Serve(l)
}
