# Especificación — Sistema de Reservas de Inventario

## 1. Resumen

El sistema implementa un mecanismo de reservas temporales de inventario diseñado para escenarios de flash sale, donde múltiples compradores compiten por un stock limitado en ventanas cortas de tiempo. Su responsabilidad central es bloquear N unidades de un item por un TTL fijo de 60 segundos, garantizando atomicidad bajo concurrencia (cero overselling), idempotencia ante reintentos del cliente y expiración automática que devuelve stock al pool sin intervención manual. La fuente de verdad es PostgreSQL; las garantías de consistencia se construyen sobre un único `UPDATE ... WHERE available >= $qty` atómico, evitando locks de aplicación. Las decisiones arquitectónicas y los principios no negociables que sustentan este diseño están definidos en `constitution.md` y deben consultarse como referencia primaria.

## 2. Glosario

**Item**: producto inventariable con stock total fijo, definido en seed.

**Stock total** (`total`): cantidad inicial e inmutable del item. No se modifica durante la operación normal del sistema.

**Stock reservado** (`reserved`): suma de unidades comprometidas en reservas activas no expiradas. Se incrementa al crear una reserva y se decrementa al liberarla o expirarla.

**Stock disponible** (`available`): valor calculado como `total - reserved`. Representa lo que un nuevo usuario puede tomar en este instante.

**Reserva**: bloqueo temporal de N unidades de un item asociado a un usuario, con TTL fijo de 60 segundos desde su creación.

**TTL**: time-to-live de una reserva, fijado en 60 segundos. Expirado el plazo, la reserva pasa a estado `expired` y su quantity se devuelve al pool de stock.

**Idempotency-Key**: identificador opaco enviado por el cliente en `POST /reservations` que permite reintentar la operación con seguridad: el servidor garantiza que la misma key con el mismo payload produce el mismo efecto exactamente una vez.

**User-Id**: identificador opaco del cliente, UUID v4 generado en el navegador y persistido en localStorage. No implica autenticación; es un identificador de scope para reservas.

## 3. Requisitos funcionales

- **RF-01 Listar inventario disponible**: el sistema expone un endpoint que devuelve todos los items con su `total`, `reserved` y `available` calculados en tiempo real.
- **RF-02 Crear reserva con cantidad N**: el cliente puede solicitar la reserva de N unidades de un item; el sistema decrementa `available` atómicamente y registra `expires_at = now + 60s`.
- **RF-03 Idempotencia en creación de reservas**: toda solicitud `POST /reservations` debe incluir `Idempotency-Key`; reintentos con la misma key y payload no producen efectos duplicados.
- **RF-04 Liberar reserva manualmente**: el cliente puede liberar una reserva activa antes de su expiración mediante `DELETE /reservations/{id}`, devolviendo el stock al pool.
- **RF-05 Idempotencia en liberación**: múltiples llamadas `DELETE` sobre la misma reserva devuelven respuesta consistente y devuelven stock al pool exactamente una vez.
- **RF-06 Expiración automática por TTL**: una goroutine background detecta reservas con `expires_at < NOW()` cada 5 segundos y las marca como `expired`, devolviendo stock atómicamente.
- **RF-07 Listar reservas activas del usuario**: el endpoint `GET /reservations` devuelve, para el `X-User-Id` indicado, las reservas en estado `active`.
- **RF-08 Detección de conflictos por agotamiento de stock**: cuando `available < quantity`, el sistema responde con error machine-readable `OUT_OF_STOCK` y no modifica el stock.

## 4. Requisitos no funcionales

- **RNF-01 Cero overselling**: bajo carga concurrente (≥100 requests simultáneos sobre el mismo item), nunca el stock reservado supera el total del item.
- **RNF-02 Atomicidad a nivel de DB**: las garantías de consistencia se sustentan en `UPDATE ... WHERE available >= $qty` y guardas por `status`, sin locks pesimistas en aplicación ni `SELECT FOR UPDATE`.
- **RNF-03 Latencia objetivo**: p95 ≤ 100ms para `POST /reservations` en condiciones normales (DB local, ≤100 conexiones concurrentes).
- **RNF-04 Sincronización del frontend**: el cliente refresca inventario y reservas mediante polling cada 2 segundos; no se requiere websockets ni SSE.
- **RNF-05 Observabilidad mínima**: toda mutación de stock (creación, liberación manual, expiración) genera log estructurado con `reservation_id`, `item_id`, `user_id`, `delta` y `cause`.

