// Pruebas unitarias del paquete banco (go test).
// Autor: Paucar Ventura, Luis Antonio
package banco

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNuevoMonto(t *testing.T) {
	casos := []struct {
		in    float64
		cent  Monto
		error bool
	}{
		{234.56, 23456, false},
		{0.1 + 0.2, 30, false}, // 0.30000000000000004 en float64
		{-20, -2000, false},
		{10.555, 0, true},
		{1e12, 0, true},
	}
	for _, c := range casos {
		m, err := NuevoMonto(c.in)
		if (err != nil) != c.error || (!c.error && m != c.cent) {
			t.Errorf("NuevoMonto(%v) = %d, %v", c.in, m, err)
		}
	}
}

func TestSinErrorDeRedondeo(t *testing.T) {
	// Con float64, 10 abonos de 0.10 no dan exactamente 1.00. Con céntimos sí.
	c := NewCuenta("T", 0)
	for i := 0; i < 10; i++ {
		c.Depositar(M(0.10))
	}
	if c.Saldo() != M(1.00) {
		t.Fatalf("saldo = %s, esperado 1.00", c.Saldo())
	}
}

func TestEstados(t *testing.T) {
	casos := []struct {
		nombre, tipo string
		saldo, monto float64
		estado       string
		saldoFinal   float64
	}{
		{"retiro exitoso", TipoRetiro, 234.56, 200, EstadoExitosa, 34.56},
		{"retiro sin fondos", TipoRetiro, 198.76, 200, EstadoRechazada, 198.76},
		{"retiro negativo", TipoRetiro, 100, -50, EstadoError, 100},
		{"retiro cero", TipoRetiro, 100, 0, EstadoError, 100},
		{"retiro todo el saldo", TipoRetiro, 100, 100, EstadoExitosa, 0},
		{"abono exitoso", TipoAbono, 234.56, 100, EstadoExitosa, 334.56},
		{"abono en el limite", TipoAbono, 10, 5000, EstadoExitosa, 5010},
		{"abono sobre el limite", TipoAbono, 198.76, 6000, EstadoRechazada, 198.76},
		{"abono cero", TipoAbono, 150, 0, EstadoError, 150},
		{"abono con 3 decimales", TipoAbono, 150, 10.555, EstadoError, 150},
	}
	for _, tc := range casos {
		t.Run(tc.nombre, func(t *testing.T) {
			c, log := NewCuenta("T", M(tc.saldo)), &BitacoraMemoria{}
			tx, err := Ejecutar(c, log, tc.tipo, tc.monto)
			if err != nil {
				t.Fatal(err)
			}
			if tx.Estado != tc.estado || c.Saldo() != M(tc.saldoFinal) || len(log.Items) != 1 {
				t.Fatalf("estado=%s saldo=%s registros=%d", tx.Estado, c.Saldo(), len(log.Items))
			}
		})
	}
}

func TestRollback(t *testing.T) {
	c := NewCuenta("Alice", M(234.56))
	_, err := Ejecutar(c, &BitacoraMemoria{Falla: true}, TipoRetiro, 200)
	if err == nil || c.Saldo() != M(234.56) {
		t.Fatalf("esperado error y saldo 234.56; err=%v saldo=%s", err, c.Saldo())
	}
}

func TestBitacoraJSONyRecarga(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "data", "transacciones.json")
	datos, _ := CargarCuentas("")
	b, err := NuevoBanco(datos, NewBitacoraJSON(ruta))
	if err != nil {
		t.Fatal(err)
	}
	b.Retirar("Alice", 200) // EXITOSA -> 34.56
	b.Retirar("Alice", 500) // RECHAZADA
	b.Abonar("Alice", -1)   // ERROR

	// Al volver a abrir el banco el saldo sale del log.
	b2, _ := NuevoBanco(datos, NewBitacoraJSON(ruta))
	if s, _ := b2.Saldo("Alice"); s != M(34.56) {
		t.Fatalf("saldo recargado = %s, esperado 34.56", s)
	}
	h, _ := b2.Historial("Alice")
	if len(h) != 3 || h[0].ID != "TX-0003" || h[0].Estado != EstadoError {
		t.Fatalf("historial inesperado: %+v", h)
	}
	if _, err := os.Stat(ruta + ".tmp"); err == nil {
		t.Error("quedó el archivo temporal")
	}
}

