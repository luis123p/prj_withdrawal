package banco

import (
	"errors"
	"fmt"
	"time"
)

// Reloj permite fijar la hora en las pruebas.
var Reloj = time.Now

const formatoFecha = "2006-01-02 15:04:05"

// ErrRegistro indica que la bitácora no pudo guardar la transacción.
var ErrRegistro = errors.New("no se pudo registrar")

// nuevaTransaccion arma la cabecera de un registro (ID correlativo y fecha).
func nuevaTransaccion(lista []Transaccion, titular, tipo string, monto float64, saldo Monto) Transaccion {
	return Transaccion{
		ID:            fmt.Sprintf("TX-%04d", len(lista)+1),
		Fecha:         Reloj().Format(formatoFecha),
		Titular:       titular,
		Tipo:          tipo,
		Monto:         MontoEscrito(monto),
		SaldoAnterior: saldo,
	}
}

// RetiradoHoy suma los retiros EXITOSA del titular en la fecha de Reloj (RN-10).
func RetiradoHoy(lista []Transaccion, titular string) Monto {
	hoy := Reloj().Format("2006-01-02")
	var total Monto
	for _, t := range lista {
		if NormalizarTitular(t.Titular) != titular || t.Tipo != TipoRetiro || t.Estado != EstadoExitosa || len(t.Fecha) < 10 || t.Fecha[:10] != hoy {
			continue
		}
		if m, err := NuevoMonto(float64(t.Monto)); err == nil {
			total += m
		}
	}
	return total
}

// disponibleHoy es lo que el titular aún puede retirar hoy (nunca negativo).
func disponibleHoy(lista []Transaccion, titular string) Monto {
	return max(LimiteDiarioRetiro-RetiradoHoy(lista, titular), 0)
}

// validarRetiro aplica, en orden, las reglas del retiro previas al saldo:
// monto > 0 (RN-04), 2 decimales (RN-07), múltiplo de 10 (RN-09) y límite diario (RN-10).
func validarRetiro(monto float64, lista []Transaccion, titular string) (Monto, error) {
	if monto <= 0 {
		return 0, ErrRetiroNoPositivo
	}
	m, err := NuevoMonto(monto)
	if err != nil {
		return 0, err
	}
	if m%DenominacionMinima != 0 {
		return 0, ErrNoMultiplo
	}
	if disp := disponibleHoy(lista, titular); m > disp {
		return 0, ErrLimiteDiario(disp)
	}
	return m, nil
}

// Ejecutar hace una operación (retiro o abono) sobre la cuenta y la registra.
// Si el registro falla, restaura el saldo (RN-03: la transacción es atómica).
// El monto llega como decimal tal cual lo escribe el usuario, y así se guarda.
func Ejecutar(c *Cuenta, log Bitacora, tipo string, monto float64) (Transaccion, error) {
	lista, err := log.Listar()
	if err != nil {
		return Transaccion{}, fmt.Errorf("no se puede operar: %w", err)
	}
	t := nuevaTransaccion(lista, c.Titular(), tipo, monto, c.Saldo())

	var (
		m  Monto
		ok bool
	)
	switch tipo {
	case TipoRetiro:
		if m, err = validarRetiro(monto, lista, c.Titular()); err == nil {
			ok, err = c.Retirar(m) // por último, el saldo (RN-01/02)
		}
	case TipoAbono:
		if m, err = NuevoMonto(monto); err == nil {
			ok, err = c.Depositar(m)
		}
	default:
		return t, fmt.Errorf("tipo de operación desconocido: %s", tipo)
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
