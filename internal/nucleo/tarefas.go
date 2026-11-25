package nucleo

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/lrdeoliveira/meia-noite/internal/so"
)

const (
	Rodando       = "rodando"
	Terminou      = "terminou"
	Erro          = "erro"
	Cancelada     = "cancelada"
	Bloqueada     = "bloqueada"
	TempoEsgotado = "tempo_esgotado"
)

type Tarefa struct {
	ID        string    `json:"id"`
	IA        string    `json:"ia"`
	Pasta     string    `json:"pasta"`
	Pedido    string    `json:"pedido"`
	Modo      string    `json:"modo"`
	Origem    string    `json:"origem"`
	Estado    string    `json:"estado"`
	Criada    time.Time `json:"criada"`
	Terminada time.Time `json:"terminada,omitzero"`
	PID       int       `json:"pid,omitempty"`       // processo que vigia a IA
	PIDFilho  int       `json:"pid_filho,omitempty"` // a IA em si (líder do grupo)
	Passos    int       `json:"passos"`
	Sessao    string    `json:"sessao,omitempty"`
	CustoUSD  float64   `json:"custo_usd,omitempty"`
	Resposta  string    `json:"resposta,omitempty"`
	Erro      string    `json:"erro,omitempty"`
}

func (t Tarefa) Ativa() bool { return t.Estado == Rodando }

func pastaTarefas() string           { return filepath.Join(Casa(), "tarefas") }
func pastaTarefa(id string) string   { return filepath.Join(pastaTarefas(), id) }
func arquivoTarefa(id string) string { return filepath.Join(pastaTarefa(id), "tarefa.json") }

func salvar(t *Tarefa) error { return gravarJSON(arquivoTarefa(t.ID), t) }

func lerCru(id string) (*Tarefa, error) {
	if !IDValido(id) {
		return nil, errors.New("id de tarefa inválido")
	}
	b, err := os.ReadFile(arquivoTarefa(id))
	if err != nil {
		return nil, fmt.Errorf("tarefa %s não encontrada", id)
	}
	t := &Tarefa{}
	if err := json.Unmarshal(b, t); err != nil {
		return nil, err
	}
	return t, nil
}

func vivo(pid int) bool { return so.Vivo(pid) }

// Ler devolve a tarefa já conferida: se consta rodando mas o vigia morreu,
// ela é marcada como erro em vez de ficar "rodando" para sempre.
func Ler(id string) (*Tarefa, error) {
	t, err := lerCru(id)
	if err != nil {
		return nil, err
	}
	if t.Estado != Rodando {
		return t, nil
	}
	morto := t.PID > 0 && !vivo(t.PID)
	naoSubiu := t.PID == 0 && time.Since(t.Criada) > 20*time.Second
	if morto || naoSubiu {
		// Relê: o vigia pode ter gravado o desfecho entre as duas leituras.
		if t2, err := lerCru(id); err == nil && t2.Estado != Rodando {
			return t2, nil
		}
		t.Estado, t.Terminada = Erro, time.Now()
		t.Erro = "a tarefa foi interrompida (o processo que a acompanhava não está mais rodando)"
		_ = salvar(t)
	}
	return t, nil
}

