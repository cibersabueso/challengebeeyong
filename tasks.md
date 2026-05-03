# Tareas — Sistema de Reservas de Inventario

Este documento descompone el `plan.md` en tareas ordenadas y verificables. Cada tarea cita los criterios de aceptación de `spec.md` que implementa, declara sus dependencias respecto a otras tareas y especifica cómo se verifica que está completa.

Convención de commits: cada commit de implementación cita el ID de la tarea en el mensaje (`feat(backend): T-007 atomic decrement of available stock`).

## Resumen

| Bloque | Tasks | Propósito |
|--------|-------|-----------|
| Backend — Infra y plataforma | T-001 a T-006 | Go module, config, pool de DB, migraciones, seed inicial |
| Backend — Dominio y repositorios | T-007 a T-014 | Structs de dominio, errores tipados, queries SQL atómicas |
| Backend — Servicio e idempotencia | T-015 a T-020 | Lógica de negocio, idempotency store con hash y replay |
| Backend — HTTP handler y middleware | T-021 a T-026 | Routing chi, validación de headers, mapping de errores |
| Backend — Goroutine de TTL | T-027 a T-029 | Expiry service, bootstrap cleanup |
| Backend — Tests | T-030 a T-034 | 4 tests obligatorios + adicionales |
| Frontend — Setup | T-035 a T-038 | Vite, TypeScript, Tailwind, TanStack Query |
| Frontend — API y hooks | T-039 a T-043 | Cliente HTTP tipado, hooks de queries y mutations |
| Frontend — Componentes | T-044 a T-049 | Dashboard, ItemCard, ReservationsPanel, Toast |
| Frontend — Tests | T-050 a T-052 | 3 tests obligatorios |
| Cierre | T-053 a T-055 | README, verificación end-to-end, push final |

Total: 55 tareas.

## Backend — Infra y plataforma

### T-001: Initialize Go module and project layout

**Capa**: backend/infra
**Implementa**: infra
**Dependencias**: ninguna
**Estimado**: 10 min
**Verificación**:
- `cd backend && go mod init github.com/cibersabueso/challengebeeyong/backend` ejecuta sin error.
- La estructura `cmd/api/`, `internal/{config,domain,repository,service,handler,platform}/`, `migrations/`, `seed/` existe.

### T-002: Add core dependencies (chi, pgx, migrate, uuid)

**Capa**: backend/infra
**Implementa**: infra
**Dependencias**: T-001
**Estimado**: 5 min
**Verificación**:
- `go get github.com/go-chi/chi/v5 github.com/jackc/pgx/v5 github.com/golang-migrate/migrate/v4 github.com/google/uuid` se completa.
- `go.mod` lista las cuatro dependencias en sus versiones más recientes estables.

### T-003: Implement config loader from environment variables

**Capa**: backend/infra
**Implementa**: infra
**Dependencias**: T-001
**Estimado**: 15 min
**Verificación**:
- `internal/config/config.go` expone `Load() (*Config, error)` que lee `DATABASE_URL`, `PORT`, `EXPIRY_INTERVAL_SECONDS`, `LOG_LEVEL`.
- Valores faltantes obligatorios producen un error explícito.

### T-004: Implement Postgres pool factory in platform package

**Capa**: backend/infra
**Implementa**: infra
**Dependencias**: T-002, T-003
**Estimado**: 15 min
**Verificación**:
- `internal/platform/postgres.go` expone `NewPool(ctx, cfg) (*pgxpool.Pool, error)`.
- Conexión exitosa con la DB local `challengebeeyong_dev`.
- `Ping(ctx)` retorna nil al arranque.

### T-005: Write initial migration (items, reservations, idempotency_keys)

**Capa**: backend/infra
**Implementa**: infra (sustenta AC-002, AC-003, AC-009, AC-013)
**Dependencias**: T-004
**Estimado**: 25 min
**Verificación**:
- `migrations/001_init.up.sql` crea las tres tablas con los CHECK constraints declarados en `spec.md` sección 8.
- `migrations/001_init.down.sql` hace `DROP TABLE` en orden inverso.
- Índices `(status, expires_at)` y `(user_id, status)` creados en `reservations`.

### T-006: Run migrations on startup and seed initial items

