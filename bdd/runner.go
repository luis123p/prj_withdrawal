// Package bdd ejecuta los archivos .feature (Gherkin) contra el paquete banco.
//
// Refactorización (acápite 4): antes había una función Go por cada escenario
// (test_bdd.go, 11 funciones casi iguales). Ahora el runner lee los .feature y
// cada línea Given/When/Then se enlaza con un "step definition" (expresión
// regular + función), igual que hace Cucumber.
// Autor: Paucar Ventura, Luis Antonio
package bdd

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

// Escenario ya expandido (un Scenario Outline genera uno por fila de Examples).
type Escenario struct {
	Feature, Rule, Nombre string
	Archivo               string
	Pasos                 []string // "Given Alice has 234.56 in their account"
}

// ResultadoPaso / ResultadoEscenario sirven para mostrar el reporte (CLI o web).
type ResultadoPaso struct {
	Texto  string `json:"texto"`
	Estado string `json:"estado"` // OK, FALLA, PENDIENTE, OMITIDO
	Error  string `json:"error,omitempty"`
}

type ResultadoEscenario struct {
	Feature string          `json:"feature"`
	Nombre  string          `json:"nombre"`
	Paso    bool            `json:"paso"`
	Pasos   []ResultadoPaso `json:"pasos"`
}

type Reporte struct {
	Escenarios []ResultadoEscenario `json:"escenarios"`
	Total      int                  `json:"total"`
	Pasaron    int                  `json:"pasaron"`
	PasosTotal int                  `json:"pasos_total"`
	PasosOK    int                  `json:"pasos_ok"`
}

var reKeyword = regexp.MustCompile(`^(Feature|Rule|Scenario Outline|Scenario|Examples):\s*(.*)$`)
var reStep = regexp.MustCompile(`^(Given|When|Then|And|But)\s+(.*)$`)

// Parsear lee un .feature y devuelve sus escenarios (con los Outline expandidos).
func Parsear(nombreArchivo string, r io.Reader) ([]Escenario, error) {
	var (
		out           []Escenario
		feature, rule string
		actual        *Escenario
		esOutline     bool
		enExamples    bool
		cabecera      []string
		plantilla     Escenario
	)
	cerrar := func() {
		if actual != nil && !esOutline {
			out = append(out, *actual)
		}
		actual, esOutline, enExamples, cabecera = nil, false, false, nil
	}
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		ln := strings.TrimSpace(sc.Text())
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		if m := reKeyword.FindStringSubmatch(ln); m != nil {
			switch m[1] {
			case "Feature":
				cerrar()
				feature = m[2]
			case "Rule":
				cerrar()
				rule = m[2]
			case "Scenario", "Scenario Outline":
				cerrar()
				actual = &Escenario{Feature: feature, Rule: rule, Nombre: m[2], Archivo: nombreArchivo}
				esOutline = m[1] == "Scenario Outline"
			case "Examples":
				if !esOutline {
					return nil, fmt.Errorf("%s:%d: Examples fuera de un Scenario Outline", nombreArchivo, n)
				}
				enExamples, plantilla = true, *actual
			}
			continue
		}
		if m := reStep.FindStringSubmatch(ln); m != nil && actual != nil && !enExamples {
			actual.Pasos = append(actual.Pasos, m[1]+" "+m[2])
			continue
		}
		if strings.HasPrefix(ln, "|") && enExamples {
			celdas := celdasTabla(ln)
			if cabecera == nil {
				cabecera = celdas
				continue
			}
			if len(celdas) != len(cabecera) {
				return nil, fmt.Errorf("%s:%d: la fila de Examples no coincide con la cabecera", nombreArchivo, n)
			}
			e := plantilla
			e.Pasos = nil
			var vals []string
			for _, p := range plantilla.Pasos {
				for i, col := range cabecera {
					p = strings.ReplaceAll(p, "<"+col+">", celdas[i])
				}
				e.Pasos = append(e.Pasos, p)
			}
			for i, col := range cabecera {
				vals = append(vals, col+"="+celdas[i])
			}
			e.Nombre = fmt.Sprintf("%s (%s)", plantilla.Nombre, strings.Join(vals, ", "))
			out = append(out, e)
			continue
		}
		// Cualquier otra línea (descripción "As a / I want / So that") se ignora.
	}
	cerrar()
	return out, sc.Err()
}