func TestLogDanado(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "t.json")
	os.WriteFile(ruta, []byte("{no es json"), 0o644)
	if _, err := NuevoBanco(nil, NewBitacoraJSON(ruta)); err == nil {
		t.Fatal("se esperaba error por log dañado")
	}
}

func TestCuentas(t *testing.T) {
	b, _ := NuevoBanco(nil, &BitacoraMemoria{})
	if err := b.AbrirCuenta("Luis", 500); err != nil {
		t.Fatal(err)
	}
	if err := b.AbrirCuenta("Luis", 1); err != ErrCuentaExiste {
		t.Fatalf("esperado ErrCuentaExiste, obtenido %v", err)
	}
	if err := b.AbrirCuenta("  ", 1); err != ErrTitularVacio {
		t.Fatalf("esperado ErrTitularVacio, obtenido %v", err)
	}
	if _, err := b.Retirar("Nadie", 1); err != ErrCuentaNoExiste {
		t.Fatalf("esperado ErrCuentaNoExiste, obtenido %v", err)
	}
}

// Muchas operaciones en paralelo no deben perder dinero ni registros.
// Los retiros son de S/ 10 (RN-09) y suman 500, por debajo del límite diario (RN-10).
func TestConcurrencia(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "t.json")
	b, _ := NuevoBanco([]DatoCuenta{{"Alice", M(1000)}}, NewBitacoraJSON(ruta))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); b.Abonar("Alice", 10) }()
		go func() { defer wg.Done(); b.Retirar("Alice", 10) }()
	}
	wg.Wait()
	s, _ := b.Saldo("Alice")
	h, _ := b.Historial("")
	if s != M(1000) || len(h) != 100 {
		t.Fatalf("saldo=%s registros=%d; esperado 1000.00 y 100", s, len(h))
	}
	for _, tx := range h {
		if tx.Estado != EstadoExitosa {
			t.Fatalf("todas debían ser EXITOSA: %+v", tx)
		}
	}
}

// Una cuenta nueva sin movimientos se conserva al reiniciar (transacción APERTURA).
func TestAperturaSeConserva(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "t.json")
	datos, _ := CargarCuentas("")
	b, _ := NuevoBanco(datos, NewBitacoraJSON(ruta))
	if err := b.AbrirCuenta("  Luis ", 500); err != nil {
		t.Fatal(err)
	}
	b2, _ := NuevoBanco(datos, NewBitacoraJSON(ruta))
	if s, err := b2.Saldo("Luis"); err != nil || s != M(500) {
		t.Fatalf("Luis tras reiniciar: saldo=%s err=%v", s, err)
	}
	h, _ := b2.Historial("Luis")
	if len(h) != 1 || h[0].Tipo != TipoApertura || h[0].Estado != EstadoExitosa || h[0].SaldoFinal != M(500) {
		t.Fatalf("historial: %+v", h)
	}
}

// Si la apertura no se puede registrar, la cuenta no se crea.
func TestAperturaSinRegistro(t *testing.T) {
	b, _ := NuevoBanco(nil, &BitacoraMemoria{Falla: true})
	if err := b.AbrirCuenta("Luis", 500); !errors.Is(err, ErrRegistro) {
		t.Fatalf("esperado ErrRegistro, obtenido %v", err)
	}
	if _, err := b.Saldo("Luis"); err != ErrCuentaNoExiste {
		t.Fatal("la cuenta no debía crearse")
	}
}

// El log guarda el monto tal como se escribió, también si es inválido.
func TestMontoEscritoEnLog(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "t.json")
	b, _ := NuevoBanco([]DatoCuenta{{"Alice", M(100)}}, NewBitacoraJSON(ruta))
	casos := []struct {
		monto float64
		texto string
	}{{1e300, "1e+300"}, {10.555, "10.555"}, {0.001, "0.001"}, {-1e300, "-1e+300"}, {20, "20.00"}, {math.NaN(), "NaN"}}
	for _, c := range casos {
		if _, err := b.Abonar("Alice", c.monto); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(ruta)
	h, err := NewBitacoraJSON(ruta).Listar() // el JSON sigue siendo válido
	if err != nil {
		t.Fatalf("%v\n%s", err, raw)
	}
	for i, c := range casos {
		if h[i].Monto.String() != c.texto {
			t.Errorf("monto %v se guardó como %s, esperado %s", c.monto, h[i].Monto, c.texto)
		}
	}
	if s, _ := b.Saldo("Alice"); s != M(120) {
		t.Fatalf("saldo = %s, esperado 120.00", s)
	}
}

