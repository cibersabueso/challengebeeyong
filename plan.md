# Plan — Sistema de Reservas de Inventario

## 1. Resumen ejecutivo

Este documento traduce los criterios de aceptación y edge cases definidos en `specs/reservation/spec.md` a decisiones de arquitectura concretas y enforzables. El sistema se descompone en tres componentes: backend en Go (chi + pgx), frontend en React (TanStack Query + Tailwind) y PostgreSQL como única fuente de verdad. La atomicidad bajo concurrencia se garantiza en la capa de DB mediante un único `UPDATE` condicional con guarda en el `WHERE`, sin locks de aplicación ni `SELECT FOR UPDATE`. El TTL de 60 segundos se gestiona por una goroutine de barrido que corre cada 5 segundos, complementada con una pasada de bootstrap cleanup al arranque del proceso para resiliencia ante reinicios. La idempotencia de `POST /reservations` se persiste en una tabla dedicada (`idempotency_keys`) que cachea la response del 201 inicial junto con un hash SHA-256 del payload normalizado, garantizando que reintentos del cliente nunca produzcan reservas duplicadas.

## 2. Stack definitivo

### Backend (Go)

| Componente | Librería | Versión |
|------------|----------|---------|
| Runtime | Go | 1.22+ (probado con 1.26.2) |
| Router HTTP | github.com/go-chi/chi/v5 | v5.x |
| Driver Postgres | github.com/jackc/pgx/v5 | v5.x |
| Migraciones | github.com/golang-migrate/migrate/v4 | v4.x |
| UUID | github.com/google/uuid | v1.x |
| Logger | log/slog (stdlib) | nativa |
| Tests | testing (stdlib) + httptest | nativa |

### Frontend (React)

| Componente | Librería | Versión |
|------------|----------|---------|
| Runtime | Node | 20.x (probado con 20.20.2) |
| Bundler | Vite | 5.x |
| UI | React | 18.x |
| Lenguaje | TypeScript | 5.x con strict mode |
| Server state | @tanstack/react-query | v5.x |
| HTTP client | fetch nativo (envuelto en helpers tipados) | — |
| Styling | Tailwind CSS | 3.x |
| Tests | Vitest + @testing-library/react + @testing-library/user-event | últimas |

### Base de datos

- PostgreSQL 16 o superior (probado con Postgres.app 18.3 sobre macOS).
- Una base de datos para desarrollo (`challengebeeyong_dev`) y otra para tests (`challengebeeyong_test`).

## 3. Arquitectura del backend

### 3.1 Estructura de carpetas

```
backend/
├── cmd/
│   └── api/
│       └── main.go                  # entrypoint: carga config, abre pool, registra rutas, lanza goroutine de TTL
├── internal/
│   ├── config/
│   │   └── config.go                # carga de variables de entorno (DATABASE_URL, PORT, etc.)
│   ├── domain/
│   │   ├── item.go                  # struct Item
│   │   ├── reservation.go           # struct Reservation, enum Status
│   │   └── errors.go                # errores tipados (ErrOutOfStock, ErrIdempotencyConflict, ...)
│   ├── repository/
│   │   ├── item_repo.go             # consultas a items
│   │   ├── reservation_repo.go      # consultas y mutaciones de reservations
│   │   └── idempotency_repo.go      # consultas y mutaciones de idempotency_keys
│   ├── service/
│   │   ├── reservation_service.go   # orquesta repos, idempotency check, manejo de errores de dominio
│   │   └── expiry_service.go        # goroutine de TTL: pasada periódica + bootstrap cleanup
│   ├── handler/
│   │   ├── items_handler.go         # GET /items
│   │   ├── reservations_handler.go  # POST/GET/DELETE /reservations
│   │   ├── middleware.go            # request_id, logger, recoverer, validación de X-User-Id
│   │   └── errors.go                # mapping de errores de dominio a HTTP status + ErrorResponse
│   └── platform/
│       └── postgres.go              # constructor del pool pgx, helper para correr migraciones
├── migrations/
│   ├── 001_init.up.sql              # tablas items, reservations, idempotency_keys + índices
│   └── 001_init.down.sql            # drop de las 3 tablas
├── seed/
│   └── seed.sql                     # 6 items iniciales para el frontend (alineados con el mockup del reto)
├── go.mod
└── go.sum
```

### 3.2 Capas y responsabilidades

