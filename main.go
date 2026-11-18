/*
Source file: main.go [prj_withdrawal v2.0.0 - App Web + CLI]
Iteración 4 (acápite 4): refactorización a App Web.

	go run .                  -> levanta la app web en http://localhost:8080
	go run . -bdd             -> ejecuta los features Gherkin en la terminal
	go run . -i               -> cajero interactivo en la terminal (CLI)
	go run . -op retiro ...   -> una operación desde la línea de comandos
	go run . -historial       -> muestra el log de transacciones

Base: codeLAB-02 de J.Medianero.A (v1.2.0).
Autor (iteraciones 2, 3 y 4): Paucar Ventura, Luis Antonio
*/
package main

import (
	"bufio"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"prj_withdrawal/banco"
	"prj_withdrawal/bdd"
	"prj_withdrawal/web"
)

//go:embed features/*.feature
var featuresFS embed.FS

func main() {
	puerto := flag.String("puerto", "8080", "Puerto de la app web")
	correrBDD := flag.Bool("bdd", false, "Ejecuta los escenarios Gherkin (features/*.feature)")
	interactivo := flag.Bool("i", false, "Cajero interactivo en la terminal")
	op := flag.String("op", "", "Operación: 'retiro' o 'abono' (con -nombre y -monto)")
	nombre := flag.String("nombre", "", "Titular de la cuenta")
	monto := flag.Float64("monto", 0, "Monto para -op")
	historial := flag.Bool("historial", false, "Muestra el historial (filtra con -nombre)")
	rutaLog := flag.String("log", "data/transacciones.json", "Archivo JSON del historial")
	rutaDatos := flag.String("datos", "", "Archivo JSON de cuentas (vacío = JSON incrustado)")
	flag.Parse()

	features, _ := fs.Sub(featuresFS, "features")

	if *correrBDD {
		rep, err := bdd.Ejecutar(features, os.Stdout)
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		if rep.Pasaron != rep.Total {
			os.Exit(1)
		}
		return
	}

	datos, err := banco.CargarCuentas(*rutaDatos)
	if err != nil {
		log.Fatalf("No se pudieron leer las cuentas: %v", err)
	}
	b, err := banco.NuevoBanco(datos, banco.NewBitacoraJSON(*rutaLog))
	if err != nil {
		log.Fatalf("No se pudo abrir el log: %v", err)
	}

	switch {
	case *historial:
		imprimirHistorial(os.Stdout, b, *nombre)
	case *op != "":
		os.Exit(operacion(os.Stdout, b, *op, *nombre, *monto))
	case *interactivo:
		cajeroCLI(b, *nombre, os.Stdin, os.Stdout)
	default:
		dir := "localhost:" + *puerto
		fmt.Println("==============================================")
		fmt.Println("  Cajero Web - prj_withdrawal v2.0")
		fmt.Printf("  Abrir en el navegador:  http://%s\n", dir)
		fmt.Printf("  Log de transacciones:   %s\n", *rutaLog)
		fmt.Println("  (Ctrl+C para detener)")
		fmt.Println("==============================================")
		log.Fatal(http.ListenAndServe(":"+*puerto, web.Nuevo(b, features)))
	}
}

func mostrar(w io.Writer, t banco.Transaccion, err error) {
	switch {
	case err != nil && t.ID == "":
		fmt.Fprintln(w, "ERROR:", err)
	case err != nil:
		fmt.Fprintf(w, "[%s] ERROR: %v\n", t.ID, err)
	case t.Estado == banco.EstadoExitosa:
		fmt.Fprintf(w, "[%s] %s exitoso. Nuevo saldo: %s\n", t.ID, strings.ToLower(t.Tipo), t.SaldoFinal)
	default:
		fmt.Fprintf(w, "[%s] %s %s: %s. Saldo: %s\n", t.ID, t.Tipo, t.Estado, t.Mensaje, t.SaldoFinal)
	}
}

func operacion(w io.Writer, b *banco.Banco, op, nombre string, monto float64) int {
	tipo := map[string]string{"retiro": banco.TipoRetiro, "abono": banco.TipoAbono}[op]
	if tipo == "" {
		fmt.Fprintln(w, "Error: -op debe ser 'retiro' o 'abono'")
		return 1
	}
	t, err := b.Operar(banco.NormalizarTitular(nombre), tipo, monto)
	mostrar(w, t, err)
	if err != nil {
		return 1
	}
	return 0
}

