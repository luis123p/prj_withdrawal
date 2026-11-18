package banco

import (
	"errors"
	"fmt"
)

const (
	// LimiteAbono es el monto máximo por operación de abono (RN-05).
	LimiteAbono = Monto(5000_00)
	// DenominacionMinima: el cajero solo entrega billetes, el menor es de S/ 10 (RN-09).
	DenominacionMinima = Monto(10_00)
	// LimiteDiarioRetiro es lo máximo que un titular puede retirar por día (RN-10).
	LimiteDiarioRetiro = Monto(2000_00)
)

var (
	ErrRetiroNoPositivo = errors.New("el monto a retirar debe ser mayor que cero")
	ErrAbonoNoPositivo  = errors.New("el monto a abonar debe ser mayor que cero")
	ErrNoMultiplo       = errors.New("el monto a retirar debe ser múltiplo de S/ " + DenominacionMinima.String())
)

// ErrLimiteDiario arma el mensaje de RN-10 con lo que aún se puede retirar hoy.
func ErrLimiteDiario(disponible Monto) error {
	return fmt.Errorf("excede el límite diario de retiro (S/ %s; disponible hoy: S/ %s)", LimiteDiarioRetiro, disponible)
}

// Cuenta es la entidad de negocio de los features Gherkin.
type Cuenta struct {
	titular string
	saldo   Monto
}

func NewCuenta(titular string, saldoInicial Monto) *Cuenta {
	return &Cuenta{titular: titular, saldo: saldoInicial}
}

func (c *Cuenta) Saldo() Monto    { return c.saldo }
func (c *Cuenta) Titular() string { return c.titular }
func (c *Cuenta) String() string  { return fmt.Sprintf("Cuenta{%s, saldo=%s}", c.titular, c.saldo) }

// Retirar aplica RN-01/RN-02 (no retirar más que el saldo) y US-03 (monto > 0).
// Devuelve (false, nil) si se rechaza por saldo insuficiente.
func (c *Cuenta) Retirar(m Monto) (bool, error) {
	if m <= 0 {
		return false, ErrRetiroNoPositivo
	}
	if m > c.saldo {
		return false, nil
	}
	c.saldo -= m
	return true, nil
}

// Depositar aplica RN-04 (monto > 0) y RN-05 (límite por operación).
func (c *Cuenta) Depositar(m Monto) (bool, error) {
	if m <= 0 {
		return false, ErrAbonoNoPositivo
	}
	if m > LimiteAbono {
		return false, nil
	}
	c.saldo += m
	return true, nil
}
