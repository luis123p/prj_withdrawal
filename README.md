# prj_withdrawal · Cajero Web (Go)

Proyecto del curso **DSL (Gherkin - BDD)** · UNI FIIS · 2026
**Alumno:** Paucar Ventura, Luis Antonio
**Base:** codeLAB-02 "Withdrawing cash" (Ing. Jorge Medianero Acosta, v1.2.0)

App web de cajero con **retiro**, **abono** y **registro de transacciones en JSON**,
desarrollada de forma iterativa e incremental con BDD (Gherkin) y TDD.

| Iteración | Acápite | Contenido |
|-----------|---------|-----------|
| 1 | 1 | Retiro de efectivo (codeLAB-02, CLI) |
| 2 | 2 | US-01b Abono en efectivo + menú interactivo |
| 3 | 3 | US-04 Integridad de la transacción: log JSON (EXITOSA / RECHAZADA / ERROR) y rollback |
| 4 | 4 | Refactorización a **App Web**: montos en céntimos, servicio `Banco` concurrente, runner Gherkin, API REST + interfaz |

## Requisitos
- Go 1.22 o superior (https://go.dev/dl/). No usa librerías externas.

## Ejecutar

```bash
go run .                 # App web  ->  abrir http://localhost:8080
go run . -puerto 9000    # otro puerto
go run . -bdd            # ejecuta features/*.feature en la terminal
go test ./...            # pruebas unitarias, de API y BDD
```

Modo terminal (de las iteraciones anteriores):

```bash
go run . -i -nombre Alice                    # cajero interactivo
go run . -op retiro -nombre Alice -monto 200 # una operación
go run . -historial                          # ver el log
go run . -datos data/cuentas.json            # cargar cuentas desde archivo
```

Compilar un ejecutable:

```bash
go build -o cajero .          # Linux / macOS  ->  ./cajero
go build -o cajero.exe .      # Windows        ->  cajero.exe
```

## Reglas de negocio
| Regla | Descripción |
|-------|-------------|
| RN-01/02 | No se puede retirar más que el saldo |
| RN-03 | La transacción es atómica: si no se puede registrar, se revierte |
| RN-04 | El monto (retiro o abono) debe ser mayor que cero |
| RN-05 | Abono máximo por operación: 5000.00 |
| RN-06 | Todo intento queda en el log con estado EXITOSA, RECHAZADA o ERROR |
| RN-07 | Los montos tienen como máximo 2 decimales (se guardan en céntimos) |

## API REST
| Método | Ruta | Descripción |
|--------|------|-------------|
| GET  | `/api/cuentas` | Lista de cuentas con su saldo |
| POST | `/api/cuentas` | Crear cuenta `{"titular":"Luis","saldo":500}` |
| POST | `/api/operaciones` | `{"titular":"Alice","tipo":"retiro","monto":200}` |
| GET  | `/api/transacciones?titular=Alice` | Historial (más reciente primero) |
| GET  | `/api/bdd` | Ejecuta los features y devuelve el reporte |

## Estructura
```
prj_withdrawal/
├── main.go               entrada: web (por defecto), -bdd, -i, -op, -historial
├── banco/                lógica de negocio
│   ├── monto.go          Monto en céntimos (int64)
│   ├── cuenta.go         Cuenta: Retirar, Depositar
│   ├── bitacora.go       Transaccion, Bitacora JSON / memoria
│   ├── operaciones.go    Ejecutar (registro + rollback)
│   ├── banco.go          Banco: cuentas + log, seguro con concurrencia
│   └── banco_test.go
├── bdd/                  runner Gherkin (step definitions con regex)
│   ├── runner.go  pasos.go  bdd_test.go
├── web/                  API HTTP + interfaz (HTML/CSS/JS embebidos)
│   ├── servidor.go  servidor_test.go
│   └── static/  index.html (con el JavaScript incluido)  estilos.css
├── features/             01-retiro-cash, 02-abono-cash, 03-transaccion-log
└── data/                 cuentas.json (el log se crea al usar la app)
```

## Publicar en GitHub
```bash
git init
git add .
git commit -m "prj_withdrawal v2.0: cajero web con BDD"
git branch -M main
git remote add origin https://github.com/USUARIO/prj_withdrawal.git
git push -u origin main
```
