// Package nucleo é o coração do meia-noite: configuração, regras de segurança,
// catálogo de IAs e ciclo de vida das tarefas.
package nucleo

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Casa devolve a pasta de dados (tarefas, config, modelos). Ordem: variável
// MN_HOME; caminho escrito em ~/.config/meia-noite/casa (para quem guarda os
// dados em outro disco); senão ~/.meia-noite.
func Casa() string {
	if v := os.Getenv("MN_HOME"); v != "" {
		return v
	}
	lar, err := os.UserHomeDir()
	if err != nil {
		return ".meia-noite"
	}
	if b, err := os.ReadFile(filepath.Join(lar, ".config", "meia-noite", "casa")); err == nil {
		if p := strings.TrimSpace(string(b)); filepath.IsAbs(p) {
			return p
		}
	}
	return filepath.Join(lar, ".meia-noite")
}

type Config struct {
	// Navegador onde o rosto abre (só macOS; vazio = o padrão do sistema).
	Navegador string `json:"navegador"`
	// Como o meia-noite chama você (vazio = não usa nome).
	Usuario string `json:"usuario"`
	// Só dentro destas pastas uma IA pode ser disparada.
	PastasPermitidas []string `json:"pastas_permitidas"`
	MaxSimultaneas   int      `json:"max_simultaneas"`
	MaxPorDia        int      `json:"max_por_dia"`
	TempoLimiteMin   int      `json:"tempo_limite_min"`
	// Modelo por IA (vazio = padrão da própria CLI). A chave "cerebro" é o modelo da conversa.
	Modelos map[string]string `json:"modelos"`
	// Quantas chamadas ao cérebro por dia (cada volta de conversa conta uma).
	MaxCerebroPorDia int     `json:"max_cerebro_por_dia"`
	Voz              Voz     `json:"voz"`
	Nexusyn          Nexusyn `json:"nexusyn"`
	Cerebro          Cerebro `json:"cerebro"`
}

// Cerebro é o modelo que decide o que fazer. Pode ser qualquer API de conversa
// com ferramentas. A chave não fica aqui: vem de variável de ambiente
// (chave_env), do 1Password (ref_chave) ou das Chaves do macOS (`mn chave`).
type Cerebro struct {
	Tipo     string `json:"tipo"`      // "openai", "anthropic" ou "mmx"
	URL      string `json:"url"`       // ex.: https://api.openai.com/v1
	Modelo   string `json:"modelo"`    // nome do modelo nesse provedor
	ChaveEnv string `json:"chave_env"` // nome da variável com a chave (padrão MN_CEREBRO_CHAVE)
	RefChave string `json:"ref_chave"` // referência op://… do 1Password
}

// Nexusyn diz onde fica a memória e de onde vem a chave (uma referência do
// 1Password, nunca a chave em si).
type Nexusyn struct {
	URL      string `json:"url"`
	RefChave string `json:"ref_chave"`
}

// Voz reúne o que a fala e a escuta precisam.
type Voz struct {
	Tom         int      `json:"tom"`         // motor mmx: -12 (mais grave) a 12 (mais agudo); 0 = natural
	Velocidade  float64  `json:"velocidade"`  // motor mmx: 0.5 a 2; 0 = natural
	Motor       string   `json:"motor"`       // "mmx" (voz da MiniMax) ou "sistema" (voz do próprio computador, sem conta); vazio = mmx se houver, senão sistema
	Nome        string   `json:"nome"`        // voz da fala
	Microfone   string   `json:"microfone"`   // índice do microfone no macOS ("0" = embutido)
	SilencioDB  int      `json:"silencio_db"` // abaixo disto é silêncio (negativo; mais perto de zero = menos sensível)
	ModeloOuve  string   `json:"modelo_ouve"` // arquivo do modelo de transcrição
	Vocabulario []string `json:"vocabulario"` // nomes próprios que você costuma dizer (produtos, pessoas)
	ModeloVAD   string   `json:"modelo_vad"`  // arquivo do filtro de voz (descarta o que não é fala)
	Escuta      string   `json:"escuta"`      // "chamada" = atende ao ouvir "meia-noite"; "enter" = só com Enter
	Editar      string   `json:"editar"`      // "confirmar" = altera arquivos por voz depois de um "sim"; "nao" = nunca
}