- **handler**: parsing de request, validación de headers y body en el orden definido en `spec.md` sección 6.1, mapping de errores de dominio a HTTP status. NO contiene lógica de negocio.
- **service**: ortogonal al transporte. Recibe DTOs limpios, orquesta repositorios, gestiona idempotencia, devuelve estructuras de dominio o errores tipados. Es la capa testeable de extremo a extremo sin levantar HTTP.
- **repository**: solo SQL. Cada query es una función con un nombre explícito (`AtomicDecrementAvailable`, `MarkExpiredAndReturnStock`, `LookupOrInsertIdempotencyKey`).
- **domain**: tipos puros, sin dependencias externas. Define el modelo conceptual y los errores que el resto de la aplicación usa.
- **platform**: integraciones de bajo nivel (pool de DB, migraciones). No tiene reglas de negocio.

### 3.3 Flujo end-to-end de POST /reservations

```
HTTP request
  │
  ▼
handler/middleware.go
  ├── request_id middleware
  ├── slog logger middleware
  └── valida X-User-Id como UUID v4         → 400 INVALID_USER_ID si falla
  │
  ▼
handler/reservations_handler.go
  ├── valida Idempotency-Key (presente, ≤256, imprimibles)  → 400 si falla
  ├── parsea body                                             → 400 INVALID_REQUEST_BODY si falla
  ├── valida item_id, quantity                                → 400 si falla
  └── llama a service.CreateReservation(...)
  │
  ▼
service/reservation_service.go
  ├── repo.LookupIdempotencyKey(key)
  │     ├── hit con mismo hash    → return cached response (200 OK)
  │     ├── hit con hash distinto → return ErrIdempotencyConflict (422)
  │     └── miss                  → continúa
  ├── repo.AtomicDecrementAvailable(item_id, quantity)
  │     UPDATE items SET reserved = reserved + $qty
  │     WHERE id = $id AND (total - reserved) >= $qty
  │     RETURNING reserved
  │     ├── rowsAffected = 0 → return ErrOutOfStock (409)
  │     └── rowsAffected = 1 → continúa
  ├── repo.InsertReservation(...)
  │     INSERT INTO reservations (id, item_id, user_id, quantity, status,
  │                               expires_at, created_at)
  │     VALUES (..., 'active', NOW() + INTERVAL '60 seconds', NOW())
  ├── repo.PersistIdempotencyResult(key, hash, reservation_id, 201, body)
  └── return Reservation
  │
  ▼
handler maps Reservation → HTTP 201 + JSON
```

Todo el flujo desde `AtomicDecrementAvailable` hasta `PersistIdempotencyResult` corre dentro de **una sola transacción de Postgres** (`BEGIN ... COMMIT`) para garantizar que un fallo en la persistencia de la idempotency key no deje stock decrementado huérfano.

## 4. Arquitectura del frontend

### 4.1 Estructura de carpetas

```
frontend/
├── src/
│   ├── main.tsx                       # entrypoint: monta App con QueryClientProvider
│   ├── App.tsx                        # layout principal del dashboard
│   ├── api/
│   │   ├── client.ts                  # fetch wrapper tipado, headers default (X-User-Id)
│   │   ├── items.ts                   # listItems()
│   │   └── reservations.ts            # createReservation, listMyReservations, releaseReservation
│   ├── hooks/
│   │   ├── useUserId.ts               # lee/genera UUID v4 en localStorage
│   │   ├── useItems.ts                # useQuery con refetchInterval 2000
│   │   ├── useMyReservations.ts       # useQuery con refetchInterval 2000
│   │   ├── useCreateReservation.ts    # useMutation con generación de Idempotency-Key
│   │   ├── useReleaseReservation.ts   # useMutation
│   │   └── useCountdown.ts            # hook puro para countdown 60s → 0s
│   ├── components/
│   │   ├── InventoryGrid.tsx          # grid de cards de items
│   │   ├── ItemCard.tsx               # card individual con barra de stock + botón Reserve
│   │   ├── ReservationsPanel.tsx      # panel lateral con reservas activas + countdown
│   │   ├── ReservationItem.tsx        # entrada individual con countdown y botón Release
│   │   ├── Toast.tsx                  # toast de error (Item Taken, etc.)
│   │   └── StatusBadge.tsx            # indicador "Live" + last refreshed
│   ├── lib/
│   │   ├── uuid.ts                    # crypto.randomUUID() helper + validación v4
│   │   └── errors.ts                  # mapping de error codes del backend a mensajes UI
│   └── styles/
│       └── index.css                  # imports de Tailwind
├── tests/
│   ├── useCountdown.test.ts           # unit test del timer
│   ├── ItemCard.test.tsx              # component test happy path
│   └── ItemCard.error.test.tsx        # component test error state (insufficient stock)
├── index.html
├── package.json
├── tsconfig.json
├── vite.config.ts
└── tailwind.config.js
```

