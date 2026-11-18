package bdd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"prj_withdrawal/banco"
)

// contexto guarda el estado de un escenario mientras se ejecutan sus pasos.
type contexto struct {
	cuenta *banco.Cuenta
	log    *banco.BitacoraMemoria
	tx     banco.Transaccion
	err    error
}

type definicion struct {
	re *regexp.Regexp
	fn func(*contexto, []string) error
}

// Step definitions: cada expresión regular corresponde a una frase de los .feature.
var definiciones = []definicion{
	{regexp.MustCompile(`^(\w+) has (-?[\d.]+) in their account$`), dadoSaldo},
	{regexp.MustCompile(`^the transaction log cannot be written$`), dadoLogFalla},
	{regexp.MustCompile(`^(\w+) has already withdrawn ([\d.]+) today$`), dadoRetiradoHoy},
	{regexp.MustCompile(`^(\w+) (?:tries to )?(withdraws?|deposits?) (-?[\d.]+)(?: in cash)?$`), cuandoOpera},
	{regexp.MustCompile(`^the (withdrawal|deposit) is (successful|declined)$`), entoncesResultado},
	{regexp.MustCompile(`^the balance (?:is|remains) (-?[\d.]+)$`), entoncesSaldo},
	{regexp.MustCompile(`^the logged final balance is (-?[\d.]+)$`), entoncesSaldoLog},
	{regexp.MustCompile(`^a transaction "(\w+)" with status "(\w+)" is logged$`), entoncesRegistro},
	{regexp.MustCompile(`^the system shows the error "(.*)"$`), entoncesError},
	{regexp.MustCompile(`^the operation is rolled back$`), entoncesRollback},
}

func buscarPaso(texto string) (*definicion, []string) {
	for i := range definiciones {
		if m := definiciones[i].re.FindStringSubmatch(texto); m != nil {
			return &definiciones[i], m[1:]
		}
	}
	return nil, nil
}

func monto(s string) (banco.Monto, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return banco.NuevoMonto(f)
}

// --- Given ---

func dadoSaldo(c *contexto, a []string) error {
	m, err := monto(a[1])
	if err != nil {
		return err
	}
	c.cuenta = banco.NewCuenta(a[0], m)
	c.log = &banco.BitacoraMemoria{}
	return nil
}

func dadoLogFalla(c *contexto, _ []string) error {
	if c.log == nil {
		return fmt.Errorf("primero debe existir una cuenta")
	}
	c.log.Falla = true
	return nil
}

// dadoRetiradoHoy deja en el log un retiro EXITOSA de hoy (para RN-10).
// Solo cuenta para el límite diario: el saldo del escenario no cambia.
func dadoRetiradoHoy(c *contexto, a []string) error {
	if c.cuenta == nil || c.cuenta.Titular() != a[0] {
		return fmt.Errorf("no hay cuenta para %s en este escenario", a[0])
	}
	m, err := monto(a[1])
	if err != nil {
		return err
	}
	c.log.Items = append(c.log.Items, banco.Transaccion{
		ID:            fmt.Sprintf("TX-%04d", len(c.log.Items)+1),
		Fecha:         banco.Reloj().Format("2006-01-02 15:04:05"),
		Titular:       a[0],
		Tipo:          banco.TipoRetiro,
		Monto:         banco.MontoEscrito(m.Float()),
		SaldoAnterior: c.cuenta.Saldo() + m,
		SaldoFinal:    c.cuenta.Saldo(),
		Estado:        banco.EstadoExitosa,
		Mensaje:       "operación realizada",
	})
	return nil
}

// --- When ---

func cuandoOpera(c *contexto, a []string) error {
	if c.cuenta == nil || c.cuenta.Titular() != a[0] {
		return fmt.Errorf("no hay cuenta para %s en este escenario", a[0])
	}
	f, err := strconv.ParseFloat(a[2], 64)
	if err != nil {
		return err
	}
	tipo := banco.TipoAbono
	if strings.HasPrefix(a[1], "withdraw") {
		tipo = banco.TipoRetiro
	}
	c.tx, c.err = banco.Ejecutar(c.cuenta, c.log, tipo, f)
	return nil
}

// --- Then ---

func entoncesResultado(c *contexto, a []string) error {
	tipo := map[string]string{"withdrawal": banco.TipoRetiro, "deposit": banco.TipoAbono}[a[0]]
	estado := map[string]string{"successful": banco.EstadoExitosa, "declined": banco.EstadoRechazada}[a[1]]
	if c.tx.Tipo != tipo || c.tx.Estado != estado {
		return fmt.Errorf("se esperaba %s %s y se obtuvo %s %s (%s)", tipo, estado, c.tx.Tipo, c.tx.Estado, c.tx.Mensaje)
	}
	return nil
}

func entoncesSaldo(c *contexto, a []string) error {
	m, err := monto(a[0])
	if err != nil {
		return err
	}
	if c.cuenta.Saldo() != m {
		return fmt.Errorf("saldo %s, se esperaba %s", c.cuenta.Saldo(), m)
	}
	return nil
}

func entoncesSaldoLog(c *contexto, a []string) error {
	m, err := monto(a[0])
	if err != nil {
		return err
	}
	if len(c.log.Items) == 0 || c.log.Items[len(c.log.Items)-1].SaldoFinal != m {
		return fmt.Errorf("el saldo final registrado no es %s", m)
	}
	return nil
}

func entoncesRegistro(c *contexto, a []string) error {
	if len(c.log.Items) != 1 {
		return fmt.Errorf("se esperaba 1 registro en el log y hay %d", len(c.log.Items))
	}
	t := c.log.Items[0]
	if t.Tipo != a[0] || t.Estado != a[1] {
		return fmt.Errorf("registro %s %s, se esperaba %s %s", t.Tipo, t.Estado, a[0], a[1])
	}
	return nil
}

func entoncesError(c *contexto, a []string) error {
	if c.tx.Estado != banco.EstadoError || c.tx.Mensaje != a[0] {
		return fmt.Errorf("se esperaba ERROR \"%s\" y se obtuvo %s \"%s\"", a[0], c.tx.Estado, c.tx.Mensaje)
	}
	return nil
}

func entoncesRollback(c *contexto, _ []string) error {
	if c.err == nil || !strings.Contains(c.err.Error(), "revertida") {
		return fmt.Errorf("se esperaba que la operación se revirtiera (err=%v)", c.err)
	}
	if c.cuenta.Saldo() != c.tx.SaldoAnterior {
		return fmt.Errorf("el saldo cambió de %s a %s", c.tx.SaldoAnterior, c.cuenta.Saldo())
	}
	return nil
}