func ConfigPadrao() Config {
	return Config{
		// Vazio de propósito: você escolhe onde as IAs podem trabalhar.
		PastasPermitidas: []string{},
		MaxSimultaneas:   3,
		MaxPorDia:        40,
		TempoLimiteMin:   30,
		// Sem modelo explícito o OpenCode cai num provedor sem crédito.
		Modelos:          map[string]string{},
		MaxCerebroPorDia: 400,
		// Memória é opcional: só liga quando ref_chave estiver preenchida.
		Nexusyn: Nexusyn{URL: "https://api.nexusyn.ai/v1/mcp"},
		Voz: Voz{
			Nome:       "Portuguese_Deep-VoicedGentleman",
			Microfone:  "0",
			SilencioDB: -30,
			ModeloOuve: filepath.Join(Casa(), "modelos", "ggml-large-v3-turbo-q5_0.bin"),
			ModeloVAD:  filepath.Join(Casa(), "modelos", "ggml-silero-v5.1.2.bin"),
			Escuta:     "chamada",
			Editar:     "confirmar",
		},
	}
}

// SalvarConfig grava a configuração.
func SalvarConfig(cfg Config) error {
	if err := os.MkdirAll(Casa(), 0o700); err != nil {
		return err
	}
	return gravarJSON(caminhoConfig(), cfg)
}

func caminhoConfig() string { return filepath.Join(Casa(), "config.json") }

// CarregarConfig lê a config; na primeira vez grava a padrão.
func CarregarConfig() (Config, error) {
	cfg := ConfigPadrao()
	b, err := os.ReadFile(caminhoConfig())
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(Casa(), 0o700); err != nil {
			return cfg, err
		}
		return cfg, gravarJSON(caminhoConfig(), cfg)
	}
	if err != nil {
		return cfg, err
	}
	lida := Config{}
	if err := json.Unmarshal(b, &lida); err != nil {
		return cfg, errors.New("config.json inválido: " + err.Error())
	}
	// Campo ausente ou zerado mantém o padrão: limite zero nunca vira "sem limite".
	if len(lida.PastasPermitidas) > 0 {
		cfg.PastasPermitidas = lida.PastasPermitidas
	}
	if lida.MaxSimultaneas > 0 {
		cfg.MaxSimultaneas = lida.MaxSimultaneas
	}
	if lida.MaxPorDia > 0 {
		cfg.MaxPorDia = lida.MaxPorDia
	}
	if lida.TempoLimiteMin > 0 {
		cfg.TempoLimiteMin = lida.TempoLimiteMin
	}
	if lida.Cerebro.Tipo != "" {
		cfg.Cerebro = lida.Cerebro
	}
	if lida.Nexusyn.URL != "" {
		cfg.Nexusyn.URL = lida.Nexusyn.URL
	}
	if lida.Nexusyn.RefChave != "" {
		cfg.Nexusyn.RefChave = lida.Nexusyn.RefChave
	}
	if lida.Voz.Tom >= -12 && lida.Voz.Tom <= 12 {
		cfg.Voz.Tom = lida.Voz.Tom
	}
	if lida.Voz.Velocidade >= 0.5 && lida.Voz.Velocidade <= 2 {
		cfg.Voz.Velocidade = lida.Voz.Velocidade
	}
	if lida.Navegador != "" {
		cfg.Navegador = lida.Navegador
	}
	if lida.Voz.Motor == "sistema" {
		lida.Voz.Motor = "say"
	}
	if lida.Voz.Motor == "mmx" || lida.Voz.Motor == "say" {
		cfg.Voz.Motor = lida.Voz.Motor
	}
	if lida.Voz.Nome != "" {
		cfg.Voz.Nome = lida.Voz.Nome
	}
	if lida.Voz.Microfone != "" {
		cfg.Voz.Microfone = lida.Voz.Microfone
	}
	if lida.Voz.SilencioDB < 0 {
		cfg.Voz.SilencioDB = lida.Voz.SilencioDB
	}
	if lida.Usuario != "" {
		cfg.Usuario = lida.Usuario
	}
	if len(lida.Voz.Vocabulario) > 0 {
		cfg.Voz.Vocabulario = lida.Voz.Vocabulario
	}
	if lida.Voz.ModeloVAD != "" {
		cfg.Voz.ModeloVAD = lida.Voz.ModeloVAD
	}
	if lida.Voz.Escuta == "chamada" || lida.Voz.Escuta == "enter" {
		cfg.Voz.Escuta = lida.Voz.Escuta
	}
	if lida.Voz.Editar == "confirmar" || lida.Voz.Editar == "nao" {
		cfg.Voz.Editar = lida.Voz.Editar
	}
	if lida.Voz.ModeloOuve != "" {
		cfg.Voz.ModeloOuve = lida.Voz.ModeloOuve
	}
	if lida.MaxCerebroPorDia > 0 {
		cfg.MaxCerebroPorDia = lida.MaxCerebroPorDia
	}
	for k, v := range lida.Modelos {
		cfg.Modelos[k] = v
	}
	return cfg, nil
}

// gravarJSON escreve de forma atômica (arquivo temporário + rename).
func gravarJSON(caminho string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := caminho + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, caminho)
}