### 4.2 Estado y sincronización

- **User identity**: UUID v4 en `localStorage['user_id']`, generado con `crypto.randomUUID()` en el primer mount.
- **Server state**: TanStack Query con `refetchInterval: 2000` para `useItems` y `useMyReservations`.
- **Optimistic updates**: NO se usan. Toda mutación (reserve, release) invalida las queries afectadas vía `queryClient.invalidateQueries`. Razón: la atomicidad y el conflict handling viven en el backend; ramificar lógica optimista en frontend introduce riesgo de drift.
- **Backoff ante errores 5xx**: TanStack Query lo aplica por defecto (1s, 2s, 4s) con `retry: 3`; sobreescribimos a `retry: 2, retryDelay: attempt => Math.min(2000 * 2 ** attempt, 8000)` para alinearlo con QA-04 del spec.
- **Last refreshed indicator**: `useQuery` expone `dataUpdatedAt`; el badge "Live" lo muestra como tooltip.

### 4.3 Idempotency-Key en el cliente

Cada llamada a `createReservation` genera una `Idempotency-Key` nueva (`crypto.randomUUID()`) y la pasa como header. La key se mantiene durante todo el ciclo de la mutación (incluyendo retries automáticos de TanStack Query) para que reintentos en el cliente no creen reservas duplicadas.

## 5. Mecanismo de concurrencia

### 5.1 Operación crítica: decremento atómico

```sql
UPDATE items
   SET reserved = reserved + $1
 WHERE id = $2
   AND (total - reserved) >= $1
RETURNING id, name, total, reserved, total - reserved AS available;
```

**Por qué es atómica**:

- PostgreSQL serializa internamente los UPDATEs concurrentes sobre la misma fila mediante MVCC y row-level locks implícitos. No se requiere `SELECT FOR UPDATE` ni `BEGIN ISOLATION LEVEL SERIALIZABLE`.
- El predicado `(total - reserved) >= $1` se evalúa contra la versión más reciente de la fila al momento de adquirir el lock implícito. Si dos transacciones intentan decrementar y solo hay stock para una, la segunda verá `(total - reserved) < $1` y no afectará filas (`rowsAffected = 0`).
- En lugar de manejar `available` como columna persistida, se calcula como `total - reserved`. Esto evita drift entre dos columnas que tendrían que mantenerse sincronizadas.

**Detección de overselling en aplicación**:

```go
result, err := tx.Exec(ctx, queryAtomicDecrement, qty, itemID)
if err != nil { return err }
if result.RowsAffected() == 0 {
    return domain.ErrOutOfStock
}
```

### 5.2 Liberación manual atómica

```sql
UPDATE reservations
   SET status = 'released',
       released_at = NOW()
 WHERE id = $1
   AND user_id = $2
   AND status = 'active'
RETURNING quantity, item_id;
```

Si `rowsAffected = 1`: la reserva era activa y se acaba de liberar. Se devuelve stock con un segundo UPDATE atómico sobre `items` dentro de la misma transacción:

```sql
UPDATE items
   SET reserved = reserved - $1
 WHERE id = $2;
```

Si `rowsAffected = 0` en el primer UPDATE: la reserva no existe, no es del usuario, o ya está en estado `released`/`expired`. El handler decide la respuesta:

- Reserva inexistente o de otro usuario → 404 RESERVATION_NOT_FOUND.
- Reserva en estado `released` → 200 OK con `{"status": "already_released"}`.
- Reserva en estado `expired` → 410 RESERVATION_EXPIRED.

Esta diferenciación se hace con un `SELECT` posterior solo cuando rowsAffected = 0, evitando queries extra en el happy path.

### 5.3 Por qué NO usamos `SELECT FOR UPDATE`

- `SELECT FOR UPDATE` toma un lock pesimista que serializa todas las transacciones competidoras hasta el COMMIT. En un escenario flash sale con 100 requests concurrentes, esto reduce el throughput drásticamente.
- El `UPDATE` condicional con guarda en el `WHERE` produce el mismo resultado correcto con mejor performance: las transacciones que pierden la condición no esperan, fallan rápido y devuelven 409 al cliente.
- El principio 3 de la constitución prohíbe locks de aplicación; este patrón también minimiza locks de DB.

## 6. Estrategia de TTL

### 6.1 Goroutine de barrido (`expiry_service`)