**Capa**: backend/infra
**Implementa**: infra (cumple "PostgreSQL seed data for reviewing purposes" del enunciado)
**Dependencias**: T-005
**Estimado**: 20 min
**Verificación**:
- `cmd/api/main.go` corre las migraciones antes de servir requests.
- `seed/seed.sql` contiene 6 items alineados con el mockup del reto: Vintage Camera (total 20), Mechanical Watch (total 10), Acoustic Guitar (total 16), Smart Flask (total 20), Running Shoes (total 12), Gaming Mouse (total 15).
- Tras el primer arranque, `SELECT COUNT(*) FROM items` retorna 6.

## Backend — Dominio y repositorios

### T-007: Define domain types (Item, Reservation, Status enum)

**Capa**: backend/domain
**Implementa**: AC-001, AC-002, AC-009
**Dependencias**: T-001
**Estimado**: 10 min
**Verificación**:
- `internal/domain/item.go` y `internal/domain/reservation.go` exponen los structs con tags JSON.
- `Status` es un tipo string con constantes `StatusActive`, `StatusReleased`, `StatusExpired`.

### T-008: Define typed domain errors

**Capa**: backend/domain
**Implementa**: AC-003 a AC-021 (lado de errores)
**Dependencias**: T-007
**Estimado**: 10 min
**Verificación**:
- `internal/domain/errors.go` define `ErrOutOfStock`, `ErrIdempotencyConflict`, `ErrReservationNotFound`, `ErrReservationExpired`, `ErrItemNotFound`, `ErrInvalidQuantity`, `ErrInvalidUserID`, `ErrMissingIdempotencyKey`, `ErrInvalidIdempotencyKey`, `ErrInvalidRequestBody`, `ErrInvalidItemID`, `ErrInvalidReservationID`.
- Cada error implementa `Error() string` y es comparable con `errors.Is`.

### T-009: Implement ItemRepository.ListAll

**Capa**: backend/repository
**Implementa**: AC-001
**Dependencias**: T-004, T-007
**Estimado**: 10 min
**Verificación**:
- `repo.ListAll(ctx)` retorna `[]Item` con `available = total - reserved` calculado en SQL.
- Test unitario con 2 items seed retorna 2 items con valores correctos.

### T-010: Implement ItemRepository.AtomicDecrementAvailable

**Capa**: backend/repository
**Implementa**: AC-002, AC-003, AC-008, AC-018, AC-020
**Dependencias**: T-007, T-008
**Estimado**: 25 min
**Verificación**:
- Query: `UPDATE items SET reserved = reserved + $1 WHERE id = $2 AND (total - reserved) >= $1 RETURNING id, name, total, reserved, total - reserved AS available`.
- Si `RowsAffected() == 0` retorna `ErrOutOfStock`.
- Test unitario con item de total=10, reserved=8 y request quantity=5 retorna `ErrOutOfStock`.

### T-011: Implement ReservationRepository.Insert

**Capa**: backend/repository
**Implementa**: AC-002
**Dependencias**: T-007
**Estimado**: 10 min
**Verificación**:
- Inserta una reserva con `status='active'` y `expires_at = NOW() + INTERVAL '60 seconds'`.
- Retorna la reserva creada con `id` generado por `gen_random_uuid()` o por aplicación.

### T-012: Implement ReservationRepository.AtomicReleaseByOwner

**Capa**: backend/repository
**Implementa**: AC-009, AC-011, AC-012, AC-021
**Dependencias**: T-007, T-008
**Estimado**: 25 min
**Verificación**:
- Query: `UPDATE reservations SET status='released', released_at=NOW() WHERE id=$1 AND user_id=$2 AND status='active' RETURNING quantity, item_id`.
- Si `RowsAffected() == 0`: hace SELECT para distinguir entre `ErrReservationNotFound` (no existe o no pertenece al usuario), `released` (devuelve marker `already_released`) y `expired` (`ErrReservationExpired`).
- Test concurrente: 50 DELETE paralelos sobre la misma reserva → 1 release exitoso, 49 marcadores `already_released`.

### T-013: Implement IdempotencyRepository (Lookup, Persist, PurgeExpired)

**Capa**: backend/repository
**Implementa**: AC-005, AC-006, AC-007
**Dependencias**: T-007, T-008
**Estimado**: 25 min
**Verificación**:
- `Lookup(ctx, key) (request_hash, status, body, found)` retorna hit/miss correctamente.
- `Persist(ctx, key, hash, reservation_id, status, body)` graba con PK violation → retry-safe.
- `PurgeExpired(ctx, retention)` borra entradas con `created_at < NOW() - retention`.

### T-014: Implement ReservationRepository.ExpireBatch

