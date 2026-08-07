package domain_test

import (
	"testing"
	"time"

	"stacktrack/internal/domain/session"
)

func TestNovaSessaoExpiraDepoisDoTTL(t *testing.T) {
	s := session.Nova("hash", "usuario-1", time.Hour)

	if s.Expirada(s.CriadoEm.Add(59 * time.Minute)) {
		t.Error("sessão não podia estar expirada antes do TTL")
	}
	if !s.Expirada(s.CriadoEm.Add(61 * time.Minute)) {
		t.Error("sessão devia estar expirada depois do TTL")
	}
}

// O instante exato do vencimento ainda vale: Expirada usa After, então
// expira_em é o último momento válido, e não o primeiro inválido.
func TestSessaoNoInstanteDoVencimentoAindaVale(t *testing.T) {
	s := session.Nova("hash", "usuario-1", time.Hour)

	if s.Expirada(s.ExpiraEm) {
		t.Error("no instante exato de expira_em a sessão ainda deve valer")
	}
}

func TestNovaSessaoGuardaUsuarioEHash(t *testing.T) {
	s := session.Nova("hash-do-token", "usuario-1", time.Hour)

	if s.TokenHash != "hash-do-token" {
		t.Errorf("TokenHash = %q", s.TokenHash)
	}
	if s.UsuarioID != "usuario-1" {
		t.Errorf("UsuarioID = %q", s.UsuarioID)
	}
}
