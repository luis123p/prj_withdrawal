// Package banco contiene la lógica de negocio: cuentas, montos y transacciones.
//
// Refactorización (acápite 4): los montos se manejan en céntimos (int64) para
// evitar errores de redondeo, como sugiere el modelo de contexto del codeLAB.
// Autor: Paucar Ventura, Luis Antonio
package banco

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

// Monto es una cantidad de dinero en céntimos (1 sol = 100).
type Monto int64

// montoMaximo evita desbordes con valores absurdos (100 millones).
const montoMaximo = 100_000_000 * 100

var ErrDecimales = errors.New("el monto solo puede tener hasta 2 decimales")
var ErrMontoInvalido = errors.New("monto inválido")

// NuevoMonto convierte un número decimal (ej. 234.56) a céntimos.
// No valida el signo: eso lo deciden las reglas de Retirar y Depositar.
func NuevoMonto(f float64) (Monto, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > montoMaximo/100 {
		return 0, ErrMontoInvalido
	}
	c := math.Round(f * 100)
	if math.Abs(f*100-c) > 1e-6 {
		return 0, ErrDecimales
	}
	return Monto(c), nil
}

// M es un atajo para escribir montos fijos en el código y en las pruebas.
func M(f float64) Monto {
	m, err := NuevoMonto(f)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Monto) Float() float64 { return float64(m) / 100 }

func (m Monto) String() string { return fmt.Sprintf("%.2f", m.Float()) }

// En el JSON se guarda como número con 2 decimales (234.56), no en céntimos,
// para que el log se pueda leer fácilmente.
func (m Monto) MarshalJSON() ([]byte, error) { return []byte(m.String()), nil }

func (m *Monto) UnmarshalJSON(b []byte) error {
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	*m = Monto(math.Round(f * 100))
	return nil
}

// MontoEscrito es el monto tal como lo escribió el usuario. Se usa en el log
// para que un monto inválido (10.555, 0.001, 1e300) quede registrado sin
// redondear ni desbordar. Si el monto es válido se muestra con 2 decimales.
type MontoEscrito float64

func (m MontoEscrito) String() string {
	if c, err := NuevoMonto(float64(m)); err == nil {
		return c.String()
	}
	return strconv.FormatFloat(float64(m), 'g', -1, 64)
}

// NaN e Infinito no existen en JSON: se guardan como texto ("NaN", "+Inf").
func (m MontoEscrito) MarshalJSON() ([]byte, error) {
	f := float64(m)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return []byte(strconv.Quote(m.String())), nil
	}
	return []byte(m.String()), nil
}

func (m *MontoEscrito) UnmarshalJSON(b []byte) error {
	s := string(b)
	if u, err := strconv.Unquote(s); err == nil {
		s = u
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*m = MontoEscrito(f)
	return nil
}