**Capa**: backend/repository
**Implementa**: AC-013, AC-019
**Dependencias**: T-007, T-008
**Estimado**: 20 min
**Verificación**:
- Query única con CTE: marca como `expired` y devuelve stock en una sola transacción.
- Test: insertar reserva con `expires_at` en el pasado, ejecutar `ExpireBatch`, verificar que `status='expired'` y que `items.reserved` decrementó.

## Backend — Servicio e idempotencia

### T-015: Implement payload canonical hashing

**Capa**: backend/service
**Implementa**: AC-006, AC-007
**Dependencias**: T-008
**Estimado**: 15 min
**Verificación**:
- Función `canonicalHash(payload []byte) string` produce SHA-256 hex de JSON con keys ordenadas alfabéticamente y sin whitespace.
- Test: `{"a":1,"b":2}` y `{"b":2,"a":1}` (con espacios distintos) producen el mismo hash.

### T-016: Implement ReservationService.Create

**Capa**: backend/service
**Implementa**: AC-002, AC-003, AC-005, AC-006, AC-007, AC-008, AC-017, AC-018, AC-020
**Dependencias**: T-010, T-011, T-013, T-015
**Estimado**: 40 min
**Verificación**:
- Orquesta dentro de UNA transacción: lookup idempotency → atomic decrement → insert reservation → persist idempotency result.
- Maneja conflict en `idempotency_keys` con retry-on-lookup.
- Test unitario cubre los 4 casos: nuevo, replay con mismo hash, replay con hash distinto, out of stock.

### T-017: Implement ReservationService.Release

**Capa**: backend/service
**Implementa**: AC-009, AC-010, AC-011, AC-012, AC-014, AC-021
**Dependencias**: T-012
**Estimado**: 25 min
**Verificación**:
- Wrapper sobre `repo.AtomicReleaseByOwner`.
- Mapea los 4 casos: released, already_released, expired, not_found.
- Test: las 4 ramas tienen test dedicado.

### T-018: Implement ReservationService.ListByUser

**Capa**: backend/service
**Implementa**: AC-015
**Dependencias**: T-007
**Estimado**: 10 min
**Verificación**:
- Query: `SELECT * FROM reservations WHERE user_id=$1 AND status='active' ORDER BY created_at DESC`.
- Test: 3 reservas de 2 usuarios distintos → solo retorna las del usuario filtrado.

### T-019: Implement ItemService.List

**Capa**: backend/service
**Implementa**: AC-001
**Dependencias**: T-009
**Estimado**: 5 min
**Verificación**:
- Wrapper trivial sobre `repo.ListAll`. Existe para mantener consistencia de capas.

### T-020: Wire structured logging on all stock mutations

**Capa**: backend/service
**Implementa**: RNF-05
**Dependencias**: T-016, T-017, T-014
**Estimado**: 15 min
**Verificación**:
- Cada mutación emite `slog.InfoContext(ctx, "stock_mutation", "reservation_id", ..., "item_id", ..., "delta", ..., "cause", ...)`.
- Causas posibles: `reservation_create`, `reservation_release`, `ttl_expired`.

## Backend — HTTP handler y middleware

### T-021: Implement chi router with middleware chain (request_id, logger, recoverer)

**Capa**: backend/handler
**Implementa**: infra
**Dependencias**: T-002, T-020
**Estimado**: 15 min
**Verificación**:
- `cmd/api/main.go` instancia chi.Router con los 3 middleware.
- `GET /healthz` responde 200 OK.

### T-022: Implement validation middleware for X-User-Id

**Capa**: backend/handler
**Implementa**: AC-016, AC-021, sección 6.1, sección 8.1
**Dependencias**: T-021, T-008
**Estimado**: 15 min
**Verificación**:
- Middleware lee `X-User-Id`, valida que sea UUID v4 estricto, lo expone vía `context.Context`.
- Si falta o es inválido: 400 con `INVALID_USER_ID`.

### T-023: Implement GET /items handler

**Capa**: backend/handler
**Implementa**: AC-001
**Dependencias**: T-019, T-021
**Estimado**: 10 min
**Verificación**:
- Llama a `service.List` y serializa a JSON con shape de `Item`.
- `curl http://localhost:8080/api/v1/items` retorna array de 6 items tras seed.

### T-024: Implement POST /reservations handler with full validation pipeline