// Listar devolve as tarefas mais recentes primeiro.
func Listar(max int) []*Tarefa {
	entradas, _ := os.ReadDir(pastaTarefas())
	var ids []string
	for _, e := range entradas {
		if e.IsDir() && IDValido(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	var out []*Tarefa
	for _, id := range ids {
		if max > 0 && len(out) >= max {
			break
		}
		if t, err := Ler(id); err == nil {
			out = append(out, t)
		}
	}
	return out
}

func novoID() string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// Disparar valida tudo, registra a tarefa e sobe um processo vigia separado.
// Volta na hora com o id; a IA continua rodando mesmo se quem chamou sair.
func Disparar(cfg Config, nomeIA, pasta, pedido, modo, origem string) (*Tarefa, error) {
	ia, ok := Catalogo[nomeIA]
	if !ok {
		return nil, fmt.Errorf("IA desconhecida: %q (use uma destas: %s)", nomeIA, strings.Join(nomesIAs(), ", "))
	}
	if modo == "" {
		modo = string(Leitura)
	}
	if modo != string(Leitura) && modo != string(Edicao) {
		return nil, fmt.Errorf("modo inválido: %q (use leitura ou edicao)", modo)
	}
	if _, err := exec.LookPath(ia.Bin); err != nil {
		return nil, fmt.Errorf("%s não está instalado neste Mac (comando %q não encontrado)", ia.Descricao, ia.Bin)
	}
	pastaReal, err := ValidarPasta(cfg, pasta)
	if err != nil {
		return nil, err
	}
	pedido, err = NormalizarPedido(pedido)
	if err != nil {
		return nil, err
	}
	// Monta os argumentos já aqui para recusar cedo um modo não suportado.
	if _, err := ia.Args(ParamsArgs{Modo: Modo(modo), Pasta: pastaReal, Pedido: pedido, ArqFinal: "/dev/null", LimiteSegs: 60}); err != nil {
		return nil, err
	}

	hoje, ativas := time.Now().Format("20060102"), 0
	doDia := 0
	for _, t := range Listar(0) {
		if strings.HasPrefix(t.ID, hoje) {
			doDia++
		}
		if t.Ativa() {
			ativas++
		}
	}
	if ativas >= cfg.MaxSimultaneas {
		return nil, fmt.Errorf("já há %d tarefas rodando (limite %d); espere uma terminar ou cancele", ativas, cfg.MaxSimultaneas)
	}
	if doDia >= cfg.MaxPorDia {
		return nil, fmt.Errorf("limite diário atingido (%d tarefas hoje)", doDia)
	}

	t := &Tarefa{ID: novoID(), IA: nomeIA, Pasta: pastaReal, Pedido: pedido, Modo: modo,
		Origem: origem, Estado: Rodando, Criada: time.Now()}
	if err := os.MkdirAll(pastaTarefa(t.ID), 0o700); err != nil {
		return nil, err
	}
	if err := salvar(t); err != nil {
		return nil, err
	}

	eu, err := os.Executable()
	if err != nil {
		return nil, err
	}
	diario, err := os.OpenFile(filepath.Join(pastaTarefa(t.ID), "vigia.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	defer diario.Close()
	vigia := exec.Command(eu, "_executar", t.ID)
	vigia.Env = append(os.Environ(), "MN_HOME="+Casa())
	vigia.Stdout, vigia.Stderr = diario, diario
	so.Destacar(vigia) // sobrevive a quem disparou
	if err := vigia.Start(); err != nil {
		t.Estado, t.Erro, t.Terminada = Erro, "não foi possível iniciar: "+err.Error(), time.Now()
		_ = salvar(t)
		return t, err
	}
	_ = vigia.Process.Release()
	return t, nil
}

func nomesIAs() []string {
	var n []string
	for k := range Catalogo {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// Executar é o vigia: roda a IA, grava a saída crua, extrai o resultado e
// fecha a tarefa. É chamado só pelo próprio binário (subcomando interno).
func Executar(id string) error {
	t, err := lerCru(id)
	if err != nil {
		return err
	}
	if t.Estado != Rodando || t.PID != 0 {
		return errors.New("tarefa já iniciada ou encerrada")
	}
	cfg, err := CarregarConfig()
	if err != nil {
		return err
	}
	ia := Catalogo[t.IA]
	dir := pastaTarefa(id)
	arqFinal := filepath.Join(dir, "ultima.txt")
	limite := time.Duration(cfg.TempoLimiteMin) * time.Minute

	falhar := func(msg string) error {
		t.Estado, t.Erro, t.Terminada = Erro, msg, time.Now()
		_ = salvar(t)
		return errors.New(msg)
	}
	args, err := ia.Args(ParamsArgs{Modo: Modo(t.Modo), Pasta: t.Pasta, Pedido: t.Pedido,
		Modelo: cfg.Modelos[t.IA], ArqFinal: arqFinal, LimiteSegs: int(limite.Seconds())})
	if err != nil {
		return falhar(err.Error())
	}

	saida, err := os.OpenFile(filepath.Join(dir, "saida.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return falhar(err.Error())
	}
	defer saida.Close()
	arqErro, err := os.OpenFile(filepath.Join(dir, "erro.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return falhar(err.Error())
	}
	defer arqErro.Close()

	cmd := exec.Command(ia.Bin, args...) // sem shell: cada argumento chega intacto
	cmd.Dir = t.Pasta
	cmd.Stderr = arqErro
	so.GrupoProprio(cmd) // grupo próprio: dá para matar a IA e os filhos dela
	cano, err := cmd.StdoutPipe()
	if err != nil {
		return falhar(err.Error())
	}
	if err := cmd.Start(); err != nil {
		return falhar("não foi possível iniciar " + ia.Descricao + ": " + err.Error())
	}
	t.PID, t.PIDFilho = os.Getpid(), cmd.Process.Pid
	_ = salvar(t)

	sinais := make(chan os.Signal, 1)
	signal.Notify(sinais, syscall.SIGTERM, syscall.SIGINT)

	var parcial Parcial
	var codigoSaida int
	fim := make(chan struct{})
	go func() {
		defer close(fim)
		leitor := bufio.NewReaderSize(cano, 1<<20)
		ultimoSalvo := time.Now()
		for {
			linha, errLeitura := leitor.ReadBytes('\n') // sem teto de tamanho de linha
			if len(linha) > 0 {
				_, _ = saida.Write(linha)
				ia.Ler(linha, &parcial)
				if time.Since(ultimoSalvo) > time.Second {
					t.Passos, t.Sessao, t.CustoUSD = parcial.Passos, parcial.Sessao, parcial.CustoUSD
					_ = salvar(t)
					ultimoSalvo = time.Now()
				}
			}
			if errLeitura != nil {
				if errLeitura != io.EOF {
					fmt.Fprintln(arqErro, "leitura:", errLeitura)
				}
				break
			}
		}
		if err := cmd.Wait(); err != nil {
			var sai *exec.ExitError
			if errors.As(err, &sai) {
				codigoSaida = sai.ExitCode()
			} else {
				codigoSaida = -1
			}
		}
	}()

	motivo := ""
	encerrar := func(m string) {
		motivo = m
		so.EncerrarGrupo(cmd.Process.Pid)
		select {
		case <-fim:
		case <-time.After(5 * time.Second):
			so.MatarGrupo(cmd.Process.Pid)
			<-fim
		}
	}
	select {
	case <-fim:
	case <-sinais:
		encerrar(Cancelada)
	case <-time.After(limite):
		encerrar(TempoEsgotado)
	}

	// A partir daqui só este fluxo mexe na tarefa.
	t.Passos, t.Sessao, t.CustoUSD = parcial.Passos, parcial.Sessao, parcial.CustoUSD
	t.Resposta = strings.TrimSpace(parcial.Texto)
	if b, err := os.ReadFile(arqFinal); err == nil && strings.TrimSpace(string(b)) != "" {
		t.Resposta = strings.TrimSpace(string(b))
	}
	t.Terminada = time.Now()
	switch {
	case motivo == Cancelada:
		t.Estado = Cancelada
	case motivo == TempoEsgotado:
		t.Estado = TempoEsgotado
		t.Erro = fmt.Sprintf("passou do tempo limite de %d minutos e foi encerrada", cfg.TempoLimiteMin)
	case parcial.Bloqueada != "":
		t.Estado, t.Erro = Bloqueada, parcial.Bloqueada
	case parcial.Erro != "":
		t.Estado, t.Erro = Erro, parcial.Erro
	case codigoSaida != 0:
		t.Estado = Erro
		t.Erro = fmt.Sprintf("%s saiu com código %d. %s", ia.Descricao, codigoSaida, cauda(filepath.Join(dir, "erro.log"), 600))
	case t.Resposta == "":
		t.Estado = Erro
		t.Erro = "a IA terminou sem devolver resposta. " + cauda(filepath.Join(dir, "erro.log"), 600)
	default:
		t.Estado = Terminou
	}
	return salvar(t)
}

func cauda(arquivo string, n int) string {
	b, err := os.ReadFile(arquivo)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		s = "…" + s[len(s)-n:]
	}
	return s
}

// Cancelar pede ao vigia que encerre a IA; se ele não responder, mata à força.
func Cancelar(id string) (*Tarefa, error) {
	t, err := Ler(id)
	if err != nil {
		return nil, err
	}
	if !t.Ativa() {
		return t, nil
	}
	voltas := 50 // até 10 s para o vigia encerrar com calma
	if t.PID <= 0 || !so.Encerrar(t.PID) {
		voltas = 0 // sem pedido educado neste sistema: vai direto ao forçado
	}
	for i := 0; i < voltas; i++ {
		time.Sleep(200 * time.Millisecond)
		if t, err = Ler(id); err != nil || !t.Ativa() {
			return t, err
		}
	}
	if t.PIDFilho > 0 {
		so.MatarGrupo(t.PIDFilho)
	}
	if t.PID > 0 {
		so.Matar(t.PID)
	}
	t.Estado, t.Terminada, t.Erro = Cancelada, time.Now(), "encerrada à força"
	return t, salvar(t)
}

// PararTudo cancela todas as tarefas que estão rodando.
func PararTudo() []*Tarefa {
	var out []*Tarefa
	for _, t := range Listar(0) {
		if t.Ativa() {
			if c, err := Cancelar(t.ID); err == nil {
				out = append(out, c)
			}
		}
	}
	return out
}
