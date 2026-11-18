package banco

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	ErrCuentaNoExiste  = errors.New("la cuenta no existe")
	ErrCuentaExiste    = errors.New("la cuenta ya existe")
	ErrTitularVacio    = errors.New("el titular no puede estar vacío")
	ErrTitularInvalido = errors.New("el titular debe tener de 1 a 30 caracteres: letras, números, espacios, punto, guion o apóstrofo")
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
		n := NormalizarTitular(d.Titular)
		b.cuentas[n] = NewCuenta(n, d.Saldo)
	}
	hist, err := log.Listar()
	if err != nil {
		return nil, err
	}
	for _, t := range hist {
		n := NormalizarTitular(t.Titular)
		if c, ok := b.cuentas[n]; ok {
			c.saldo = t.SaldoFinal
		} else {
			b.cuentas[n] = NewCuenta(n, t.SaldoFinal)
		}
	}
	return b, nil
}

// MaxTitular es el largo máximo del nombre del titular (en caracteres).
const MaxTitular = 30

// Empieza con letra o número; luego letras (con ñ y tildes), números,
// espacios, punto, guion o apóstrofo (RN-08).
var titularValido = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{M}\p{N} .'’-]*$`)

// NormalizarTitular recorta los extremos y reduce los espacios repetidos a uno,
// para que "Ana  Paula" y " Ana Paula " sean la misma cuenta.
func NormalizarTitular(titular string) string {
	return strings.Join(strings.Fields(titular), " ")
}

// ValidarTitular normaliza los espacios y revisa el nombre (RN-08).
func ValidarTitular(titular string) (string, error) {
	titular = NormalizarTitular(titular)
	switch {
	case titular == "":
		return "", ErrTitularVacio
	case utf8.RuneCountInString(titular) > MaxTitular || !titularValido.MatchString(titular):
		return "", ErrTitularInvalido
	}
	return titular, nil
}

// AbrirCuenta crea una cuenta nueva con un saldo inicial. La apertura queda
// en el log como transacción APERTURA, así la cuenta se conserva al reiniciar.
func (b *Banco) AbrirCuenta(titular string, saldo float64) error {
	titular, err := ValidarTitular(titular)
	if err != nil {
		return err
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
	lista, err := b.log.Listar()
	if err != nil {
		return fmt.Errorf("no se puede abrir la cuenta: %w", err)
	}
	t := nuevaTransaccion(lista, titular, TipoApertura, saldo, 0)
	t.SaldoFinal, t.Estado, t.Mensaje = m, EstadoExitosa, "cuenta abierta"
	if err := b.log.Registrar(t); err != nil {
		return fmt.Errorf("cuenta no creada, %w: %v", ErrRegistro, err)
	}
	b.cuentas[titular] = NewCuenta(titular, m)
	return nil
}

// Operar hace un retiro o abono sobre la cuenta del titular.
func (b *Banco) Operar(titular, tipo string, monto float64) (Transaccion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.cuentas[NormalizarTitular(titular)]
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
	c, ok := b.cuentas[NormalizarTitular(titular)]
	if !ok {
		return 0, ErrCuentaNoExiste
	}
	return c.Saldo(), nil
}

// EstadoCuenta es una cuenta con su saldo y lo que aún puede retirar hoy (RN-10).
type EstadoCuenta struct {
	Titular       string `json:"titular"`
	Saldo         Monto  `json:"saldo"`
	DisponibleHoy Monto  `json:"disponible_hoy"`
}

// Cuentas devuelve todas las cuentas ordenadas por titular.
func (b *Banco) Cuentas() []EstadoCuenta {
	b.mu.Lock()
	defer b.mu.Unlock()
	lista, _ := b.log.Listar() // si el log no se puede leer, Operar lo informará
	out := make([]EstadoCuenta, 0, len(b.cuentas))
	for _, c := range b.cuentas {
		out = append(out, EstadoCuenta{Titular: c.titular, Saldo: c.saldo, DisponibleHoy: disponibleHoy(lista, c.titular)})
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
	titular = NormalizarTitular(titular)
	out := []Transaccion{}
	for i := len(lista) - 1; i >= 0; i-- {
		if titular == "" || NormalizarTitular(lista[i].Titular) == titular {
			out = append(out, lista[i])
		}
	}
	return out, nil
}