**Capa**: backend/handler
**Implementa**: AC-002 a AC-008, AC-017, AC-018, AC-020, sección 6.1
**Dependencias**: T-016, T-022
**Estimado**: 35 min
**Verificación**:
- Aplica el orden estricto de validación de `spec.md` 6.1.
- Cada error de dominio mapea al status code documentado en `openapi.yaml`.
- Test de integración cubre los 9 caminos de validación.

### T-025: Implement DELETE /reservations/{id} handler

**Capa**: backend/handler
**Implementa**: AC-009 a AC-014, AC-021
**Dependencias**: T-017, T-022
**Estimado**: 20 min
**Verificación**:
- Valida `id` como UUID v4.
- Mapea release/already_released/expired/not_found a 200 / 200 / 410 / 404 respectivamente.

### T-026: Implement GET /reservations handler

**Capa**: backend/handler
**Implementa**: AC-015, AC-016
**Dependencias**: T-018, T-022
**Estimado**: 10 min
**Verificación**:
- Llama a `service.ListByUser` con el `user_id` del context.
- Retorna array filtrado.

## Backend — Goroutine de TTL

### T-027: Implement ExpiryService.RunOnce (single batch pass)

**Capa**: backend/expiry
**Implementa**: AC-013, AC-019, EC-02
**Dependencias**: T-014, T-013
**Estimado**: 15 min
**Verificación**:
- `RunOnce(ctx)` ejecuta `repo.ExpireBatch` y `repo.PurgeExpired` con retention de 24h.
- Logs estructurados con cantidad de reservas expiradas y keys purgadas.

### T-028: Wire ExpiryService loop in main.go

**Capa**: backend/expiry
**Implementa**: AC-013
**Dependencias**: T-027
**Estimado**: 15 min
**Verificación**:
- Goroutine corre `RunOnce` cada `EXPIRY_INTERVAL_SECONDS` (default 5).
- Termina limpiamente con `context.Cancel()` ante SIGTERM.

### T-029: Implement bootstrap cleanup at startup

**Capa**: backend/expiry
**Implementa**: EC-06
**Dependencias**: T-027
**Estimado**: 5 min
**Verificación**:
- `main.go` llama a `expiry.RunOnce(ctx)` UNA VEZ después de migraciones y antes de iniciar HTTP.
- Test: insertar reserva con `expires_at` en el pasado en DB, arrancar app, verificar que ya está `expired` antes de servir requests.

## Backend — Tests

### T-030: Test concurrencia 50+ goroutines por última unidad

**Capa**: backend/tests
**Implementa**: AC-008 (variante 50)
**Dependencias**: T-024, T-029
**Estimado**: 25 min
**Verificación**:
- `go test -run TestConcurrency_50_LastUnit -race ./...` pasa con: 1 reserva exitosa, 49 OUT_OF_STOCK, available final = 0.

### T-031: Test concurrencia 100 requests por 10 unidades

**Capa**: backend/tests
**Implementa**: AC-008
**Dependencias**: T-024, T-029
**Estimado**: 25 min
**Verificación**:
- `go test -run TestConcurrency_100_over_10 -race ./...` pasa con: exactamente 10 success, 90 OUT_OF_STOCK, available final = 0, sin negative stock.

### T-032: Test idempotencia POST concurrente con misma key

**Capa**: backend/tests
**Implementa**: AC-006
**Dependencias**: T-024
**Estimado**: 20 min
**Verificación**:
- `go test -run TestIdempotency_PostConcurrent -race ./...` pasa: 2 requests paralelos con misma `Idempotency-Key` → 1 reserva creada, stock decrementado UNA VEZ.

### T-033: Test idempotencia DELETE concurrente

**Capa**: backend/tests
**Implementa**: AC-011, AC-012
**Dependencias**: T-025
**Estimado**: 20 min
**Verificación**:
- `go test -run TestIdempotency_DeleteConcurrent -race ./...` pasa: 50 DELETE paralelos → 1 con `released`, 49 con `already_released`, stock devuelto UNA VEZ.

### T-034: Test bootstrap cleanup y cross-user DELETE

**Capa**: backend/tests
**Implementa**: AC-021, EC-06
**Dependencias**: T-025, T-029
**Estimado**: 15 min
**Verificación**:
- `go test -run TestBootstrapCleanup ./...` pasa.
- `go test -run TestCrossUserDelete ./...` pasa retornando 404 al usuario no dueño.

## Frontend — Setup

### T-035: Bootstrap Vite + React + TypeScript project