## 5. User stories

**US-01**: Como comprador, quiero ver el inventario en tiempo real, para saber qué items tienen stock antes de reservar.

**US-02**: Como comprador, quiero reservar N unidades de un item, para asegurar mi compra antes de pagar.

**US-03**: Como comprador, quiero ver mis reservas activas con countdown, para saber cuánto tiempo me queda antes de que expiren.

**US-04**: Como comprador, quiero liberar una reserva manualmente, para devolver stock que ya no necesito sin tener que esperar al TTL.

**US-05**: Como comprador, quiero recibir un mensaje claro cuando otro usuario tomó el item, para entender por qué falló mi reserva en lugar de ver un error genérico.

## 6. Criterios de aceptación

### 6.1 Orden de validación en POST /reservations

Las validaciones del handler `POST /reservations` se ejecutan en este orden estricto. La primera que falla detiene el procesamiento y retorna su error correspondiente, evitando queries innecesarias a la base de datos:

1. `X-User-Id` presente y UUID v4 válido         → `INVALID_USER_ID`
2. `Idempotency-Key` presente y no vacío         → `MISSING_IDEMPOTENCY_KEY`
3. `Idempotency-Key` ≤ 256 chars y caracteres imprimibles → `INVALID_IDEMPOTENCY_KEY`
4. JSON body parseable                            → `INVALID_REQUEST_BODY`
5. `item_id` presente y UUID v4 válido           → `INVALID_ITEM_ID`
6. `quantity` entero positivo                     → `INVALID_QUANTITY`
7. Idempotency check: la `key` existe en el store → 200 OK con response cacheado, o 422 `IDEMPOTENCY_CONFLICT` si hash de payload difiere
8. `item_id` existe en DB                         → `ITEM_NOT_FOUND`
9. `UPDATE` atómico con guarda `available >= quantity` → `OUT_OF_STOCK` si `rowsAffected = 0`

**Rationale**: el orden es defensivo. Primero se validan headers de identidad, luego sintaxis del request, luego semántica del payload, y finalmente estado de la base de datos. Esto minimiza carga sobre PostgreSQL ante requests malformados y evita exponer detalles internos por timing.

Para `DELETE /reservations/{id}` el orden es:

1. `X-User-Id` presente y UUID v4 válido         → `INVALID_USER_ID`
2. `id` en path es UUID v4 válido                → `INVALID_RESERVATION_ID`
3. La reserva existe Y pertenece al `X-User-Id`  → `RESERVATION_NOT_FOUND` (404 indistinto entre "no existe" y "no es tuya", para no filtrar información)
4. La reserva NO está en estado `expired`        → `RESERVATION_EXPIRED` (410)
5. `UPDATE` atómico con guarda `status='active'` → `already_released` si `rowsAffected = 0`

### AC-001: Listado de inventario devuelve items con total, reserved, available
**DADO** que existen items en la base de datos con stock seed
**CUANDO** el cliente solicita `GET /items`
**ENTONCES** el response es 200 OK
**Y** el cuerpo es un array donde cada item incluye `id`, `name`, `total`, `reserved`, `available`
**Y** `available = total - reserved` para cada item

### AC-002: Reserva exitosa con stock disponible
**DADO** un item con `available >= quantity`
**CUANDO** el cliente envía `POST /reservations` con `item_id` y `quantity` válidos, header `X-User-Id` y `Idempotency-Key`
**ENTONCES** el response es 201 Created
**Y** el cuerpo contiene la reserva con `status="active"` y `expires_at = created_at + 60s`
**Y** `items.reserved` se incrementa en `quantity` (y `available` decrementa correspondientemente)

### AC-003: Reserva rechazada por stock insuficiente
**DADO** un item con `available < quantity`
**CUANDO** el cliente envía `POST /reservations`
**ENTONCES** el response es 409 Conflict con código `OUT_OF_STOCK`
**Y** el stock del item permanece sin cambios
**Y** no se persiste ninguna reserva

### AC-004: Reserva rechazada por quantity inválida
**DADO** una solicitud con `quantity <= 0` o no entero
**CUANDO** el cliente envía `POST /reservations`
**ENTONCES** el response es 400 Bad Request con código `INVALID_QUANTITY`
**Y** el stock del item permanece sin cambios

