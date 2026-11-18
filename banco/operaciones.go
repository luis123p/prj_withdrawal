package banco

import (
	"errors"
	"fmt"
	"time"
)

// Reloj permite fijar la hora en las pruebas.
var Reloj = time.Now

// ErrRegistro indica que la bitácora no pudo guardar la transacción.
var ErrRegistro = errors.New("no se pudo registrar")

// nuevaTransaccion arma la cabecera de un registro (ID correlativo y fecha).
func nuevaTransaccion(log Bitacora, titular, tipo string, monto float64, saldo Monto) (Transaccion, error) {
	lista, err := log.Listar()
	if err != nil {
		return Transaccion{}, fmt.Errorf("no se puede operar: %w", err)
	}
	return Transaccion{
		ID:            fmt.Sprintf("TX-%04d", len(lista)+1),
		Fecha:         Reloj().Format("2006-01-02 15:04:05"),
		Titular:       titular,
		Tipo:          tipo,
		Monto:         MontoEscrito(monto),
		SaldoAnterior: saldo,
	}, nil
}

// Ejecutar hace una operación (retiro o abono) sobre la cuenta y la registra.
// Si el registro falla, restaura el saldo (RN-03: la transacción es atómica).
// El monto llega como decimal tal cual lo escribe el usuario, y así se guarda.
func Ejecutar(c *Cuenta, log Bitacora, tipo string, monto float64) (Transaccion, error) {
	t, err := nuevaTransaccion(log, c.Titular(), tipo, monto, c.Saldo())
	if err != nil {
		return t, err
	}

	m, err := NuevoMonto(monto)
	ok := false
	if err == nil {
		switch tipo {
		case TipoRetiro:
			ok, err = c.Retirar(m)
		case TipoAbono:
			ok, err = c.Depositar(m)
		default:
			return t, fmt.Errorf("tipo de operación desconocido: %s", tipo)
		}
	}
	switch {
	case err != nil:
		t.Estado, t.Mensaje = EstadoError, err.Error()
	case !ok && tipo == TipoRetiro:
		t.Estado, t.Mensaje = EstadoRechazada, "saldo insuficiente"
	case !ok:
		t.Estado, t.Mensaje = EstadoRechazada, "excede el límite por operación ("+LimiteAbono.String()+")"
	default:
		t.Estado, t.Mensaje = EstadoExitosa, "operación realizada"
	}
	t.SaldoFinal = c.Saldo()

	if errReg := log.Registrar(t); errReg != nil {
		c.saldo = t.SaldoAnterior // rollback
		t.SaldoFinal = t.SaldoAnterior
		return t, fmt.Errorf("transacción revertida, %w: %v", ErrRegistro, errReg)
	}
	return t, nil
}
