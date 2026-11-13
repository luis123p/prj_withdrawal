// Corre los .feature del proyecto con "go test ./bdd".
// Autor: Paucar Ventura, Luis Antonio
package bdd

import (
	"os"
	"strings"
	"testing"
)

func TestFeatures(t *testing.T) {
	var sb strings.Builder
	rep, err := Ejecutar(os.DirFS("../features"), &sb)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(sb.String())
	if rep.Total < 13 || rep.Pasaron != rep.Total {
		t.Fatalf("%d de %d escenarios pasaron", rep.Pasaron, rep.Total)
	}
}

// El runner tiene que detectar un paso que falla y uno sin definir
// (si no, una suite "todo verde" no probaría nada).
func TestRunnerDetectaFallas(t *testing.T) {
	src := `Feature: Prueba
  Scenario: saldo mal escrito a propósito
    Given Ana has 100.00 in their account
    When Ana tries to withdraw 30.00
    Then the balance is 99.00
    And the withdrawal is successful
  Scenario: paso sin definir
    Given Ana has 100.00 in their account
    When Ana pays the electricity bill
`
	esc, err := Parsear("prueba.feature", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	rep := EjecutarEscenarios(esc, nil)
	if rep.Pasaron != 0 || rep.Total != 2 {
		t.Fatalf("esperado 0 de 2, obtenido %d de %d", rep.Pasaron, rep.Total)
	}
	p := rep.Escenarios[0].Pasos
	if p[2].Estado != "FALLA" || p[3].Estado != "OMITIDO" {
		t.Fatalf("estados inesperados: %+v", p)
	}
	if rep.Escenarios[1].Pasos[1].Estado != "PENDIENTE" {
		t.Fatalf("se esperaba PENDIENTE: %+v", rep.Escenarios[1].Pasos)
	}
}

func TestOutlineSeExpande(t *testing.T) {
	src := `Feature: X
  Scenario Outline: abonos
    Given Ana has 10.00 in their account
    When Ana deposits <m>
    Then the balance is <s>
    Examples:
      | m    | s     |
      | 1.00 | 11.00 |
      | 2.50 | 12.50 |
`
	esc, _ := Parsear("x.feature", strings.NewReader(src))
	if len(esc) != 2 || esc[1].Pasos[1] != "When Ana deposits 2.50" {
		t.Fatalf("outline mal expandido: %+v", esc)
	}
	if rep := EjecutarEscenarios(esc, nil); rep.Pasaron != 2 {
		t.Fatalf("pasaron %d de 2", rep.Pasaron)
	}
}