### AC-005: Reserva sin Idempotency-Key
**DADO** una solicitud `POST /reservations` sin header `Idempotency-Key` o con valor vacío
**CUANDO** el handler procesa la request
**ENTONCES** el response es 400 Bad Request con código `MISSING_IDEMPOTENCY_KEY`
**Y** no se intenta ninguna mutación de stock

### AC-006: Idempotencia con misma key y mismo payload
**DADO** una reserva ya creada con `Idempotency-Key=K` y payload P
**CUANDO** el cliente reenvía `POST /reservations` con la misma `K` y el mismo P
**ENTONCES** el response es 200 OK con la reserva original (mismo `id`, mismo `expires_at`)
**Y** el stock NO se decrementa por segunda vez
**Y** no se persiste una nueva reserva

### AC-007: Idempotencia con misma key y payload distinto
**DADO** una reserva ya creada con `Idempotency-Key=K` y payload P1
**CUANDO** el cliente envía `POST /reservations` con la misma `K` pero payload P2 ≠ P1
**ENTONCES** el response es 422 Unprocessable Entity con código `IDEMPOTENCY_CONFLICT`
**Y** el stock no se modifica
**Y** la reserva original permanece intacta

### AC-008: Concurrencia controlada bajo contención
**DADO** un item con `available = 10`
**CUANDO** se envían 100 requests `POST /reservations` simultáneos solicitando 1 unidad cada uno, con `Idempotency-Key` distintos
**ENTONCES** exactamente 10 responses son 201 Created
**Y** exactamente 90 responses son 409 Conflict con código `OUT_OF_STOCK`
**Y** el `available` final del item es 0
**Y** `reserved` nunca excede `total` en ningún momento

### AC-009: Liberación manual de reserva activa
**DADO** una reserva con `status="active"` y `expires_at > NOW()`
**CUANDO** el cliente envía `DELETE /reservations/{id}` con `X-User-Id` correspondiente
**ENTONCES** el response es 200 OK con `status="released"`
**Y** la quantity se devuelve al pool (`reserved` decrementa, `available` incrementa)
**Y** `released_at` queda registrado

### AC-010: Liberación de reserva inexistente
**DADO** un `reservation_id` que no existe en la base de datos
**CUANDO** el cliente envía `DELETE /reservations/{id}`
**ENTONCES** el response es 404 Not Found con código `RESERVATION_NOT_FOUND`

### AC-011: Liberación idempotente
**DADO** una reserva activa
**CUANDO** el cliente envía dos `DELETE /reservations/{id}` consecutivos
**ENTONCES** ambas responses son 200 OK
**Y** la primera retorna `{"status":"released"}` y la segunda `{"status":"already_released"}`
**Y** el stock se devuelve al pool exactamente una vez

### AC-012: Liberación concurrente
**DADO** una reserva activa
**CUANDO** se envían 50 `DELETE /reservations/{id}` paralelos
**ENTONCES** exactamente 1 response retorna `{"status":"released"}`
**Y** las 49 restantes retornan `{"status":"already_released"}`
**Y** el stock se devuelve al pool exactamente una vez

### AC-013: Expiración por TTL
**DADO** una reserva con `expires_at < NOW()` y `status="active"`
**CUANDO** la goroutine de expiración ejecuta su pasada (cada 5 segundos)
**ENTONCES** la reserva pasa a `status="expired"` dentro de los 5 segundos siguientes al vencimiento
**Y** la quantity se devuelve al pool atómicamente
**Y** se emite un log estructurado con `cause="ttl_expired"`

### AC-014: Liberación de reserva ya expirada
**DADO** una reserva con `status="expired"`
**CUANDO** el cliente envía `DELETE /reservations/{id}`
**ENTONCES** el response es 410 Gone con código `RESERVATION_EXPIRED`
**Y** el stock no se devuelve por segunda vez

### AC-015: GET /reservations filtra por usuario
**DADO** múltiples reservas en la base con distintos `user_id` y `status`
**CUANDO** el cliente envía `GET /reservations` con header `X-User-Id=U`
**ENTONCES** el response es 200 OK con un array
**Y** todas las reservas devueltas tienen `user_id=U` y `status="active"`

### AC-016: GET /reservations sin X-User-Id
**DADO** una request `GET /reservations` sin header `X-User-Id` o con UUID inválido
**CUANDO** el handler procesa la request
**ENTONCES** el response es 400 Bad Request con código `INVALID_USER_ID`

