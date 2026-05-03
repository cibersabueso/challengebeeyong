# Inventory Reservation System — Beeyond Code Challenge

Sistema de reservas atómicas de inventario para escenarios de flash sale, con TTL de 60 segundos, idempotencia obligatoria y cero overselling bajo carga concurrente.

Repositorio público desarrollado bajo el **Spec Kit Workflow** (Architecture First) en cinco fases estrictas: Constitution, Spec, Plan, Tasks e Implementation. Cada fase tiene su commit dedicado y todo el código de implementación traza hasta criterios de aceptación numerados en `specs/reservation/spec.md`.

---

## Stack

**Backend**
- Go 1.22+ (probado con 1.26.2)
- chi v5 (router HTTP)
- jackc/pgx/v5 (driver PostgreSQL)
- golang-migrate/migrate v4 (migraciones embebidas)
- log/slog (stdlib, observabilidad estructurada)
- testing stdlib + race detector

**Frontend**
- React 18.3 + TypeScript 5.6 (strict)
- Vite 5.4
- Tailwind CSS 3.4
- TanStack Query 5 (polling 2s, retry/backoff)
- Vitest 4 + @testing-library/react

**Base de datos**
- PostgreSQL 16+ (probado con Postgres.app 18.3 sobre macOS)
- Una DB para desarrollo (`challengebeeyong_dev`) y otra para tests (`challengebeeyong_test`).

---

## Arquitectura del repositorio

```
challengebeeyong/
├── constitution.md              Principios de ingeniería no negociables (8 principios)
├── specs/reservation/
│   ├── spec.md                  Requisitos, 21 ACs, 8 edge cases, modelo de datos
│   └── openapi.yaml             Contrato HTTP completo (OpenAPI 3.1)
├── plan.md                      Arquitectura, stack, concurrencia, testing
├── tasks.md                     55 tareas T-001 a T-055 con dependencias y verificación
├── spec-kit-notes.md            Bitácora del workflow: asunciones, pivots, comandos
├── backend/
│   ├── cmd/api/main.go          Entrypoint
│   ├── internal/
│   │   ├── config/              Variables de entorno
│   │   ├── domain/              Tipos puros y errores tipados
│   │   ├── repository/          Queries SQL atómicas
│   │   ├── service/             Lógica transaccional + idempotencia
│   │   ├── handler/             HTTP handlers + middleware
│   │   ├── expiry/              Goroutine de TTL + bootstrap cleanup
│   │   ├── platform/            Pool de Postgres
│   │   └── testutil/            Fixtures de tests
│   ├── migrations/              001_init.up.sql + 001_init.down.sql
│   ├── seed/seed.sql            6 items alineados al mockup del reto
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── api/                 Cliente HTTP tipado
│   │   ├── hooks/               useItems, useMyReservations, useCountdown, mutations
│   │   ├── components/          ItemCard, InventoryGrid, ReservationsPanel, Toast
│   │   ├── lib/                 UUID, errors
│   │   └── App.tsx              Dashboard principal
│   ├── index.html
│   └── package.json
└── README.md
```

---

## Estrategia de concurrencia

El requisito central del reto es **prevenir over-reservation incluso bajo carga concurrente**. La rúbrica exige cero overselling con 100 requests simultáneos sobre 10 unidades.

### Decisión: `UPDATE` condicional atómico, sin locks de aplicación

Toda mutación de stock se ejecuta con una sola sentencia SQL:

```sql
UPDATE items
   SET reserved = reserved + $1
 WHERE id = $2
   AND (total - reserved) >= $1
RETURNING id, name, total, reserved, total - reserved AS available;
```

**Por qué es atómica**:

- PostgreSQL serializa internamente los UPDATEs concurrentes sobre la misma fila mediante MVCC y row-level locks implícitos. No se requiere `SELECT FOR UPDATE` ni `BEGIN ISOLATION LEVEL SERIALIZABLE`.
- El predicado `(total - reserved) >= $1` se evalúa contra la versión más reciente de la fila al momento de adquirir el lock implícito. Si dos transacciones intentan decrementar y solo hay stock para una, la segunda verá la guarda en `false` y no afectará filas (`rowsAffected = 0`).
- En Go: si `result.RowsAffected() == 0`, el handler retorna `409 OUT_OF_STOCK` inmediatamente sin reintentos.
- `available` no se persiste como columna; se calcula como `total - reserved` para evitar drift entre dos columnas que tendrían que mantenerse sincronizadas.