// Formato de la tabla del historial: el mismo para la cabecera y las filas.
// TIPO tiene 8 de ancho (APERTURA) y TITULAR 12; los nombres más largos se recortan.
const (
	formatoHistorial = "%-8s %-19s %-12s %-8s %12s %12s %12s  %-9s %s\n"
	anchoTitular     = 12
)

// recortar deja s en n caracteres como máximo (contando runas, no bytes).
func recortar(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func imprimirHistorial(w io.Writer, b *banco.Banco, titular string) {
	h, err := b.Historial(titular)
	if err != nil {
		fmt.Fprintln(w, "Error:", err)
		return
	}
	fmt.Fprintf(w, formatoHistorial,
		"ID", "FECHA", "TITULAR", "TIPO", "MONTO", "S.ANTES", "S.DESPUES", "ESTADO", "MENSAJE")
	for i := len(h) - 1; i >= 0; i-- { // en orden cronológico
		t := h[i]
		fmt.Fprintf(w, formatoHistorial, t.ID, t.Fecha, recortar(t.Titular, anchoTitular), t.Tipo,
			t.Monto, t.SaldoAnterior, t.SaldoFinal, t.Estado, t.Mensaje)
	}
	fmt.Fprintf(w, "(%d transacciones)\n", len(h))
}

// cajeroCLI es el cajero interactivo (-i). Solo la opción 5 cierra la sesión;
// si la entrada se termina (EOF, por ejemplo Ctrl+Z / Ctrl+D) también sale.
func cajeroCLI(b *banco.Banco, nombre string, entrada io.Reader, w io.Writer) {
	in := bufio.NewReader(entrada)
	// leer devuelve false cuando ya no hay nada más que leer.
	leer := func(p string) (string, bool) {
		fmt.Fprint(w, p)
		s, err := in.ReadString('\n')
		return strings.TrimSpace(s), err == nil || s != ""
	}
	fin := func() { fmt.Fprintln(w, "\nSesión finalizada.") }

	nombre = banco.NormalizarTitular(nombre)
	if nombre == "" {
		var ok bool
		if nombre, ok = leer("Nombre del titular: "); !ok {
			fin()
			return
		}
		nombre = banco.NormalizarTitular(nombre)
	}
	if _, err := b.Saldo(nombre); err != nil {
		txt, ok := leer("Cuenta nueva. Saldo inicial: ")
		if !ok {
			fin()
			return
		}
		s, err := strconv.ParseFloat(txt, 64)
		if err != nil {
			fmt.Fprintln(w, "No se pudo crear la cuenta: saldo inválido (escriba solo números).")
			return
		}
		if err := b.AbrirCuenta(nombre, s); err != nil {
			fmt.Fprintln(w, "No se pudo crear la cuenta:", err)
			return
		}
	}
	for {
		fmt.Fprintf(w, "\n===== CAJERO - %s =====\n", nombre)
		fmt.Fprintln(w, "1. Retirar  2. Abonar  3. Saldo  4. Mis transacciones  5. Salir")
		opc, ok := leer("Opción: ")
		if !ok {
			fin()
			return
		}
		switch opc {
		case "1", "2":
			txt, ok := leer("Monto: ")
			if !ok {
				fin()
				return
			}
			m, err := strconv.ParseFloat(txt, 64)
			if err != nil {
				fmt.Fprintln(w, "Monto inválido (escriba solo números).")
				continue
			}
			tipo := banco.TipoRetiro
			if opc == "2" {
				tipo = banco.TipoAbono
			}
			t, err := b.Operar(nombre, tipo, m)
			mostrar(w, t, err)
		case "3":
			s, _ := b.Saldo(nombre)
			fmt.Fprintln(w, "Saldo actual:", s)
		case "4":
			imprimirHistorial(w, b, nombre)
		case "5":
			fmt.Fprintln(w, "Sesión finalizada.")
			return
		default: // incluye la opción vacía (Enter)
			fmt.Fprintln(w, "Opción no válida.")
		}
	}
}
