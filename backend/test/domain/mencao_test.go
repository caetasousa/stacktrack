package domain_test

// Quem um comentário menciona — o que decide quem recebe a notificação.
//
// Função pura, cujo resultado o usuário vê (o sino de quem foi chamado). Como
// isto quebra:
//
//	M1 o nome completo de quem participa não é reconhecido, ou só é
//	   reconhecido com as maiúsculas exatas;
//	M2 o primeiro nome, quando só uma pessoa do quadro o tem, não chama
//	   ninguém — e é o que todo mundo digita;
//	M3 o primeiro nome repetido chama a pessoa errada;
//	M4 um email no meio do texto ("bruno@exemplo.com") vira menção;
//	M5 "@Ana Silva" chama a "Ana" quando as duas participam, ou "@Brunox"
//	   chama o Bruno;
//	M6 a mesma pessoa chamada duas vezes recebe duas notificações;
//	M7 nome com acento não é reconhecido.

import (
	"reflect"
	"testing"

	"stacktrack/internal/domain/comentario"
)

var pessoasDoQuadro = []comentario.Mencionavel{
	{ID: "bruno", Nome: "Bruno Costa"},
	{ID: "carla", Nome: "Carla Dias"},
	{ID: "ana-silva", Nome: "Ana Silva"},
	{ID: "ana-souza", Nome: "Ana Souza"},
	{ID: "jose", Nome: "José Álvaro"},
}

func TestMencoes(t *testing.T) {
	casos := []struct {
		nome   string
		texto  string
		espera []string
	}{
		{"M1 nome completo", "@Bruno Costa, pode revisar?", []string{"bruno"}},
		{"M1 sem caixa", "@bruno costa pode revisar?", []string{"bruno"}},
		{"M1 pontuação depois", "Valeu, @Carla Dias!", []string{"carla"}},
		{"M1 entre parênteses e em negrito", "(@Carla Dias) e **@Bruno Costa**", []string{"carla", "bruno"}},
		{"M2 primeiro nome único", "@Bruno, consegue hoje?", []string{"bruno"}},
		{"M3 primeiro nome repetido não chama ninguém", "@Ana, consegue hoje?", nil},
		{"M3 o nome completo desfaz a dúvida", "@Ana Souza, consegue hoje?", []string{"ana-souza"}},
		{"M4 email não é menção", "mande para bruno@exemplo.com", nil},
		{"M5 letra colada ao nome", "@Brunox e @Carlaz", nil},
		{"M5 sobrenome errado ainda vale pelo primeiro nome", "@Carla Diaz", []string{"carla"}},
		{"M6 sem repetição, na ordem do texto", "@Carla Dias e @Bruno, de novo @carla dias", []string{"carla", "bruno"}},
		{"M7 acento", "@josé álvaro, olha isto", []string{"jose"}},
		{"arroba sozinha", "@ e @@ e @", nil},
		{"espaços a mais entre os nomes", "@Bruno   Costa", []string{"bruno"}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := comentario.Mencoes(c.texto, pessoasDoQuadro); !reflect.DeepEqual(got, c.espera) {
				t.Errorf("Mencoes(%q) = %v, esperado %v", c.texto, got, c.espera)
			}
		})
	}
}

// M5: quando um nome é o começo de outro, vale o mais longo que casar inteiro.
func TestMencaoPrefereONomeMaisLongoQueCasaInteiro(t *testing.T) {
	pessoas := []comentario.Mencionavel{{ID: "ana", Nome: "Ana"}, {ID: "ana-silva", Nome: "Ana Silva"}}
	if got := comentario.Mencoes("@Ana Silva, veja", pessoas); !reflect.DeepEqual(got, []string{"ana-silva"}) {
		t.Errorf("@Ana Silva = %v, esperado só ana-silva", got)
	}
	if got := comentario.Mencoes("@Ana Silvana, veja", pessoas); !reflect.DeepEqual(got, []string{"ana"}) {
		t.Errorf("@Ana Silvana = %v, esperado só ana", got)
	}
}

// Duas pessoas com o MESMO nome completo: não há o que digitar a mais para
// distinguir, então as duas são chamadas. As duas participam do quadro e leem
// o comentário de qualquer jeito — avisar a mais não expõe nada.
func TestNomeCompletoRepetidoChamaOsDois(t *testing.T) {
	pessoas := []comentario.Mencionavel{{ID: "a1", Nome: "Ana Silva"}, {ID: "a2", Nome: "Ana Silva"}}
	if got := comentario.Mencoes("@Ana Silva", pessoas); !reflect.DeepEqual(got, []string{"a1", "a2"}) {
		t.Errorf("Mencoes = %v, esperado as duas", got)
	}
}