### Por qué NO `SELECT FOR UPDATE`

El lock pesimista serializa todas las transacciones competidoras hasta el COMMIT. En 100 requests concurrentes esto reduce el throughput drásticamente. El `UPDATE` condicional con guarda en el `WHERE` produce el mismo resultado correcto con mejor rendimiento: las transacciones que pierden la condición no esperan, fallan rápido y devuelven 409 al cliente.

### TTL gestionado por la base de datos

La columna `expires_at TIMESTAMPTZ` en `reservations` es la fuente de verdad del vencimiento. Una goroutine corre cada 5 segundos ejecutando este CTE atómico:

```sql
WITH expired AS (
    UPDATE reservations
       SET status = 'expired'
     WHERE status = 'active'
       AND expires_at <= NOW()
    RETURNING id, item_id, quantity
)
UPDATE items i
   SET reserved = i.reserved - e.quantity
  FROM expired e
 WHERE i.id = e.item_id
RETURNING e.id;
```

Una sola transacción, dos UPDATEs encadenados, sin posibilidad de doble-devolución de stock. La misma goroutine purga entradas viejas de `idempotency_keys` con retención de 24 horas.

Al arranque, antes de aceptar requests, se ejecuta una pasada sincrónica de cleanup que procesa cualquier reserva vencida durante un eventual downtime (cubre EC-06 documentado en spec.md).

### Idempotencia transaccional

`POST /reservations` requiere el header `Idempotency-Key`. El service ejecuta dentro de UNA SOLA transacción:

1. Lookup en `idempotency_keys` por la key.
2. Hit con mismo hash de payload → return cached response (200 OK).
3. Hit con hash distinto → return 422 IDEMPOTENCY_CONFLICT.
4. Miss → atomic decrement + insert reservation + persist idempotency record.
5. Si el INSERT en `idempotency_keys` choca con la PK constraint (race entre dos goroutines con la misma key), se hace rollback explícito y replay desde la entrada ganadora.

`DELETE /reservations/{id}` es naturalmente idempotente: un UPDATE con guarda `WHERE status = 'active'` permite que solo la primera invocación devuelva stock; las siguientes responden con `{"status": "already_released"}` sin tocar el contador.

---

## Setup y ejecución local

### Pre-requisitos

- Go 1.22 o superior
- Node 20.x
- PostgreSQL 16 o superior corriendo en `localhost:5432`
- Usuario `enrique` (o ajustar `DATABASE_URL` en consecuencia)

### Crear las bases de datos

```bash
createdb challengebeeyong_dev
createdb challengebeeyong_test
psql -d challengebeeyong_dev -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
psql -d challengebeeyong_test -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
```

### Backend

```bash
cd backend
go mod download
go run ./cmd/api
```

El servidor levanta en `http://localhost:8080`. Las migraciones y el seed corren automáticamente al arranque. Healthcheck en `GET /healthz`.

Variables de entorno disponibles:

| Variable | Default | Descripción |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://enrique@localhost:5432/challengebeeyong_dev?sslmode=disable` | Conexión a Postgres |
| `PORT` | `8080` | Puerto HTTP del backend |
| `EXPIRY_INTERVAL_SECONDS` | `5` | Intervalo del barrido de TTL |
| `LOG_LEVEL` | `info` | Nivel de slog (debug, info, warn, error) |

### Frontend

```bash
cd frontend
npm install
npm run dev
```

UI disponible en `http://localhost:5173`. Vite proxy redirige `/api` a `http://localhost:8080`, así que el frontend no necesita configuración de CORS.

---

## Suite de tests

### Backend (Go)

Los 4 tests obligatorios del enunciado más 2 adicionales corren bajo el race detector:

```bash
cd backend
DATABASE_URL_TEST="postgres://enrique@localhost:5432/challengebeeyong_test?sslmode=disable" \
  go test -race -count=1 -v ./internal/service/...
```

Se ejecutan **6 tests** que validan invariantes vía SELECT directo a la base de datos, no solo por status codes:

| Test | Implementa AC | Verificación |
|------|---------------|--------------|
| `TestConcurrency_50_LastUnit` | AC-008 (variante 50) | 50 goroutines, 1 success, 49 conflicts, `items.reserved=1` |
| `TestConcurrency_100_over_10` | AC-008 | 100 goroutines, 10 success, 90 conflicts, `items.reserved=10`, sin stock negativo |
| `TestIdempotency_PostConcurrent` | AC-006 | 2 requests paralelos con misma key, 1 reserva creada, `items.reserved=2` (decremento único) |
| `TestIdempotency_DeleteConcurrent` | AC-011, AC-012 | 50 DELETEs paralelos, 1 released + 49 already_released, stock devuelto una sola vez |
| `TestBootstrapCleanup` | EC-06 | Reserva vencida insertada manualmente queda `expired` tras Bootstrap, stock devuelto |
| `TestCrossUserDelete` | AC-021 | Intruder recibe 404 RESERVATION_NOT_FOUND, sin filtrar ownership |

Runtime típico: ~1.8 segundos. El race detector no reporta data races.

### Frontend (React)

Los 3 tests obligatorios del enunciado más casos de borde:

```bash
cd frontend
npm test
```

Se ejecutan **11 tests**:

- `useCountdown.test.ts`: 5 tests del hook de countdown (decremento, bottom-out, valor pasado, timestamp inválido, `formatCountdown`).
- `ItemCard.test.tsx`: 6 tests del componente principal (happy path, "Reserving..." state, click invoca onReserve con quantity correcta, Out of Stock disabled, clamping de quantity, increment disabled cuando quantity = available).

Runtime típico: ~3 segundos.

---

## LLM utilizado

**Modelo**: Claude Opus 4.7 (Anthropic), accedido vía Claude Code v2.1.126 desde el panel integrado de VS Code.

**Razones técnicas**:

1. **Coherencia con el Spec Kit Workflow**: Claude Code lee los `.md` del repositorio como contexto persistente. Eso permite que la constitución, el spec, el plan y el spec-kit-notes actúen como fuente de verdad para el agente, no como prompts improvisados sesión a sesión.
2. **Razonamiento sobre concurrencia**: la pieza crítica del reto (atomic decrement con `UPDATE ... WHERE available >= $qty`, manejo de Idempotency-Key concurrente, goroutine de TTL con CTE atómica) requiere un modelo fuerte para diseñar invariantes correctamente. Opus 4.7 produjo decisiones consistentes con la documentación oficial de PostgreSQL y patrones de Stripe y Shopify para inventory.
3. **Human-in-the-loop riguroso**: el agente generó borradores, pero cada edit fue revisado y validado antes de commitear. El criterio arquitectónico es del autor; el agente acelera la redacción y la implementación, no sustituye la decisión.
4. **Soporte nativo en VS Code**: la integración Claude Code → archivos del workspace evita copy-paste manual entre chat y editor, reduciendo errores de transcripción en archivos largos como `openapi.yaml` (432 líneas) y `spec.md` (364 líneas).

Para más detalle sobre cómo se usó el agente en cada fase y las decisiones de pivot, ver `spec-kit-notes.md`.

---

## Estructura de los commits y trazabilidad

El repositorio se construye en una secuencia estricta de fases. Cada commit cumple uno de tres patrones:

- `chore:` para setup y configuración.
- `docs(<scope>):` para documentos de planificación.
- `feat(<layer>): T-XXX summary` para implementación referenciando una task ID.
- `test(<layer>): T-XXX summary` para test suites.

Esto permite trazar cualquier línea de código de vuelta a:

```
commit  →  task ID en tasks.md
task    →  AC IDs en spec.md
AC      →  edge case o requisito en spec.md
        →  principio en constitution.md
```

Para ver el historial completo:

```bash
git log --oneline
```

---

## Notas de operación

- **Polling vs WebSockets**: el frontend usa polling cada 2 segundos. El reto admite cualquiera de los dos; la decisión está justificada en `plan.md` sección 10.
- **CORS**: no se configura porque Vite proxea las llamadas. En despliegue real se debería agregar middleware de CORS al backend.
- **Sin Docker**: el setup es nativo (Go binario + Postgres.app + Vite dev server). El proyecto es trivialmente dockerizable y se documenta en `plan.md` sección 9.3.
- **Time budget**: el enunciado sugiere 8 a 10 horas de trabajo focalizado, con un máximo absoluto de 12. El proyecto se desarrolló respetando esa ventana usando Claude Code como acelerador de redacción e implementación bajo revisión humana.
