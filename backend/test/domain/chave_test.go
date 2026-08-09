package domain_test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"stacktrack/internal/domain/ordem"
)

// entre é o atalho que falha o teste em vez de devolver erro — a chave textual
// só erra por entrada inválida, e nenhum caso aqui informa entrada inválida.
func entre(t *testing.T, anterior, proximo string) string {
	t.Helper()
	k, err := ordem.ChaveEntre(anterior, proximo)
	if err != nil {
		t.Fatalf("ChaveEntre(%q, %q): %v", anterior, proximo, err)
	}
	return k
}

// A chave da lista vazia nasce PERTO do meio do alfabeto, e não em "a": é isso
// que preserva margem para quem for inserido antes do primeiro item.
//
// "Perto", e não exatamente no meio, porque há um sorteio — ver o teste da
// colisão mais abaixo.
func TestChaveDaListaVaziaNasceLongeDasPontas(t *testing.T) {
	for i := 0; i < 50; i++ {
		k := entre(t, "", "")
		if len(k) != 1 {
			t.Fatalf("chave = %q, esperado um caractere só", k)
		}
		if k <= "e" || k >= "u" {
			t.Errorf("chave = %q, esperado perto do meio do alfabeto", k)
		}
	}
}

// Entre duas letras vizinhas do alfabeto não cabe outra letra — cabe uma chave
// mais longa. O plano ilustrava com "entre a e b cabe an"; "a" não é chave
// válida aqui (termina no menor caractere), então o exemplo equivalente é
// entre "b" e "c".
//
// A afirmação é de PROPRIEDADE, e não de valor exato: com o sorteio que evita
// colisão, o caractere final varia. Exigir "bn" travaria o teste num detalhe de
// implementação em vez de na regra — e a regra é "fica entre os dois".
func TestEntreDuasLetrasVizinhasCabeUmaChaveMaior(t *testing.T) {
	k := entre(t, "b", "c")

	if !(k > "b" && k < "c") {
		t.Fatalf("chave = %q, esperado entre 'b' e 'c'", k)
	}
	if len(k) != 2 || k[0] != 'b' {
		t.Errorf("chave = %q, esperado estender 'b' em um caractere", k)
	}
}

// A PROPRIEDADE QUE DEFINE A FASE 9, e o contraste direto com o float:
// reordenar sempre no mesmo ponto continua funcionando por centenas de vezes.
// Com double precision isto falhava em algumas dezenas de tentativas — o teto
// medido era 52, e ele era alcançável.
//
// Estes testes já afirmaram "mil vezes nunca esgota", e isso era FALSO: só
// passava porque o domínio ignorava o tamanho da coluna. Contra o VARCHAR(200)
// de verdade, os quatro padrões esgotam perto de 750. O número honesto é esse,
// e ele está aqui embaixo em vez de numa promessa que o banco não sustenta.
//
// Quando o teto chega, o erro tem de ser o PREVISTO (ErrChaveLonga, que vira
// 409) — nunca uma chave fora de ordem, que corromperia o quadro em silêncio.
func TestReordenarNoMesmoPontoAguentaCentenasDeVezes(t *testing.T) {
	// O piso que se exige de cada padrão. Não é o teto medido: cravar o número
	// exato faria o teste quebrar a cada ajuste no sorteio, que é aleatório de
	// propósito. É o "muito mais que o float aguentava" que importa.
	const piso = 600

	casos := []struct {
		nome string
		// passo devolve o próximo par de vizinhos a partir da chave gerada.
		passo func(k string, anterior, proximo string) (string, string)
	}{
		{"apertando o intervalo pela direita", func(k, a, p string) (string, string) { return a, k }},
		{"sempre logo depois do mesmo item", func(k, a, p string) (string, string) { return k, p }},
		{"sempre no topo da lista", func(k, a, p string) (string, string) { return "", k }},
		{"sempre no fim da lista", func(k, a, p string) (string, string) { return k, "" }},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			anterior, proximo := "b", "c"
			if caso.nome == "sempre no topo da lista" {
				anterior, proximo = "", ordem.ChaveInicial
			}
			if caso.nome == "sempre no fim da lista" {
				anterior, proximo = ordem.ChaveInicial, ""
			}

			for i := 1; i <= 2000; i++ {
				k, err := ordem.ChaveEntre(anterior, proximo)
				if errors.Is(err, ordem.ErrChaveLonga) {
					if i <= piso {
						t.Fatalf("esgotou na inserção %d, antes do piso de %d", i, piso)
					}
					return // esgotou onde é aceitável, e com o erro certo
				}
				if err != nil {
					t.Fatalf("na inserção %d, erro inesperado: %v", i, err)
				}
				if anterior != "" && k <= anterior {
					t.Fatalf("na inserção %d: %q não vem depois de %q", i, k, anterior)
				}
				if proximo != "" && k >= proximo {
					t.Fatalf("na inserção %d: %q não vem antes de %q", i, k, proximo)
				}
				anterior, proximo = caso.passo(k, anterior, proximo)
			}
		})
	}
}

