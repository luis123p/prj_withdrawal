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
		imprimirHistorial(b, *nombre)
	case *op != "":
		os.Exit(operacion(b, *op, *nombre, *monto))
	case *interactivo:
		cajeroCLI(b, *nombre)
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

func mostrar(t banco.Transaccion, err error) {
	switch {
	case err != nil && t.ID == "":
		fmt.Println("ERROR:", err)
	case err != nil:
		fmt.Printf("[%s] ERROR: %v\n", t.ID, err)
	case t.Estado == banco.EstadoExitosa:
		fmt.Printf("[%s] %s exitoso. Nuevo saldo: %s\n", t.ID, strings.ToLower(t.Tipo), t.SaldoFinal)
	default:
		fmt.Printf("[%s] %s %s: %s. Saldo: %s\n", t.ID, t.Tipo, t.Estado, t.Mensaje, t.SaldoFinal)
	}
}

func operacion(b *banco.Banco, op, nombre string, monto float64) int {
	tipo := map[string]string{"retiro": banco.TipoRetiro, "abono": banco.TipoAbono}[op]
	if tipo == "" {
		fmt.Println("Error: -op debe ser 'retiro' o 'abono'")
		return 1
	}
	t, err := b.Operar(nombre, tipo, monto)
	mostrar(t, err)
	if err != nil {
		return 1
	}
	return 0
}

func imprimirHistorial(b *banco.Banco, titular string) {
	h, err := b.Historial(titular)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("%-8s %-19s %-8s %-7s %9s %9s %9s  %-9s %s\n",
		"ID", "FECHA", "TITULAR", "TIPO", "MONTO", "S.ANTES", "S.DESPUES", "ESTADO", "MENSAJE")
	for i := len(h) - 1; i >= 0; i-- { // en orden cronológico
		t := h[i]
		fmt.Printf("%-8s %-19s %-8s %-7s %9s %9s %9s  %-9s %s\n",
			t.ID, t.Fecha, t.Titular, t.Tipo, t.Monto, t.SaldoAnterior, t.SaldoFinal, t.Estado, t.Mensaje)
	}
	fmt.Printf("(%d transacciones)\n", len(h))
}

func cajeroCLI(b *banco.Banco, nombre string) {
	in := bufio.NewReader(os.Stdin)
	leer := func(p string) string {
		fmt.Print(p)
		s, _ := in.ReadString('\n')
		return strings.TrimSpace(s)
	}
	if nombre == "" {
		nombre = leer("Nombre del titular: ")
	}
	if _, err := b.Saldo(nombre); err != nil {
		s, err := strconv.ParseFloat(leer("Cuenta nueva. Saldo inicial: "), 64)
		if err != nil {
			fmt.Println("No se pudo crear la cuenta: saldo inválido (escriba solo números).")
			return
		}
		if err := b.AbrirCuenta(nombre, s); err != nil {
			fmt.Println("No se pudo crear la cuenta:", err)
			return
		}
		nombre = strings.TrimSpace(nombre)
	}
	for {
		fmt.Printf("\n===== CAJERO - %s =====\n", nombre)
		fmt.Println("1. Retirar  2. Abonar  3. Saldo  4. Mis transacciones  5. Salir")
		opc := leer("Opción: ")
		switch opc {
		case "1", "2":
			m, err := strconv.ParseFloat(leer("Monto: "), 64)
			if err != nil {
				fmt.Println("Monto inválido (escriba solo números).")
				continue
			}
			tipo := banco.TipoRetiro
			if opc == "2" {
				tipo = banco.TipoAbono
			}
			mostrar(b.Operar(nombre, tipo, m))
		case "3":
			s, _ := b.Saldo(nombre)
			fmt.Println("Saldo actual:", s)
		case "4":
			imprimirHistorial(b, nombre)
		case "5", "":
			fmt.Println("Sesión finalizada.")
			return
		default:
			fmt.Println("Opción no válida.")
		}
	}
}
