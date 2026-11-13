// Pruebas unitarias del paquete banco (go test).
// Autor: Paucar Ventura, Luis Antonio
package banco

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
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
func TestConcurrencia(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "t.json")
	b, _ := NuevoBanco([]DatoCuenta{{"Alice", M(1000)}}, NewBitacoraJSON(ruta))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); b.Abonar("Alice", 1) }()
		go func() { defer wg.Done(); b.Retirar("Alice", 1) }()
	}
	wg.Wait()
	s, _ := b.Saldo("Alice")
	h, _ := b.Historial("")
	if s != M(1000) || len(h) != 100 {
		t.Fatalf("saldo=%s registros=%d; esperado 1000.00 y 100", s, len(h))
	}
}
