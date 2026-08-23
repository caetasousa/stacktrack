package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestErroInternoDeDeadlineVira503ComRetryAfter(t *testing.T) {
	ctx, cancelar := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelar()
	req := httptest.NewRequest(http.MethodGet, "/boards", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	responderErroInterno(rec, req, "consulta falhou", errors.New("erro embrulhado"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, esperado 503", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("503 de deadline sem Retry-After")
	}
}