// O desperdício que custava caro: entre "bq" e "c" cabem "br".."bz", e estender
// para três caracteres gastaria uma letra POR inserção. Era o que fazia o padrão
// "sempre logo depois" esgotar em 199 em vez de perto de 750.
func TestNaoEstendeAChaveQuandoAindaCabeNoComprimentoAtual(t *testing.T) {
	k := entre(t, "bq", "c")

	if len(k) != 2 {
		t.Errorf("chave = %q (%d caracteres), esperado 2 — havia folga em \"br\"..\"bz\"", k, len(k))
	}
	if k <= "bq" || k >= "c" {
		t.Errorf("chave = %q, fora do intervalo", k)
	}
}

// A invariante que sustenta tudo: nenhuma chave gerada termina no menor
// caractere. É ela que garante que sempre cabe alguém ANTES de qualquer item —
// sem ela, uma chave "a" seria um beco sem saída.
func TestNenhumaChaveGeradaTerminaNoMenorCaractere(t *testing.T) {
	chaves := []string{entre(t, "", "")}

	// Gera um monte de chaves por todos os caminhos: no topo, no fim e no meio.
	for i := 0; i < 200; i++ {
		primeira := chaves[0]
		ultima := chaves[len(chaves)-1]
		chaves = append([]string{entre(t, "", primeira)}, chaves...)
		chaves = append(chaves, entre(t, ultima, ""))
		if len(chaves) >= 2 {
			chaves = append(chaves, entre(t, chaves[0], chaves[1]))
		}
	}

	for _, k := range chaves {
		if k == "" {
			t.Fatal("chave vazia gerada")
		}
		if k[len(k)-1] == 'a' {
			t.Errorf("a chave %q termina no menor caractere — quebra a invariante", k)
		}
	}
}

// A prova de que o esquema serve para ORDENAR: uma sequência de inserções em
// posições arbitrárias, lida de volta em ordem alfabética, devolve exatamente a
// ordem em que os itens foram colocados.
func TestAOrdemAlfabeticaReproduzAOrdemDasInsercoes(t *testing.T) {
	// Começa com três itens.
	lista := []string{entre(t, "", "")}
	lista = append(lista, entre(t, lista[0], ""))
	lista = append([]string{entre(t, "", lista[0])}, lista...)

	// Insere no meio, repetidamente, em pontos variados.
	for i := 0; i < 100; i++ {
		pos := (i * 7) % (len(lista) - 1)
		nova := entre(t, lista[pos], lista[pos+1])

		resto := append([]string{}, lista[pos+1:]...)
		lista = append(append(lista[:pos+1], nova), resto...)
	}

	ordenada := append([]string{}, lista...)
	sort.Strings(ordenada)

	for i := range lista {
		if lista[i] != ordenada[i] {
			t.Fatalf("na posição %d a ordem alfabética discorda da ordem de inserção: %q != %q",
				i, ordenada[i], lista[i])
		}
	}
}

func TestVizinhosForaDeOrdemSaoRecusados(t *testing.T) {
	if _, err := ordem.ChaveEntre("n", "b"); !errors.Is(err, ordem.ErrForaDeOrdem) {
		t.Errorf("invertidos: erro = %v", err)
	}
	if _, err := ordem.ChaveEntre("n", "n"); !errors.Is(err, ordem.ErrForaDeOrdem) {
		t.Errorf("iguais: erro = %v", err)
	}
}

func TestChaveInvalidaERecusada(t *testing.T) {
	invalidas := []string{"A", "n1", "n ", "ná", "na"}
	for _, k := range invalidas {
		if _, err := ordem.ChaveEntre(k, ""); !errors.Is(err, ordem.ErrChaveInvalida) {
			t.Errorf("ChaveEntre(%q, \"\") aceitou: %v", k, err)
		}
	}
}