func celdasTabla(ln string) []string {
	partes := strings.Split(strings.Trim(ln, "|"), "|")
	for i := range partes {
		partes[i] = strings.TrimSpace(partes[i])
	}
	return partes
}

// Ejecutar corre todos los .feature de fsys e imprime el resultado en w (si no es nil).
func Ejecutar(fsys fs.FS, w io.Writer) (Reporte, error) {
	archivos, err := fs.Glob(fsys, "*.feature")
	if err != nil {
		return Reporte{}, err
	}
	sort.Strings(archivos)
	var todos []Escenario
	for _, a := range archivos {
		f, err := fsys.Open(a)
		if err != nil {
			return Reporte{}, err
		}
		esc, err := Parsear(a, f)
		f.Close()
		if err != nil {
			return Reporte{}, err
		}
		todos = append(todos, esc...)
	}
	return EjecutarEscenarios(todos, w), nil
}

// EjecutarEscenarios corre una lista de escenarios ya parseados.
func EjecutarEscenarios(esc []Escenario, w io.Writer) Reporte {
	if w == nil {
		w = io.Discard
	}
	var rep Reporte
	featureActual := ""
	for _, e := range esc {
		if e.Feature != featureActual {
			featureActual = e.Feature
			fmt.Fprintf(w, "\nFeature: %s\n", e.Feature)
		}
		fmt.Fprintf(w, "  Scenario: %s\n", e.Nombre)
		r := correr(e)
		for _, p := range r.Pasos {
			marca := map[string]string{"OK": "[OK]   ", "FALLA": "[FALLA]", "PENDIENTE": "[??]   ", "OMITIDO": "[--]   "}[p.Estado]
			fmt.Fprintf(w, "    %s %s\n", marca, p.Texto)
			if p.Error != "" {
				fmt.Fprintf(w, "            -> %s\n", p.Error)
			}
			rep.PasosTotal++
			if p.Estado == "OK" {
				rep.PasosOK++
			}
		}
		rep.Total++
		if r.Paso {
			rep.Pasaron++
		}
		rep.Escenarios = append(rep.Escenarios, r)
	}
	fmt.Fprintf(w, "\n%d escenarios (%d pasaron, %d fallaron)\n%d pasos (%d OK)\n",
		rep.Total, rep.Pasaron, rep.Total-rep.Pasaron, rep.PasosTotal, rep.PasosOK)
	return rep
}

func correr(e Escenario) ResultadoEscenario {
	r := ResultadoEscenario{Feature: e.Feature, Nombre: e.Nombre, Paso: true}
	ctx := &contexto{}
	fallo := false
	for _, p := range e.Pasos {
		if fallo {
			r.Pasos = append(r.Pasos, ResultadoPaso{Texto: p, Estado: "OMITIDO"})
			continue
		}
		texto := p[strings.Index(p, " ")+1:]
		def, args := buscarPaso(texto)
		if def == nil {
			r.Pasos = append(r.Pasos, ResultadoPaso{Texto: p, Estado: "PENDIENTE", Error: "paso sin definir"})
			fallo, r.Paso = true, false
			continue
		}
		if err := def.fn(ctx, args); err != nil {
			r.Pasos = append(r.Pasos, ResultadoPaso{Texto: p, Estado: "FALLA", Error: err.Error()})
			fallo, r.Paso = true, false
			continue
		}
		r.Pasos = append(r.Pasos, ResultadoPaso{Texto: p, Estado: "OK"})
	}
	return r
}
