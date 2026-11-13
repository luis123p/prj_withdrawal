// Pruebas de la API con httptest.
// Autor: Paucar Ventura, Luis Antonio
package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"prj_withdrawal/banco"
)

func nuevoServidor(t *testing.T, log banco.Bitacora) *Servidor {
	datos, _ := banco.CargarCuentas("")
	b, err := banco.NuevoBanco(datos, log)
	if err != nil {
		t.Fatal(err)
	}
	return Nuevo(b, os.DirFS("../features"))
}

func pedir(t *testing.T, s *Servidor, metodo, ruta, cuerpo string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var m map[string]any
	json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

func TestOperacionesAPI(t *testing.T) {
	s := nuevoServidor(t, &banco.BitacoraMemoria{})
	casos := []struct {
		cuerpo, estado string
		codigo         int
		saldo          float64
	}{
		{`{"titular":"Alice","tipo":"retiro","monto":200}`, "EXITOSA", 200, 34.56},
		{`{"titular":"Alice","tipo":"retiro","monto":100}`, "RECHAZADA", 200, 34.56},
		{`{"titular":"Alice","tipo":"abono","monto":-5}`, "ERROR", 200, 34.56},
		{`{"titular":"Alice","tipo":"abono","monto":65.44}`, "EXITOSA", 200, 100},
	}
	for _, c := range casos {
		rec, m := pedir(t, s, "POST", "/api/operaciones", c.cuerpo)
		tx, _ := m["transaccion"].(map[string]any)
		if rec.Code != c.codigo || tx["estado"] != c.estado || m["saldo"] != c.saldo {
			t.Errorf("%s -> %d %v", c.cuerpo, rec.Code, rec.Body.String())
		}
	}
	rec, _ := pedir(t, s, "GET", "/api/transacciones?titular=Alice", "")
	var h []banco.Transaccion
	json.Unmarshal(rec.Body.Bytes(), &h)
	if len(h) != 4 || h[0].ID != "TX-0004" {
		t.Fatalf("historial: %s", rec.Body.String())
	}
}

func TestErroresAPI(t *testing.T) {
	s := nuevoServidor(t, &banco.BitacoraMemoria{})
	casos := []struct {
		metodo, ruta, cuerpo string
		codigo               int
	}{
		{"POST", "/api/operaciones", `{"titular":"Nadie","tipo":"retiro","monto":1}`, 404},
		{"POST", "/api/operaciones", `{"titular":"Alice","tipo":"prestamo","monto":1}`, 400},
		{"POST", "/api/operaciones", `{"titular":"Alice","tipo":"retiro"}`, 400},
		{"POST", "/api/operaciones", `no es json`, 400},
		{"POST", "/api/cuentas", `{"titular":"Alice","saldo":1}`, 409},
		{"POST", "/api/cuentas", `{"titular":"","saldo":1}`, 400},
		{"POST", "/api/cuentas", `{"titular":"Luis","saldo":500}`, 201},
		{"GET", "/api/cuentas", ``, 200},
	}
	for _, c := range casos {
		if rec, _ := pedir(t, s, c.metodo, c.ruta, c.cuerpo); rec.Code != c.codigo {
			t.Errorf("%s %s %s -> %d, esperado %d", c.metodo, c.ruta, c.cuerpo, rec.Code, c.codigo)
		}
	}
}

func TestRollbackAPI(t *testing.T) {
	s := nuevoServidor(t, &banco.BitacoraMemoria{Falla: true})
	rec, m := pedir(t, s, "POST", "/api/operaciones", `{"titular":"Alice","tipo":"retiro","monto":200}`)
	if rec.Code != 500 || m["saldo"] != 234.56 {
		t.Fatalf("esperado 500 y saldo 234.56: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPaginaYBDD(t *testing.T) {
	s := nuevoServidor(t, &banco.BitacoraMemoria{})
	rec, _ := pedir(t, s, "GET", "/", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>") {
		t.Fatalf("index: %d", rec.Code)
	}
	rec, m := pedir(t, s, "GET", "/api/bdd", "")
	if rec.Code != 200 || m["total"] != m["pasaron"] {
		t.Fatalf("bdd: %s", rec.Body.String())
	}
}