Una goroutine se lanza en `main.go` después de las migraciones y antes de servir requests. Su loop:

```
for {
    expireBatch(ctx)        // ejecuta UPDATE atómico de batch
    purgeOldKeys(ctx)       // limpia idempotency_keys con created_at < NOW() - 24h
    select {
    case <-ticker.C:        // cada 5 segundos
        continue
    case <-ctx.Done():      // shutdown limpio
        return
    }
}
```

### 6.2 Query de expiración

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
 WHERE i.id = e.item_id;
```

Una sola transacción, dos UPDATEs encadenados con CTE. Devuelve stock atómicamente, sin posibilidad de doble-devolución.

### 6.3 Bootstrap cleanup

Al arranque del proceso, antes de aceptar requests, se ejecuta una pasada de `expireBatch` para procesar todas las reservas vencidas durante un eventual downtime. Esto cubre EC-06 documentado en `spec.md`.

### 6.4 Garantía de no doble-devolución

Tanto la liberación manual como la expiración automática usan la misma guarda `WHERE status = 'active'` en el UPDATE. Solo una de las dos operaciones puede afectar la fila; la otra ve `rowsAffected = 0` y no devuelve stock. Esto resuelve AC-019 (race entre release y expiración).

## 7. Estrategia de idempotencia

### 7.1 Tabla `idempotency_keys`

Definida en `spec.md` sección 8. Recordatorio del propósito:

- `key` (PK): valor opaco recibido en el header.
- `request_hash`: SHA-256 del payload normalizado (JSON con keys ordenadas alfabéticamente y sin espacios).
- `reservation_id`: FK a la reserva creada (puede ser NULL si la primera respuesta fue un error tipado, aunque el caso de uso principal cachea solo respuestas 201).
- `response_status` y `response_body`: cuerpo cacheado para replay.

### 7.2 Flujo de lookup

```
service recibe (key, payload)
  │
  ▼
hash = sha256(canonical_json(payload))
  │
  ▼
SELECT request_hash, response_status, response_body
  FROM idempotency_keys
 WHERE key = $1
  │
  ├── hit con request_hash == hash       → return cached response (200)
  ├── hit con request_hash != hash       → return ErrIdempotencyConflict (422)
  └── miss                               → continúa con AtomicDecrementAvailable
