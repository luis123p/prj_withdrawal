// Pruebas del cajero de terminal (modo -i, -op y -historial).
// Autor: Paucar Ventura, Luis Antonio
package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"prj_withdrawal/banco"
)

func bancoPrueba(t *testing.T) *banco.Banco {
	t.Helper()
	datos, _ := banco.CargarCuentas("")
	b, err := banco.NuevoBanco(datos, &banco.BitacoraMemoria{})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// correrCajero ejecuta cajeroCLI con la entrada dada y falla si no termina
// (por ejemplo, un bucle infinito al llegar a EOF).
func correrCajero(t *testing.T, b *banco.Banco, nombre, entrada string) string {
	t.Helper()
	var out strings.Builder
	listo := make(chan struct{})
	go func() {
		cajeroCLI(b, nombre, strings.NewReader(entrada), &out)
		close(listo)
	}()
	select {
	case <-listo:
	case <-time.After(3 * time.Second):
		t.Fatalf("cajeroCLI no terminó con la entrada %q", entrada)
	}
	return out.String()
}

func TestCajeroEnterNoCierraSesion(t *testing.T) {
	out := correrCajero(t, bancoPrueba(t), "Alice", "\n\n3\n5\n")
	if n := strings.Count(out, "Opción no válida."); n != 2 {
		t.Errorf("se esperaban 2 \"Opción no válida.\" y hay %d:\n%s", n, out)
	}
	if n := strings.Count(out, "===== CAJERO - Alice ====="); n != 4 {
		t.Errorf("el menú debía mostrarse 4 veces y se mostró %d:\n%s", n, out)
	}
	if !strings.Contains(out, "Saldo actual: 234.56") || !strings.HasSuffix(strings.TrimSpace(out), "Sesión finalizada.") {
		t.Errorf("salida inesperada:\n%s", out)
	}
}

func TestCajeroTerminaConEOF(t *testing.T) {
	for _, entrada := range []string{"", "\n", "1\n", "1", "3\n"} {
		out := correrCajero(t, bancoPrueba(t), "Alice", entrada)
		if !strings.Contains(out, "Sesión finalizada.") {
			t.Errorf("entrada %q: no se cerró la sesión:\n%s", entrada, out)
		}
	}
	// Sin nombre y sin entrada tampoco debe colgarse.
	correrCajero(t, bancoPrueba(t), "", "")
}

func TestCajeroOpcion5ConEOFSinSalto(t *testing.T) {
	out := correrCajero(t, bancoPrueba(t), "Alice", "1\n50\n5")
	if !strings.Contains(out, "retiro exitoso. Nuevo saldo: 184.56") {
		t.Errorf("salida inesperada:\n%s", out)
	}
}

func TestCajeroNormalizaNombre(t *testing.T) {
	b := bancoPrueba(t)
	out := correrCajero(t, b, "  Alice ", "3\n5\n")
	if !strings.Contains(out, "===== CAJERO - Alice =====") || !strings.Contains(out, "Saldo actual: 234.56") {
		t.Errorf("\"  Alice \" debía usar la cuenta Alice:\n%s", out)
	}
	// Cuenta nueva con espacios repetidos: se crea normalizada.
	correrCajero(t, b, "Ana   Paula", "100\n5\n")
	if s, err := b.Saldo("Ana Paula"); err != nil || s != banco.M(100) {
		t.Errorf("cuenta \"Ana Paula\": saldo=%s err=%v", s, err)
	}
}

func TestOperacionNormalizaNombre(t *testing.T) {
	b := bancoPrueba(t)
	var out strings.Builder
	if code := operacion(&out, b, "retiro", " Alice ", 10); code != 0 {
		t.Fatalf("código %d: %s", code, out.String())
	}
	if s, _ := b.Saldo("Alice"); s != banco.M(224.56) {
		t.Fatalf("saldo = %s, esperado 224.56", s)
	}
}

// Las columnas del historial quedan alineadas con APERTURA y nombres largos o con tildes.
func TestHistorialAlineado(t *testing.T) {
	b := bancoPrueba(t)
	for _, n := range []string{"José Ñúñez", "María Peñalo", "Maximiliano Rodríguez"} {
		if err := b.AbrirCuenta(n, 5000); err != nil {
			t.Fatal(err)
		}
	}
	b.Retirar("María Peñalo", 2000)
	b.Retirar("José Ñúñez", 25)
	b.Abonar("Maximiliano Rodríguez", 6000)
	b.Abonar("Alice", 10.555)

	var out strings.Builder
	imprimirHistorial(&out, b, "")
	lineas := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	lineas = lineas[:len(lineas)-1] // la última es "(N transacciones)"
	cab := lineas[0]
	col := func(s, campo string) int { return utf8.RuneCountInString(s[:strings.Index(s, campo)]) }
	cTipo, cEstado := col(cab, "TIPO"), col(cab, "ESTADO")
	cMonto := col(cab, "MONTO") + len("MONTO") // columna alineada a la derecha
	for _, ln := range lineas[1:] {
		r := []rune(ln)
		tipo := strings.Fields(string(r[cTipo:]))[0]
		estado := strings.Fields(string(r[cEstado:]))[0]
		if r[cTipo-1] != ' ' || r[cEstado-1] != ' ' || r[cMonto-1] == ' ' || r[cMonto] != ' ' ||
			!strings.Contains("APERTURA RETIRO ABONO", tipo) || !strings.Contains("EXITOSA RECHAZADA ERROR", estado) {
			t.Errorf("fila desalineada:\n%s\n%s", cab, ln)
		}
	}
	if !strings.Contains(out.String(), "Maximiliano…") {
		t.Errorf("los nombres de más de 12 caracteres debían recortarse:\n%s", out.String())
	}
	t.Log("\n" + out.String())
}