func TestValidarTitular(t *testing.T) {
	validos := []string{"Luis", "María José", "Peña", "O'Brien", "Ana-Lucía", "Jr. Pérez", "Cliente 2", strings.Repeat("ñ", 30)}
	for _, n := range validos {
		if _, err := ValidarTitular(n); err != nil {
			t.Errorf("%q debería ser válido: %v", n, err)
		}
	}
	invalidos := []string{strings.Repeat("x", 31), strings.Repeat("a", 300), `<img src=x onerror=alert(1)>`, "Luis;DROP",
		"-Luis", ".Luis", "'Luis", "’Luis", " - Luis"}
	for _, n := range invalidos {
		if _, err := ValidarTitular(n); err != ErrTitularInvalido {
			t.Errorf("%q debería ser inválido, obtenido %v", n, err)
		}
	}
	for _, n := range []string{"", "   ", "\t\n"} {
		if _, err := ValidarTitular(n); err != ErrTitularVacio {
			t.Errorf("%q debería dar ErrTitularVacio, obtenido %v", n, err)
		}
	}
	// Los espacios se normalizan: extremos fuera y repetidos reducidos a uno.
	normalizados := map[string]string{"  Ana   Paula ": "Ana Paula", "Ana\tPaula": "Ana Paula", "Lu\nis": "Lu is", "1 Luis": "1 Luis"}
	for in, want := range normalizados {
		if got, err := ValidarTitular(in); err != nil || got != want {
			t.Errorf("ValidarTitular(%q) = %q, %v; esperado %q", in, got, err, want)
		}
	}
	// 30 caracteres después de normalizar es válido aunque la entrada sea más larga.
	if _, err := ValidarTitular("  " + strings.Repeat("a", 15) + "    " + strings.Repeat("b", 14) + "  "); err != nil {
		t.Errorf("30 caracteres normalizados debería ser válido: %v", err)
	}
}

// "Ana  Paula" y "Ana Paula" son la misma cuenta; " Alice " encuentra a "Alice".
func TestTitularNormalizado(t *testing.T) {
	b, _ := NuevoBanco([]DatoCuenta{{"Alice", M(100)}}, &BitacoraMemoria{})
	if err := b.AbrirCuenta("Ana  Paula", 50); err != nil {
		t.Fatal(err)
	}
	if err := b.AbrirCuenta("Ana Paula", 50); err != ErrCuentaExiste {
		t.Fatalf("esperado ErrCuentaExiste, obtenido %v", err)
	}
	if err := b.AbrirCuenta("-Luis", 50); err != ErrTitularInvalido {
		t.Fatalf("esperado ErrTitularInvalido, obtenido %v", err)
	}
	if s, err := b.Saldo(" Ana   Paula "); err != nil || s != M(50) {
		t.Fatalf("Saldo(\" Ana   Paula \") = %s, %v", s, err)
	}
	tx, err := b.Retirar(" Alice ", 10)
	if err != nil || tx.Titular != "Alice" || tx.Estado != EstadoExitosa {
		t.Fatalf("retiro de \" Alice \": %+v, %v", tx, err)
	}
	if s, _ := b.Saldo("Alice"); s != M(90) {
		t.Fatalf("saldo de Alice = %s, esperado 90.00", s)
	}
	if h, _ := b.Historial("  Alice"); len(h) != 1 {
		t.Fatalf("Historial(\"  Alice\") = %d registros, esperado 1", len(h))
	}
}