```

### 7.3 Persistencia transaccional

`InsertReservation` y `PersistIdempotencyResult` corren dentro de la misma transacción que el `UPDATE` atómico de stock. Si cualquier paso falla, la transacción hace rollback y el stock no queda decrementado.

### 7.4 Concurrencia sobre la misma key

Si dos requests con la misma key llegan al mismo instante (AC obligatorio del reto: idempotency test concurrente):

- Ambas calculan el hash y hacen lookup. Ambas ven miss.
- Ambas intentan `AtomicDecrementAvailable`. Una gana, la otra ve `rowsAffected = 0` (si el stock se agotó) o ambas ganan (si había stock).
- Ambas intentan `INSERT INTO idempotency_keys (key, ...)`. La PK constraint bloquea: la primera commitea, la segunda ve `pq: duplicate key value violates unique constraint`.
- La segunda hace lookup de nuevo, ahora ve hit y retorna la respuesta cacheada de la primera.

Esto requiere lógica de retry-on-conflict en el repositorio de idempotency. Es un detalle implementable sin sacrificar atomicidad.

## 8. Estrategia de testing

### 8.1 Tests obligatorios del enunciado (Go)

1. **Concurrencia 50+**: 50 goroutines compiten por el último ítem disponible. Esperado: exactamente 1 success, 49 fallan con OUT_OF_STOCK.
2. **Concurrencia 100/10**: 100 requests por 10 unidades. Esperado: 10 success, 90 OUT_OF_STOCK, available final = 0, sin stock negativo.
3. **Idempotencia POST concurrente**: 2 requests paralelos con la misma Idempotency-Key. Esperado: 1 reserva creada, stock decrementado UNA SOLA VEZ.
4. **Idempotencia DELETE concurrente**: 2 requests paralelos sobre la misma reserva. Esperado: stock devuelto UNA SOLA VEZ.

### 8.2 Tests obligatorios del enunciado (React)

1. **Unit test del timer**: `useCountdown` decrementa correctamente desde 60s a 0s.
2. **Component test happy path**: click en "Reserve Item" llama al hook, muestra success.
3. **Component test error state**: click en "Reserve Item" sobre item sin stock muestra error visible.

### 8.3 Tests adicionales (no obligatorios pero suman a Spec Kit Rigor)

Backend:
- Test de bootstrap cleanup: insertar reserva con `expires_at` en el pasado, arrancar la goroutine, verificar que pasa a `expired` en la primera pasada.
- Test de cross-user DELETE: verificar que retorna 404 (no 403) cuando el X-User-Id no coincide (AC-021).
- Test de idempotency con payload distinto: verificar 422 IDEMPOTENCY_CONFLICT.

Frontend:
- Component test del panel de reservas: cuando el countdown llega a 00:00, la reserva desaparece tras el siguiente refetch.
- Component test del toast "Item Taken": cuando un POST retorna 409, aparece el toast con el mensaje correcto.

### 8.4 Estrategia de DB para tests

DB dedicada `challengebeeyong_test` en la misma instancia de Postgres.app. Cada test suite (cada `_test.go`) ejecuta `TRUNCATE items, reservations, idempotency_keys CASCADE` antes de cada test. Las migraciones se corren una sola vez al inicio de la suite.

**Nota operacional**: en CI/CD se sustituiría por testcontainers-go para garantizar aislamiento total entre runs. La elección de DB local es pragmática para el time budget del reto (8-10 horas) y no compromete correctness.

### 8.5 Aserciones críticas

Todos los tests de concurrencia validan post-conditions sobre el estado de la base de datos, no solo sobre los códigos HTTP individuales. Tras el run:

- `SELECT total, reserved FROM items WHERE id = $id` debe respetar el invariante `0 <= reserved <= total`.
- `SELECT COUNT(*) FROM reservations WHERE item_id = $id AND status = 'active'` debe coincidir con la suma esperada.

## 9. Deployment y ejecución local

### 9.1 Variables de entorno

| Variable | Default | Descripción |
|----------|---------|-------------|
| DATABASE_URL | postgres://enrique@localhost:5432/challengebeeyong_dev?sslmode=disable | Conexión a Postgres |
| PORT | 8080 | Puerto HTTP del backend |
| EXPIRY_INTERVAL_SECONDS | 5 | Intervalo del barrido de TTL |
| LOG_LEVEL | info | Nivel de slog (debug, info, warn, error) |

### 9.2 Pasos de ejecución (documentados en README.md final)

```
# Backend
createdb challengebeeyong_dev
createdb challengebeeyong_test
cd backend
go mod download
go run ./cmd/api

# Frontend (en otra terminal)
cd frontend
npm install
npm run dev

# Tests backend
cd backend
go test ./... -race -count=1

# Tests frontend
cd frontend
npm test
```

### 9.3 Sin Docker

No se utiliza Docker en este reto. La razón es pragmática: el entorno de desarrollo (iMac con Postgres.app) no tiene Docker instalado y agregarlo sumaría 30-45 minutos al setup sin valor para la rúbrica. El proyecto es trivialmente dockerizable: bastaría con un Dockerfile multi-stage para el backend y un nginx que sirva el bundle del frontend.

## 10. Trade-offs y descartes

| Descarte | Por qué no se hizo | Cuándo se haría |
|----------|--------------------|-----------------|
| WebSockets | Polling 2s cumple el requisito; WebSockets agrega manejo de reconexión que sale del scope. | Si el time budget fuera 20+ horas o si se necesitara push real-time bidireccional. |
| testcontainers-go | Requiere Docker en la máquina de tests; el time budget no lo justifica. | En CI/CD con runners que ya tienen Docker. |
| ORM (GORM, sqlx) | El SQL crítico de concurrencia debe ser visible y auditable. Una capa ORM lo oculta. | Para CRUDs simples sin requisitos de concurrencia atómica. |
| pg_cron para TTL | Requiere extensión instalada en Postgres; no es portable. | En proyectos con Postgres administrado donde la extensión esté disponible. |
| Optimistic concurrency con columna `version` | Innecesario: el UPDATE condicional ya garantiza atomicidad sin retry logic. | En sistemas donde las mutaciones no admiten un predicado en el WHERE. |
| Redis para idempotency store | Postgres con índice sobre `key` es suficiente para el throughput esperado. | Ante throughput de >10k req/s donde la latencia de Postgres se vuelva el cuello. |
| JWT/OAuth para autenticación | El reto no lo pide; X-User-Id por header cumple el requisito de "Your Reservations". | Si el sistema fuera multi-tenant o tuviera datos sensibles por usuario. |
| Server-Sent Events (SSE) | Polling 2s es más simple y compatible con cualquier hosting. | Si el frontend necesitara notificaciones push de stock sin polling activo. |
