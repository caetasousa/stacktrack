package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"stacktrack/internal/adapter/http/handler"
	"stacktrack/internal/domain/usuario"
	ucauth "stacktrack/internal/usecase/auth"
	"stacktrack/test/repository/memoria"
)

type hasherDeRajada struct {
	chamadas atomic.Int64
	liberar  <-chan struct{}
	erro     error
}

func (h *hasherDeRajada) Gerar(string) (string, error) { return "hash", nil }
func (h *hasherDeRajada) Verificar(string, string) (bool, error) {
	h.chamadas.Add(1)
	if h.liberar != nil {
		<-h.liberar
	}
	return false, h.erro
}

func handlerDeLoginCom(t *testing.T, hasher *hasherDeRajada, teto int) *handler.AuthHandler {
	t.Helper()
	usuarios := memoria.NovosUsuarios()
	u, err := usuario.Novo(uuid.NewString(), "Ana", "ana@exemplo.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := usuarios.Salvar(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	login := ucauth.NovoLoginUseCase(usuarios, memoria.NovasSessoes(), hasher)
	return handler.NovoAuthHandler(nil, login, nil, nil, false,
		handler.NovoLimitadorPorConta(teto, time.Minute), nil)
}

func pedirLogin(h *handler.AuthHandler) int {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(
		`{"email":"ana@exemplo.com","senha":"senha-errada-longa"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	return rec.Code
}

func TestRajadaDeLoginNaoExecutaMaisHashesQueOTeto(t *testing.T) {
	const teto = 3
	liberar := make(chan struct{})
	hasher := &hasherDeRajada{liberar: liberar}
	h := handlerDeLoginCom(t, hasher, teto)
	inicio := make(chan struct{})
	status := make(chan int, 50)
	var wg sync.WaitGroup

	for i := 0; i < cap(status); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			status <- pedirLogin(h)
		}()
	}
	close(inicio)

	limite := time.After(time.Second)
	for hasher.chamadas.Load() < teto {
		select {
		case <-limite:
			t.Fatal("as reservas permitidas nao chegaram ao hasher")
		case <-time.After(time.Millisecond):
		}
	}
	close(liberar)
	wg.Wait()
	close(status)

	var recusadas, invalidas int
	for codigo := range status {
		switch codigo {
		case http.StatusTooManyRequests:
			recusadas++
		case http.StatusUnauthorized:
			invalidas++
		default:
			t.Errorf("status inesperado: %d", codigo)
		}
	}
	if got := hasher.chamadas.Load(); got != teto {
		t.Errorf("hashes = %d, esperado %d", got, teto)
	}
	if invalidas != teto || recusadas != cap(status)-teto {
		t.Errorf("401=%d 429=%d, esperado %d e %d", invalidas, recusadas, teto, cap(status)-teto)
	}
}

func TestErroDoHasherNaoTrancaAConta(t *testing.T) {
	hasher := &hasherDeRajada{erro: errors.New("argon indisponivel")}
	h := handlerDeLoginCom(t, hasher, 1)
	for i := 0; i < 5; i++ {
		if codigo := pedirLogin(h); codigo != http.StatusInternalServerError {
			t.Fatalf("tentativa %d: status = %d, esperado 500", i, codigo)
		}
	}
	if got := hasher.chamadas.Load(); got != 5 {
		t.Errorf("hashes = %d, esperado 5: falha de infraestrutura consumiu cota", got)
	}
}