### AC-017: Reserva sobre item inexistente
**DADO** un `item_id` que no existe en la base de datos
**CUANDO** el cliente envía `POST /reservations` con ese `item_id`
**ENTONCES** el response es 404 Not Found con código `ITEM_NOT_FOUND`
**Y** no se persiste ninguna reserva

### AC-018: Cantidad mayor que el stock total
**DADO** un item con `total=10`
**CUANDO** el cliente envía `POST /reservations` con `quantity=15`
**ENTONCES** el response es 409 Conflict con código `OUT_OF_STOCK`
**Y** el sistema no distingue este caso de "agotado por reservas previas"

### AC-019: Race entre expiración y liberación manual
**DADO** una reserva cuyo `expires_at` está a milisegundos del NOW
**CUANDO** la goroutine de expiración y un `DELETE` manual compiten por la misma reserva
**ENTONCES** la primera operación atómica gana (el `UPDATE` con guarda de `status='active'`)
**Y** la operación perdedora observa `status` distinto de `active` y no devuelve stock
**Y** el stock final del item es consistente con una única devolución

### AC-020: Item con available=0 al inicio de la query
**DADO** un item con `available = 0`
**CUANDO** el cliente envía `POST /reservations` con `quantity >= 1`
**ENTONCES** el response es 409 Conflict con código `OUT_OF_STOCK` inmediatamente
**Y** el sistema no realiza ningún intento de UPDATE redundante

### AC-021: DELETE de reserva con X-User-Id distinto al dueño
**DADO** una reserva creada por el usuario A
**CUANDO** el usuario B (con `X-User-Id` distinto al de A) envía `DELETE /reservations/{id}`
**ENTONCES** el response es 404 Not Found con código `RESERVATION_NOT_FOUND`
**Y** la reserva NO se modifica
**Y** el sistema NO revela la existencia de reservas de otros usuarios

## 7. Edge cases explícitos

### EC-01: Reintento de POST con misma Idempotency-Key después de expiración
**Escenario**: el cliente envía `POST /reservations` con key `K`, recibe 201, la reserva expira por TTL a los 60s, y el cliente reintenta con la misma `K`.
**Decisión de diseño**: el idempotency store retiene la respuesta original por al menos 24h. El reintento devuelve la reserva original con su estado actual (`expired`) y el response cacheado del 201 inicial. El cliente, al ver `status="expired"` en el cuerpo o al consultar `GET /reservations`, debe iniciar una nueva solicitud con una `Idempotency-Key` nueva.
**Test que lo cubre**: AC-006 más caso adicional documentado en suite de integración.

### EC-02: Race entre release manual y expiración automática
**Escenario**: una reserva está a 1 segundo de expirar y el usuario ejecuta `DELETE` en ese instante; simultáneamente, la goroutine de expiración la procesa.
**Decisión de diseño**: ambas operaciones intentan un `UPDATE` atómico con guarda `WHERE status='active'`. La primera en adquirir el row gana; la segunda no afecta filas y retorna `already_released` o `already_expired` sin devolver stock.
**Test que lo cubre**: AC-011, AC-012, AC-014, AC-019.

### EC-03: Múltiples reservas del mismo user sobre el mismo item
**Escenario**: el usuario reserva 2 unidades del item `X`, luego envía otra solicitud para reservar 3 unidades más del mismo item.
**Decisión de diseño**: permitido. Cada reserva es independiente con su propia `Idempotency-Key`, su propio TTL y su propio `reservation_id`. `GET /reservations` retorna las dos como entradas distintas.
**Test que lo cubre**: AC-015 más caso adicional en suite de integración.

### EC-04: Idempotency-Key vacío o malformado
**Escenario**: el header `Idempotency-Key` está presente pero con valor `""`, excede 256 caracteres, o contiene caracteres no imprimibles.
**Decisión de diseño**: validación a nivel handler antes de cualquier mutación. Vacío o ausente → 400 con `MISSING_IDEMPOTENCY_KEY`; >256 chars o caracteres no imprimibles → 400 con `INVALID_IDEMPOTENCY_KEY`.
**Test que lo cubre**: AC-005 más casos adicionales de borde.