func TestNormalizarChave(t *testing.T) {
	if k, err := ordem.NormalizarChave("  ng  "); err != nil || k != "ng" {
		t.Errorf("normalizar = %q, %v", k, err)
	}
	if _, err := ordem.NormalizarChave("na"); !errors.Is(err, ordem.ErrChaveInvalida) {
		t.Errorf("chave terminada no menor caractere foi aceita: %v", err)
	}
}

// A chave cresce quando se insere sempre no mesmo ponto — é o preço do esquema,
// e vale medir para saber que ele é modesto.
func TestOCrescimentoDaChaveEModesto(t *testing.T) {
	anterior, proximo := "b", "c"
	for i := 0; i < 100; i++ {
		proximo = entre(t, anterior, proximo)
	}

	if len(proximo) > 110 {
		t.Errorf("depois de 100 inserções no mesmo ponto a chave tem %d caracteres", len(proximo))
	}
	t.Logf("100 inserções no mesmo ponto: chave de %d caracteres (%q)", len(proximo), proximo)
}

// A COLISÃO, que é o motivo de o sorteio existir.
//
// Duas pessoas arrastam um card para o mesmo lugar ao mesmo tempo: as duas
// enxergam os mesmos vizinhos e pedem a chave do mesmo intervalo. Sem sorteio,
// as duas recebiam a MESMA chave.
//
// O empate em si é tolerável — ninguém pediu uma ordem entre esses dois. O
// estrago vem depois: não existe chave ENTRE duas iguais, então arrastar
// qualquer coisa entre elas passava a falhar, que é justamente o tipo de
// impedimento que a fase 9 existe para remover.
func TestChavesDoMesmoIntervaloRaramenteColidem(t *testing.T) {
	const tentativas = 200
	vistas := map[string]int{}
	for i := 0; i < tentativas; i++ {
		vistas[entre(t, "b", "n")]++
	}

	if len(vistas) < 5 {
		t.Errorf("%d tentativas produziram só %d chaves distintas — o sorteio não está espalhando",
			tentativas, len(vistas))
	}
	t.Logf("%d tentativas -> %d chaves distintas", tentativas, len(vistas))
}

// E o sorteio não pode custar tamanho: ele acontece DENTRO da folga que já
// existia, então a chave não fica mais longa por causa dele.
func TestOSorteioNaoAlongaAChave(t *testing.T) {
	for i := 0; i < 100; i++ {
		if k := entre(t, "b", "n"); len(k) != 1 {
			t.Fatalf("chave = %q, esperado um caractere — havia folga de sobra", k)
		}
	}
}

// "Infinitamente" é verdade no papel; na coluna VARCHAR(200) não é.
//
// Quando a chave calculada não couber, o domínio precisa dizer isso — se ele se
// calar, quem reclama é o driver do Postgres, e um erro previsto vira 500.
func TestChaveQueNaoCabeNaColunaEhRecusadaPeloDominio(t *testing.T) {
	// Duas chaves vizinhas e já no teto: qualquer coisa entre elas teria de
	// estender, e estender passaria do limite.
	a := strings.Repeat("n", ordem.TamanhoMaximo)
	b := strings.Repeat("n", ordem.TamanhoMaximo-1) + "o"

	if _, err := ordem.ChaveEntre(a, b); !errors.Is(err, ordem.ErrChaveLonga) {
		t.Errorf("erro = %v, esperado ErrChaveLonga", err)
	}
}

// E o teto não pode cortar quem ainda cabe: no limite exato a chave passa.
func TestChaveNoLimiteExatoAindaPassa(t *testing.T) {
	// Entre "b" e "n" cabe um caractere só — bem longe do teto.
	k, err := ordem.ChaveEntre("b", "n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(k) > ordem.TamanhoMaximo {
		t.Errorf("chave de %d caracteres passou do teto de %d", len(k), ordem.TamanhoMaximo)
	}
}

// O teto do domínio e o da coluna são o mesmo número, e é o teste de
// integração (chave_cabe_na_coluna_test.go) que pergunta isso ao banco. Aqui
// fica só a âncora: se alguém mudar a constante, este teste lembra que existe
// uma migration do outro lado.
func TestOTetoEhOTamanhoDaColunaDaMigration(t *testing.T) {
	if ordem.TamanhoMaximo != 200 {
		t.Errorf("TamanhoMaximo = %d: mude também o VARCHAR de cards.chave e "+
			"colunas.chave, numa migration nova", ordem.TamanhoMaximo)
	}
}