**Capa**: frontend/setup
**Implementa**: infra
**Dependencias**: ninguna
**Estimado**: 10 min
**Verificación**:
- `npm create vite@latest frontend -- --template react-ts` completado.
- `tsconfig.json` con `strict: true`.
- `npm run dev` levanta el servidor en `http://localhost:5173`.

### T-036: Install Tailwind CSS

**Capa**: frontend/setup
**Implementa**: infra
**Dependencias**: T-035
**Estimado**: 10 min
**Verificación**:
- `tailwind.config.js` y `postcss.config.js` creados.
- `src/styles/index.css` importa las directivas de Tailwind.
- Una clase `bg-blue-500` aplica color al render.

### T-037: Install TanStack Query and configure provider

**Capa**: frontend/setup
**Implementa**: RNF-04
**Dependencias**: T-035
**Estimado**: 10 min
**Verificación**:
- `@tanstack/react-query` instalado.
- `main.tsx` envuelve `<App />` con `QueryClientProvider`.
- `QueryClient` configurado con `retry: 2, retryDelay: attempt => Math.min(2000 * 2 ** attempt, 8000)`.

### T-038: Install Vitest and Testing Library

**Capa**: frontend/setup
**Implementa**: infra
**Dependencias**: T-035
**Estimado**: 10 min
**Verificación**:
- `vitest`, `@testing-library/react`, `@testing-library/user-event`, `@testing-library/jest-dom` instalados.
- `vite.config.ts` configurado con `test.environment: 'jsdom'`.
- `npm test` corre sin tests y termina exitosamente.

## Frontend — API y hooks

### T-039: Implement typed fetch client with default headers

**Capa**: frontend/api
**Implementa**: infra
**Dependencias**: T-035
**Estimado**: 20 min
**Verificación**:
- `src/api/client.ts` expone `apiFetch<T>(path, init)` que inyecta `X-User-Id` y `Content-Type`.
- Maneja 4xx/5xx parseando `ErrorResponse` y arrojando un error tipado con `code` y `message`.

### T-040: Implement useUserId hook with localStorage persistence

**Capa**: frontend/hooks
**Implementa**: AC-016
**Dependencias**: T-035
**Estimado**: 10 min
**Verificación**:
- `useUserId()` retorna un UUID v4. En el primer mount lo genera con `crypto.randomUUID()` y lo persiste.
- En mounts posteriores devuelve el mismo valor.

### T-041: Implement useItems and useMyReservations queries

**Capa**: frontend/hooks
**Implementa**: AC-001, AC-015
**Dependencias**: T-037, T-039, T-040
**Estimado**: 20 min
**Verificación**:
- Ambos hooks usan `refetchInterval: 2000`.
- `useItems` retorna `Item[]`. `useMyReservations` retorna `Reservation[]`.
- Sin lifelock cuando el server retorna error 5xx (backoff aplica).

### T-042: Implement useCreateReservation mutation

**Capa**: frontend/hooks
**Implementa**: AC-002 a AC-008
**Dependencias**: T-037, T-039, T-040
**Estimado**: 20 min
**Verificación**:
- Genera `Idempotency-Key` con `crypto.randomUUID()` por invocación.
- En success invalida `['items']` y `['reservations']` queries.
- En error 409 expone el código `OUT_OF_STOCK` para que el componente muestre el toast correcto.

### T-043: Implement useReleaseReservation mutation y useCountdown hook

**Capa**: frontend/hooks
**Implementa**: AC-009 a AC-014, US-03
**Dependencias**: T-037, T-039
**Estimado**: 20 min
**Verificación**:
- `useReleaseReservation` invalida queries en success.
- `useCountdown(expiresAt)` retorna segundos restantes; al llegar a 0 emite `null` o `0` consistente.

## Frontend — Componentes

### T-044: Implement layout principal del dashboard (App + StatusBadge)

**Capa**: frontend/components
**Implementa**: US-01, RNF-04
**Dependencias**: T-041, T-036
**Estimado**: 20 min
**Verificación**:
- `App.tsx` contiene header con título, badge "Live" y botón Refresh.
- Layout responde al mockup del reto (zona izquierda inventario, zona derecha reservas).

### T-045: Implement InventoryGrid + ItemCard components

**Capa**: frontend/components
**Implementa**: US-01, AC-001, EC-07
**Dependencias**: T-041, T-042
**Estimado**: 30 min
**Verificación**:
- Grid de 3 columnas en desktop, responsive a 2 y 1 columna en pantallas chicas.
- Cada `ItemCard` muestra inicial, nombre, barra de progreso de stock, contador "X / Y Available", botón Reserve Item.
- Si `available === 0` el botón se deshabilita y muestra "Out of Stock".

