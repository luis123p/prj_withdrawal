package banco

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
)

var (
	ErrCuentaNoExiste = errors.New("la cuenta no existe")
	ErrCuentaExiste   = errors.New("la cuenta ya existe")
	ErrTitularVacio   = errors.New("el titular no puede estar vacío")
)

// Banco agrupa las cuentas y la bitácora. Lo usan la CLI y la app web.
// El mutex hace que dos operaciones al mismo tiempo (por ejemplo dos
// pestañas del navegador) no se mezclen.
type Banco struct {
	mu      sync.Mutex
	cuentas map[string]*Cuenta
	log     Bitacora
}

// DatoCuenta es una fila del JSON de cuentas.
type DatoCuenta struct {
	Titular string `json:"titular"`
	Saldo   Monto  `json:"saldo"`
}

const cuentasIncrustadas = `[
  {"titular": "Alice", "saldo": 234.56},
  {"titular": "Hamza", "saldo": 198.76},
  {"titular": "Bruno", "saldo": 100.00},
  {"titular": "Carla", "saldo": 150.00}
]`

// CargarCuentas lee el JSON de cuentas de un archivo, o el incrustado si ruta == "".
func CargarCuentas(ruta string) ([]DatoCuenta, error) {
	datos := []byte(cuentasIncrustadas)
	if ruta != "" {
		var err error
		if datos, err = os.ReadFile(ruta); err != nil {
			return nil, err
		}
	}
	var lista []DatoCuenta
	return lista, json.Unmarshal(datos, &lista)
}

// NuevoBanco crea las cuentas. Si un titular ya tiene movimientos en el log,
// su saldo es el saldo final de su última transacción.
func NuevoBanco(datos []DatoCuenta, log Bitacora) (*Banco, error) {
	b := &Banco{cuentas: map[string]*Cuenta{}, log: log}
	for _, d := range datos {
		b.cuentas[d.Titular] = NewCuenta(d.Titular, d.Saldo)
	}
	hist, err := log.Listar()
	if err != nil {
		return nil, err
	}
	for _, t := range hist {
		if c, ok := b.cuentas[t.Titular]; ok {
			c.saldo = t.SaldoFinal
		} else {
			b.cuentas[t.Titular] = NewCuenta(t.Titular, t.SaldoFinal)
		}
	}
	return b, nil
}

// AbrirCuenta crea una cuenta nueva con un saldo inicial.
func (b *Banco) AbrirCuenta(titular string, saldo float64) error {
	titular = strings.TrimSpace(titular)
	if titular == "" {
		return ErrTitularVacio
	}
	m, err := NuevoMonto(saldo)
	if err != nil {
		return err
	}
	if m < 0 {
		return errors.New("el saldo inicial no puede ser negativo")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.cuentas[titular]; ok {
		return ErrCuentaExiste
	}
	b.cuentas[titular] = NewCuenta(titular, m)
	return nil
}

// Operar hace un retiro o abono sobre la cuenta del titular.
func (b *Banco) Operar(titular, tipo string, monto float64) (Transaccion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.cuentas[titular]
	if !ok {
		return Transaccion{}, ErrCuentaNoExiste
	}
	return Ejecutar(c, b.log, tipo, monto)
}

func (b *Banco) Retirar(titular string, monto float64) (Transaccion, error) {
	return b.Operar(titular, TipoRetiro, monto)
}

func (b *Banco) Abonar(titular string, monto float64) (Transaccion, error) {
	return b.Operar(titular, TipoAbono, monto)
}

// Saldo devuelve el saldo actual de un titular.
func (b *Banco) Saldo(titular string) (Monto, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.cuentas[titular]
	if !ok {
		return 0, ErrCuentaNoExiste
	}
	return c.Saldo(), nil
}

// Cuentas devuelve todas las cuentas ordenadas por titular.
func (b *Banco) Cuentas() []DatoCuenta {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]DatoCuenta, 0, len(b.cuentas))
	for _, c := range b.cuentas {
		out = append(out, DatoCuenta{Titular: c.titular, Saldo: c.saldo})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Titular < out[j].Titular })
	return out
}

// Historial devuelve las transacciones (todas o de un titular), más recientes primero.
func (b *Banco) Historial(titular string) ([]Transaccion, error) {
	lista, err := b.log.Listar()
	if err != nil {
		return nil, err
	}
	out := []Transaccion{}
	for i := len(lista) - 1; i >= 0; i-- {
		if titular == "" || lista[i].Titular == titular {
			out = append(out, lista[i])
		}
	}
	return out, nil
}