### EC-05: Stock total del item modificado externamente
**Escenario**: alguien ejecuta `UPDATE items SET total = ...` manualmente fuera del API durante la operación.
**Decisión de diseño**: fuera del scope del reto. La constitución asume que `items.total` es inmutable post-seed; cualquier cambio externo es responsabilidad del operador y puede dejar el sistema en estado inconsistente temporal hasta que las reservas activas expiren.
**Test que lo cubre**: ninguno; documentado como asunción explícita.

### EC-06: Goroutine de expiración cae mientras hay reservas vencidas
**Escenario**: el proceso Go reinicia o la goroutine se cae. Existen reservas con `expires_at < NOW()` aún en `status='active'`.
**Decisión de diseño**: al arranque del proceso, antes de aceptar requests, la goroutine ejecuta una pasada inicial de cleanup que marca todas las reservas vencidas como `expired` y devuelve stock atómicamente. Esto garantiza convergencia incluso tras downtime.
**Test que lo cubre**: documentado en `plan.md` como requisito de bootstrap; no requiere AC propio en la spec funcional.

### EC-07: Distinción entre "available=0 desde el inicio" vs "available=0 por reservas"
**Escenario**: el frontend debe mostrar "Out of Stock" coherentemente independientemente de la causa.
**Decisión de diseño**: el endpoint `GET /items` devuelve `available` calculado en tiempo real. El frontend renderiza "Out of Stock" si `available === 0` sin distinguir la causa raíz; esto simplifica la UI y evita exponer detalles internos de stock.
**Test que lo cubre**: AC-001, AC-020.

### EC-08: Idempotency store crece sin límite
**Escenario**: la tabla `idempotency_keys` acumula entradas indefinidamente, degradando rendimiento.
**Decisión de diseño**: TTL operacional de 24h en el idempotency store; la misma goroutine de expiración purga entradas vencidas. Documentado como ítem operacional que no afecta correctness funcional.
**Test que lo cubre**: ninguno (operacional); documentado en `plan.md`.

## 8. Modelo de datos

### Tabla `items`

| campo       | tipo        | constraints                                              |
|-------------|-------------|----------------------------------------------------------|
| id          | UUID        | PK                                                       |
| name        | TEXT        | NOT NULL                                                 |
| total       | INTEGER     | NOT NULL, CHECK (total >= 0)                             |
| reserved    | INTEGER     | NOT NULL DEFAULT 0, CHECK (reserved >= 0 AND reserved <= total) |
| created_at  | TIMESTAMPTZ | NOT NULL DEFAULT NOW()                                   |

Nota: `available` es un valor calculado como `total - reserved`. No se persiste para evitar denormalización y posibles inconsistencias.

### Tabla `reservations`

| campo        | tipo        | constraints                                                          |
|--------------|-------------|----------------------------------------------------------------------|
| id           | UUID        | PK                                                                   |
| item_id      | UUID        | FK → items.id                                                        |
| user_id      | UUID        | NOT NULL                                                             |
| quantity     | INTEGER     | NOT NULL, CHECK (quantity > 0)                                       |
| status       | TEXT        | NOT NULL, CHECK (status IN ('active','released','expired'))          |
| expires_at   | TIMESTAMPTZ | NOT NULL                                                             |
| created_at   | TIMESTAMPTZ | NOT NULL DEFAULT NOW()                                               |
| released_at  | TIMESTAMPTZ | NULL                                                                 |

Índices:
- `(status, expires_at)` para acelerar la query del job de expiración.
- `(user_id, status)` para `GET /reservations`.

### Tabla `idempotency_keys`

| campo            | tipo        | constraints                                          |
|------------------|-------------|------------------------------------------------------|
| key              | TEXT        | PK (≤256 chars)                                      |
| request_hash     | TEXT        | NOT NULL (SHA-256 del payload normalizado)           |
| reservation_id   | UUID        | FK → reservations.id                                 |
| response_status  | INTEGER     | NOT NULL                                             |
| response_body    | JSONB       | NOT NULL                                             |
| created_at       | TIMESTAMPTZ | NOT NULL DEFAULT NOW()                               |

TTL operacional de 24h, limpiado por la goroutine de mantenimiento.

### 8.1 Validación de identificadores UUID

Todos los headers y campos UUID del API se validan como **UUID v4 estricto**: formato `8-4-4-4-12` hexadecimal, con `4` en la posición de versión y `8|9|a|b` en la variante. UUIDs v1, v3, v5, o strings con formato UUID pero versión distinta de v4 retornan:

- `INVALID_USER_ID` para el header `X-User-Id`
- `INVALID_ITEM_ID` para el campo `item_id` del body
- `INVALID_RESERVATION_ID` para el parámetro `id` del path en `DELETE /reservations/{id}`

**Rationale**: aceptar solo UUID v4 reduce la superficie de ambigüedad y elimina la necesidad de validación cruzada entre versiones de UUID. La generación en cliente (`crypto.randomUUID()` en navegadores modernos) produce v4 por defecto.

## 9. Contrato de API

El contrato HTTP completo (schemas, parámetros, ejemplos, códigos de respuesta) está definido en `specs/reservation/openapi.yaml`. Resumen de endpoints:

| Método  | Ruta                       | Propósito                                       |
|---------|----------------------------|-------------------------------------------------|
| GET     | /items                     | Lista todos los items con stock calculado       |
| POST    | /reservations              | Crea una reserva (requiere Idempotency-Key)     |
| DELETE  | /reservations/{id}         | Libera una reserva (idempotente)                |
| GET     | /reservations              | Lista reservas activas del usuario actual       |

Headers comunes:
- `X-User-Id`: UUID v4. Requerido en `POST /reservations`, `DELETE /reservations/{id}` y `GET /reservations`.
- `Idempotency-Key`: string opaco, ≤256 chars. Requerido en `POST /reservations`.

## 9.1 Catálogo completo de códigos de error

Lista actualizada de códigos `code` retornados por el API (machine-readable, estables para versionado):

| Código                          | HTTP | Origen                                                |
|---------------------------------|------|-------------------------------------------------------|
| INVALID_USER_ID                 | 400  | Header `X-User-Id` ausente, vacío o no UUID v4        |
| MISSING_IDEMPOTENCY_KEY         | 400  | Header `Idempotency-Key` ausente o vacío en POST      |
| INVALID_IDEMPOTENCY_KEY         | 400  | `Idempotency-Key` excede 256 chars o tiene no-printables |
| INVALID_REQUEST_BODY            | 400  | JSON malformado o tipo incorrecto                     |
| INVALID_ITEM_ID                 | 400  | `item_id` ausente o no UUID v4                        |
| INVALID_RESERVATION_ID          | 400  | `id` en path no es UUID v4                            |
| INVALID_QUANTITY                | 400  | `quantity` no entero o ≤ 0                            |
| ITEM_NOT_FOUND                  | 404  | `item_id` no existe en DB                             |
| RESERVATION_NOT_FOUND           | 404  | Reserva no existe o pertenece a otro usuario          |
| OUT_OF_STOCK                    | 409  | `available < quantity`                                |
| RESERVATION_EXPIRED             | 410  | DELETE sobre reserva en estado `expired`              |
| RESERVATION_ALREADY_RELEASED    | 200  | DELETE sobre reserva en estado `released` (no es error, es no-op informativo en el body) |
| IDEMPOTENCY_CONFLICT            | 422  | Misma `Idempotency-Key`, payload distinto             |

Nota: `RESERVATION_ALREADY_RELEASED` se devuelve con HTTP 200 dentro del campo `status` del response (`{"status":"already_released"}`), no como error en el envelope `{code, message}`. Es la única excepción al patrón.

## 10. Preguntas abiertas y asunciones

**QA-01**: ¿La `quantity` máxima por reserva tiene un cap absoluto (ej. 100 unidades)?
**Asunción**: no, solo está limitada por el `available` actual del item. Si se requiere un cap, se documentará en Fase 2.

**QA-02**: ¿Las reservas de un usuario sobreviven a un cambio de `X-User-Id` (ej. logout/clear localStorage)?
**Asunción**: no, las reservas son strictly client-scoped por el header. Un cambio de `X-User-Id` deja las reservas previas huérfanas hasta su expiración natural por TTL.

**QA-03**: ¿El estado `expired` se mantiene en historial o se purga?
**Asunción**: se mantiene 24h y luego se purga junto con su `idempotency_key` asociada por la misma goroutine de mantenimiento.

**QA-04**: ¿El polling del frontend debe degradarse ante errores 5xx?
**Asunción**: backoff exponencial simple (2s → 4s → 8s, máximo 8s). El detalle de implementación se documenta en `plan.md`.