### T-046: Implement ReservationsPanel + ReservationItem components

**Capa**: frontend/components
**Implementa**: US-03, AC-015
**Dependencias**: T-041, T-043
**Estimado**: 25 min
**Verificación**:
- Panel lateral derecho lista las reservas activas del usuario.
- Cada entrada muestra nombre del item, ID corto (ej. `#RES-789`), countdown grande (`MM:SS`), unidades reservadas, botón Release.

### T-047: Implement Toast component for conflict feedback

**Capa**: frontend/components
**Implementa**: US-05, AC-003, EC-07
**Dependencias**: T-042
**Estimado**: 15 min
**Verificación**:
- Toast aparece arriba a la derecha cuando una mutación retorna error con código `OUT_OF_STOCK`.
- Texto: "Item Taken — Sorry, the {ItemName} was just reserved by another user".
- Auto-dismiss en 4 segundos.

### T-048: Implement quantity selector inside ItemCard

**Capa**: frontend/components
**Implementa**: AC-002, AC-004
**Dependencias**: T-045
**Estimado**: 15 min
**Verificación**:
- Stepper o input numérico con min=1, max=available.
- Botón Reserve invoca la mutation con el valor seleccionado.

### T-049: Wire error states and loading skeletons

**Capa**: frontend/components
**Implementa**: misc del reto ("Loading and error states")
**Dependencias**: T-041, T-045, T-046
**Estimado**: 20 min
**Verificación**:
- Mientras `isLoading` se muestran skeletons.
- Si la query falla con 5xx, banner global de error con botón Retry.

## Frontend — Tests

### T-050: Unit test useCountdown logic

**Capa**: frontend/tests
**Implementa**: requisito de tests obligatorios del enunciado
**Dependencias**: T-043, T-038
**Estimado**: 15 min
**Verificación**:
- `useCountdown.test.ts` valida: decremento correcto cada segundo, llegada a 0, no decrementa por debajo de 0.
- `npm test useCountdown` pasa.

### T-051: Component test happy path of reserve flow

**Capa**: frontend/tests
**Implementa**: requisito de tests obligatorios del enunciado
**Dependencias**: T-045, T-038
**Estimado**: 20 min
**Verificación**:
- `ItemCard.test.tsx`: renderiza, click en Reserve, mock devuelve 201, verifica que se llamó al hook con la quantity correcta.

### T-052: Component test error state insufficient stock

**Capa**: frontend/tests
**Implementa**: requisito de tests obligatorios del enunciado
**Dependencias**: T-045, T-047, T-038
**Estimado**: 15 min
**Verificación**:
- `ItemCard.error.test.tsx`: mock devuelve 409 OUT_OF_STOCK, verifica que el toast aparece con el mensaje correcto.

## Cierre

### T-053: Write README.md with run instructions, concurrency strategy, LLM rationale

**Capa**: meta
**Implementa**: requisito explícito del enunciado ("README.md explaining the concurrency strategy, how to run the test suite and the LLM used")
**Dependencias**: T-006, T-035
**Estimado**: 30 min
**Verificación**:
- `README.md` cubre: setup local, comandos, estrategia de concurrencia (con la query atómica), TTL, idempotencia, LLM utilizado y por qué.
- Una persona con Go y Postgres instalados puede levantarlo siguiendo solo el README.

### T-054: Update spec-kit-notes.md with Phase 5 commands and any pivots

**Capa**: meta
**Implementa**: requisito explícito del enunciado ("spec-kit-notes.md explaining commands used, assumptions and refinements along with any 'Pivots'")
**Dependencias**: T-053
**Estimado**: 15 min
**Verificación**:
- Sección 6 (comandos) actualizada con los prompts relevantes de Fase 5.
- Sección 5 (pivots) refleja cualquier desvío real del plan original.

### T-055: End-to-end smoke test and final push

**Capa**: meta
**Implementa**: integración completa
**Dependencias**: todas las anteriores
**Estimado**: 20 min
**Verificación**:
- Backend y frontend levantan sin errores.
- Flow manual: crear reserva, ver countdown, liberar, ver desaparecer del panel.
- Flow manual: agotar stock con N reservas, verificar "Out of Stock" en UI.
- `git log --oneline` muestra historia completa Spec Kit Workflow.
- `git push` final exitoso.
