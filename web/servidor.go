// Package web expone el banco como una API HTTP (JSON) y sirve la interfaz web.
// Autor: Paucar Ventura, Luis Antonio
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"prj_withdrawal/banco"
	"prj_withdrawal/bdd"
)

//go:embed static
var estaticos embed.FS

type Servidor struct {
	banco    *banco.Banco
	features fs.FS
	mux      *http.ServeMux
}

// Nuevo arma las rutas. features puede ser nil si no se quiere la pestaña BDD.
func Nuevo(b *banco.Banco, features fs.FS) *Servidor {
	s := &Servidor{banco: b, features: features, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/cuentas", s.listarCuentas)
	s.mux.HandleFunc("POST /api/cuentas", s.abrirCuenta)
	s.mux.HandleFunc("POST /api/operaciones", s.operar)
	s.mux.HandleFunc("GET /api/transacciones", s.historial)
	s.mux.HandleFunc("GET /api/bdd", s.pruebasBDD)
	static, _ := fs.Sub(estaticos, "static")
	s.mux.Handle("GET /", http.FileServer(http.FS(static)))
	return s
}

func (s *Servidor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	inicio := time.Now()
	s.mux.ServeHTTP(w, r)
	if strings.HasPrefix(r.URL.Path, "/api/") {
		log.Printf("%s %s (%v)", r.Method, r.URL.Path, time.Since(inicio).Round(time.Microsecond))
	}
}

func responder(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(v)
}

func responderError(w http.ResponseWriter, codigo int, msg string) {
	responder(w, codigo, map[string]string{"error": msg})
}

func (s *Servidor) listarCuentas(w http.ResponseWriter, r *http.Request) {
	responder(w, http.StatusOK, s.banco.Cuentas())
}

func (s *Servidor) abrirCuenta(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Titular string  `json:"titular"`
		Saldo   float64 `json:"saldo"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		responderError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	err := s.banco.AbrirCuenta(req.Titular, req.Saldo)
	switch {
	case errors.Is(err, banco.ErrCuentaExiste):
		responderError(w, http.StatusConflict, err.Error())
	case errors.Is(err, banco.ErrRegistro):
		responderError(w, http.StatusInternalServerError, err.Error())
	case err != nil:
		responderError(w, http.StatusBadRequest, err.Error())
	default:
		responder(w, http.StatusCreated, s.banco.Cuentas())
	}
}

type respuestaOperacion struct {
	Transaccion banco.Transaccion `json:"transaccion"`
	Saldo       banco.Monto       `json:"saldo"`
	Error       string            `json:"error,omitempty"`
}

func (s *Servidor) operar(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Titular string   `json:"titular"`
		Tipo    string   `json:"tipo"`
		Monto   *float64 `json:"monto"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		responderError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Monto == nil {
		responderError(w, http.StatusBadRequest, "falta el monto")
		return
	}
	tipo := map[string]string{"retiro": banco.TipoRetiro, "abono": banco.TipoAbono}[strings.ToLower(req.Tipo)]
	if tipo == "" {
		responderError(w, http.StatusBadRequest, "tipo debe ser 'retiro' o 'abono'")
		return
	}
	tx, err := s.banco.Operar(req.Titular, tipo, *req.Monto)
	if errors.Is(err, banco.ErrCuentaNoExiste) {
		responderError(w, http.StatusNotFound, err.Error())
		return
	}
	saldo, _ := s.banco.Saldo(req.Titular)
	if err != nil { // no se pudo registrar: la operación se revirtió
		responder(w, http.StatusInternalServerError, respuestaOperacion{tx, saldo, err.Error()})
		return
	}
	// Rechazos y errores de validación también son 200: la transacción
	// se procesó y quedó registrada; el estado viene en la respuesta.
	responder(w, http.StatusOK, respuestaOperacion{Transaccion: tx, Saldo: saldo})
}

func (s *Servidor) historial(w http.ResponseWriter, r *http.Request) {
	h, err := s.banco.Historial(r.URL.Query().Get("titular"))
	if err != nil {
		responderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	responder(w, http.StatusOK, h)
}

func (s *Servidor) pruebasBDD(w http.ResponseWriter, r *http.Request) {
	if s.features == nil {
		responderError(w, http.StatusNotFound, "no hay features cargados")
		return
	}
	rep, err := bdd.Ejecutar(s.features, nil)
	if err != nil {
		responderError(w, http.StatusInternalServerError, err.Error())
		return
	}
	responder(w, http.StatusOK, rep)
}
