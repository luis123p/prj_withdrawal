package banco

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Tipos y estados (modelo de contexto OCL del codeLAB + iteración 3).
const (
	TipoRetiro   = "RETIRO"
	TipoAbono    = "ABONO"
	TipoApertura = "APERTURA" // alta de una cuenta nueva con su saldo inicial

	EstadoExitosa   = "EXITOSA"
	EstadoRechazada = "RECHAZADA"
	EstadoError     = "ERROR"
)

// Transaccion es un registro del historial.
type Transaccion struct {
	ID            string       `json:"id"`
	Fecha         string       `json:"fecha"`
	Titular       string       `json:"titular"`
	Tipo          string       `json:"tipo"`
	Monto         MontoEscrito `json:"monto"` // tal como lo escribió el usuario
	SaldoAnterior Monto        `json:"saldo_anterior"`
	SaldoFinal    Monto        `json:"saldo_final"`
	Estado        string       `json:"estado"`
	Mensaje       string       `json:"mensaje"`
}

// Bitacora guarda y lista transacciones.
type Bitacora interface {
	Registrar(t Transaccion) error
	Listar() ([]Transaccion, error)
}

// BitacoraJSON guarda el historial en un archivo JSON (arreglo de objetos).
type BitacoraJSON struct {
	Ruta string
	mu   sync.Mutex
}

func NewBitacoraJSON(ruta string) *BitacoraJSON { return &BitacoraJSON{Ruta: ruta} }

func (b *BitacoraJSON) Listar() ([]Transaccion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.leer()
}

func (b *BitacoraJSON) leer() ([]Transaccion, error) {
	datos, err := os.ReadFile(b.Ruta)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(datos) == 0) {
		return []Transaccion{}, nil
	}
	if err != nil {
		return nil, err
	}
	var lista []Transaccion
	if err := json.Unmarshal(datos, &lista); err != nil {
		return nil, fmt.Errorf("log JSON dañado (%s): %w", b.Ruta, err)
	}
	return lista, nil
}

// Registrar escribe en un .tmp y luego renombra, para no dejar el log a medias.
func (b *BitacoraJSON) Registrar(t Transaccion) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	lista, err := b.leer()
	if err != nil {
		return err
	}
	datos, err := json.MarshalIndent(append(lista, t), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.Ruta), 0o755); err != nil {
		return err
	}
	tmp := b.Ruta + ".tmp"
	if err := os.WriteFile(tmp, datos, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.Ruta)
}

// BitacoraMemoria se usa en las pruebas; con Falla=true simula un error al guardar.
type BitacoraMemoria struct {
	Items []Transaccion
	Falla bool
}

func (b *BitacoraMemoria) Registrar(t Transaccion) error {
	if b.Falla {
		return errors.New("no se pudo escribir en la bitácora")
	}
	b.Items = append(b.Items, t)
	return nil
}

func (b *BitacoraMemoria) Listar() ([]Transaccion, error) { return b.Items, nil }
