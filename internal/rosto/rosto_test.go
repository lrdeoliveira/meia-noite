package rosto

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pedir(t *testing.T, metodo, caminho, host string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(metodo, caminho, nil)
	r.Host = host
	w := httptest.NewRecorder()
	Manipulador(4780, nil).ServeHTTP(w, r)
	return w
}

func TestSoAtendeHostLocal(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	for _, host := range []string{"evil.example", "127.0.0.1:9999", "127.0.0.1", "meia-noite.evil.example:4780", ""} {
		if w := pedir(t, "GET", "/estado", host); w.Code != http.StatusForbidden {
			t.Errorf("host %q deveria ser recusado, veio %d", host, w.Code)
		}
	}
	for _, host := range []string{"127.0.0.1:4780", "localhost:4780"} {
		if w := pedir(t, "GET", "/estado", host); w.Code != http.StatusOK {
			t.Errorf("host %q deveria ser aceito, veio %d", host, w.Code)
		}
	}
}

func TestSomenteLeitura(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if w := pedir(t, m, "/estado", "127.0.0.1:4780"); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /estado deveria ser recusado, veio %d", m, w.Code)
		}
	}
}

func TestCabecalhosDeSeguranca(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	w := pedir(t, "GET", "/", "127.0.0.1:4780")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "orbe.js") {
		t.Fatalf("página inicial não veio: %d", w.Code)
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "unsafe") {
		t.Errorf("CSP fraca: %q", csp)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("faltam cabeçalhos de segurança")
	}
	// a página não pode depender de nada de fora (CSP barraria, e o projeto é autocontido)
	for _, arq := range []string{"/", "/orbe.js", "/orbe.css"} {
		corpo := pedir(t, "GET", arq, "127.0.0.1:4780").Body.String()
		if strings.Contains(corpo, "http://") || strings.Contains(corpo, "https://") {
			t.Errorf("%s referencia endereço externo", arq)
		}
	}
}

func TestEstado(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	ler := func() Estado {
		var e Estado
		if err := json.Unmarshal(pedir(t, "GET", "/estado", "127.0.0.1:4780").Body.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	if e := ler(); e.Estado != "parado" || e.Rodando != 0 {
		t.Errorf("sem tarefas deveria estar parado: %+v", e)
	}
	if err := Definir("falando", 30); err != nil {
		t.Fatal(err)
	}
	if e := ler(); e.Estado != "falando" {
		t.Errorf("estado fixado não apareceu: %+v", e)
	}
	for _, ruim := range []string{"", "dançando", "../../x", "<script>"} {
		if err := Definir(ruim, 5); err == nil {
			t.Errorf("estado %q deveria ser recusado", ruim)
		}
	}
	if e := ler(); e.Estado != "falando" {
		t.Errorf("pedido recusado não pode mudar o estado: %+v", e)
	}
}

func TestPortaInvalida(t *testing.T) {
	for _, p := range []int{0, 80, 70000, -1} {
		if l, err := Escutar(p); err == nil {
			l.Close()
			t.Errorf("porta %d deveria ser recusada", p)
		}
	}
}

func tocarCom(t *testing.T, origem, cabecalho string, tocar func() string) int {
	t.Helper()
	r := httptest.NewRequest("POST", "/tocar", nil)
	r.Host = "127.0.0.1:4780"
	if origem != "" {
		r.Header.Set("Origin", origem)
	}
	if cabecalho != "" {
		r.Header.Set("X-Meia-Noite", cabecalho)
	}
	w := httptest.NewRecorder()
	Manipulador(4780, tocar).ServeHTTP(w, r)
	return w.Code
}

func TestTocarSoPelaPropriaJanela(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	vezes := 0
	tocar := func() string { vezes++; return "ouvindo" }
	if c := tocarCom(t, "http://127.0.0.1:4780", "1", tocar); c != http.StatusOK || vezes != 1 {
		t.Fatalf("a própria janela deveria conseguir: código %d, chamadas %d", c, vezes)
	}
	recusas := [][2]string{
		{"https://evil.example", "1"},  // outro site
		{"http://127.0.0.1:9999", "1"}, // outra porta local
		{"", "1"},                      // sem origem (curl, formulário antigo)
		{"http://127.0.0.1:4780", ""},  // sem o cabeçalho próprio
		{"null", "1"},
	}
	for _, c := range recusas {
		if cod := tocarCom(t, c[0], c[1], tocar); cod != http.StatusForbidden {
			t.Errorf("origem %q cabeçalho %q deveria ser recusado, veio %d", c[0], c[1], cod)
		}
	}
	if vezes != 1 {
		t.Errorf("pedido recusado não pode acionar a voz (chamadas: %d)", vezes)
	}
	if c := tocarCom(t, "http://127.0.0.1:4780", "1", nil); c != http.StatusNotFound {
		t.Errorf("sem voz ligada deveria dar 404, veio %d", c)
	}
}

func TestLegendaApareceNoEstado(t *testing.T) {
	t.Setenv("MN_HOME", t.TempDir())
	Legendar("oi", "olá, Ana")
	defer Legendar("", "")
	r := httptest.NewRequest("GET", "/estado", nil)
	r.Host = "127.0.0.1:4780"
	w := httptest.NewRecorder()
	Manipulador(4780, func() string { return "" }).ServeHTTP(w, r)
	var e Estado
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Voce != "oi" || e.Resposta != "olá, Ana" || !e.Voz {
		t.Errorf("estado sem legenda: %+v", e)
	}
}