// fijarReloj fija banco.Reloj en una fecha y la restaura al terminar la prueba.
func fijarReloj(t *testing.T, fecha string) {
	t.Helper()
	antes := Reloj
	t.Cleanup(func() { Reloj = antes })
	f, err := time.ParseInLocation("2006-01-02 15:04", fecha, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	Reloj = func() time.Time { return f }
}

// retiroPrevio simula un retiro ya registrado en el log.
func retiroPrevio(titular, fecha string, monto float64, estado string) Transaccion {
	return Transaccion{Titular: titular, Tipo: TipoRetiro, Fecha: fecha + " 09:00:00", Monto: MontoEscrito(monto), Estado: estado}
}

// RN-09: el cajero solo entrega billetes, en múltiplos de S/ 10.
func TestRetiroMultiploDe10(t *testing.T) {
	const msg = "el monto a retirar debe ser múltiplo de S/ 10.00"
	for _, m := range []float64{25, 15, 0.5, 105, 9.99, 10.01} {
		c := NewCuenta("T", M(500))
		tx, _ := Ejecutar(c, &BitacoraMemoria{}, TipoRetiro, m)
		if tx.Estado != EstadoError || tx.Mensaje != msg || c.Saldo() != M(500) {
			t.Errorf("retiro %v: %s %q saldo=%s", m, tx.Estado, tx.Mensaje, c.Saldo())
		}
	}
	for _, m := range []float64{10, 20, 100, 500} {
		c := NewCuenta("T", M(500))
		if tx, _ := Ejecutar(c, &BitacoraMemoria{}, TipoRetiro, m); tx.Estado != EstadoExitosa {
			t.Errorf("retiro %v: %s %q", m, tx.Estado, tx.Mensaje)
		}
	}
	// El abono no cambia: puede ser cualquier monto con 2 decimales.
	c := NewCuenta("T", M(500))
	if tx, _ := Ejecutar(c, &BitacoraMemoria{}, TipoAbono, 25.5); tx.Estado != EstadoExitosa {
		t.Errorf("abono 25.50: %s %q", tx.Estado, tx.Mensaje)
	}
}

// Orden: monto > 0, 2 decimales, múltiplo de 10, límite diario y por último saldo.
func TestOrdenValidacionRetiro(t *testing.T) {
	fijarReloj(t, "2026-11-17 10:00")
	casos := []struct {
		monto  float64
		estado string
		msg    string
	}{
		{-10.555, EstadoError, "el monto a retirar debe ser mayor que cero"},
		{0, EstadoError, "el monto a retirar debe ser mayor que cero"},
		{10.555, EstadoError, "el monto solo puede tener hasta 2 decimales"},
		{25, EstadoError, "el monto a retirar debe ser múltiplo de S/ 10.00"},
		{2010, EstadoError, "excede el límite diario de retiro (S/ 2000.00; disponible hoy: S/ 2000.00)"},
		{30, EstadoRechazada, "saldo insuficiente"},
	}
	for _, c := range casos {
		cta := NewCuenta("T", M(20)) // poco saldo: los errores previos van antes que RECHAZADA
		log := &BitacoraMemoria{}
		tx, err := Ejecutar(cta, log, TipoRetiro, c.monto)
		if err != nil || tx.Estado != c.estado || tx.Mensaje != c.msg || cta.Saldo() != M(20) || len(log.Items) != 1 {
			t.Errorf("retiro %v: %s %q (registros=%d, err=%v)", c.monto, tx.Estado, tx.Mensaje, len(log.Items), err)
		}
	}
}

// RN-10: máximo S/ 2000.00 por día y titular, sumando solo los retiros EXITOSA.
func TestLimiteDiario(t *testing.T) {
	fijarReloj(t, "2026-11-17 10:00")
	c, log := NewCuenta("Ana", M(5000)), &BitacoraMemoria{}
	for _, m := range []float64{1000, 900, 100} { // llega justo a 2000
		if tx, _ := Ejecutar(c, log, TipoRetiro, m); tx.Estado != EstadoExitosa {
			t.Fatalf("retiro %v: %s %q", m, tx.Estado, tx.Mensaje)
		}
	}
	tx, _ := Ejecutar(c, log, TipoRetiro, 10)
	want := "excede el límite diario de retiro (S/ 2000.00; disponible hoy: S/ 0.00)"
	if tx.Estado != EstadoError || tx.Mensaje != want || c.Saldo() != M(3000) {
		t.Fatalf("retiro sobre el límite: %s %q saldo=%s", tx.Estado, tx.Mensaje, c.Saldo())
	}
	// El abono no tiene límite diario.
	if tx, _ := Ejecutar(c, log, TipoAbono, 100); tx.Estado != EstadoExitosa {
		t.Fatalf("abono: %s %q", tx.Estado, tx.Mensaje)
	}
}

func TestLimiteDiarioDisponible(t *testing.T) {
	fijarReloj(t, "2026-11-17 10:00")
	c := NewCuenta("Ana", M(5000))
	log := &BitacoraMemoria{Items: []Transaccion{retiroPrevio("Ana", "2026-11-17", 1900, EstadoExitosa)}}
	tx, _ := Ejecutar(c, log, TipoRetiro, 200)
	want := "excede el límite diario de retiro (S/ 2000.00; disponible hoy: S/ 100.00)"
	if tx.Estado != EstadoError || tx.Mensaje != want || c.Saldo() != M(5000) {
		t.Fatalf("%s %q saldo=%s", tx.Estado, tx.Mensaje, c.Saldo())
	}
}

// Un acumulado de 2000.01 ya supera el límite (p. ej. un retiro de 1990.01 de antes de RN-09).
func TestLimiteDiarioAcumulado200001(t *testing.T) {
	fijarReloj(t, "2026-11-17 10:00")
	c := NewCuenta("Ana", M(5000))
	log := &BitacoraMemoria{Items: []Transaccion{retiroPrevio("Ana", "2026-11-17", 1990.01, EstadoExitosa)}}
	tx, _ := Ejecutar(c, log, TipoRetiro, 10)
	want := "excede el límite diario de retiro (S/ 2000.00; disponible hoy: S/ 9.99)"
	if tx.Estado != EstadoError || tx.Mensaje != want {
		t.Fatalf("%s %q", tx.Estado, tx.Mensaje)
	}
}

// No cuentan: retiros RECHAZADA o ERROR, abonos, otros titulares ni otros días.
func TestLimiteDiarioSoloCuentaExitosasDeHoy(t *testing.T) {
	fijarReloj(t, "2026-11-17 10:00")
	abono := retiroPrevio("Ana", "2026-11-17", 1500, EstadoExitosa)
	abono.Tipo = TipoAbono
	log := &BitacoraMemoria{Items: []Transaccion{
		retiroPrevio("Ana", "2026-11-17", 1500, EstadoRechazada),
		retiroPrevio("Ana", "2026-11-17", 1500, EstadoError),
		retiroPrevio("Ana", "2026-11-16", 2000, EstadoExitosa),
		retiroPrevio("Beto", "2026-11-17", 2000, EstadoExitosa),
		abono,
	}}
	c := NewCuenta("Ana", M(5000))
	if tx, _ := Ejecutar(c, log, TipoRetiro, 2000); tx.Estado != EstadoExitosa {
		t.Fatalf("%s %q", tx.Estado, tx.Mensaje)
	}
}

// Al cambiar de día el límite se reinicia.
func TestLimiteDiarioSeReinicia(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "t.json")
	b, _ := NuevoBanco([]DatoCuenta{{"Ana", M(5000)}}, NewBitacoraJSON(ruta))
	fijarReloj(t, "2026-11-17 23:59")
	if tx, _ := b.Retirar("Ana", 2000); tx.Estado != EstadoExitosa {
		t.Fatalf("día 1: %s %q", tx.Estado, tx.Mensaje)
	}
	if tx, _ := b.Retirar("Ana", 10); tx.Estado != EstadoError {
		t.Fatalf("día 1 sobre el límite: %s %q", tx.Estado, tx.Mensaje)
	}
	fijarReloj(t, "2026-11-18 00:00")
	if tx, _ := b.Retirar("Ana", 2000); tx.Estado != EstadoExitosa {
		t.Fatalf("día 2: %s %q", tx.Estado, tx.Mensaje)
	}
	if s, _ := b.Saldo("Ana"); s != M(1000) {
		t.Fatalf("saldo = %s, esperado 1000.00", s)
	}
}

// El listado de cuentas informa cuánto puede retirar hoy cada titular.
func TestCuentasDisponibleHoy(t *testing.T) {
	fijarReloj(t, "2026-11-17 10:00")
	b, _ := NuevoBanco([]DatoCuenta{{"Ana", M(5000)}, {"Beto", M(10)}}, &BitacoraMemoria{})
	b.Retirar("Ana", 1500)
	for _, c := range b.Cuentas() {
		want := map[string]Monto{"Ana": M(500), "Beto": M(2000)}[c.Titular]
		if c.DisponibleHoy != want {
			t.Errorf("%s: disponible hoy %s, esperado %s", c.Titular, c.DisponibleHoy, want)
		}
	}
}
