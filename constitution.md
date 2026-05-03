# Constitución del Proyecto — Sistema de Reservas de Inventario

Este documento declara los principios de ingeniería no negociables que rigen el diseño, la implementación y la revisión del Sistema de Reservas de Inventario. Todo contribuyente — humano o agente — debe respetarlos. Las violaciones son bloqueantes en code review y CI.

## 1. Desarrollo guiado por especificación

Todo cambio se rastrea hasta un criterio de aceptación numerado en `spec.md`. No se escribe código sin un AC que lo respalde.

**Cómo lo hacemos cumplir:**
- La descripción de cada PR referencia los IDs de AC que implementa (por ejemplo, `AC-001`, `AC-002`).
- El mensaje de cada commit incluye el o los IDs de AC relevantes en su cuerpo.
- Los PRs que introducen comportamiento no cubierto por un AC se rechazan; primero se añade el AC a `spec.md` y luego se escribe el código.

## 2. APIs con contrato primero

Toda ruta REST se define en `openapi.yaml` antes de escribir cualquier handler. El documento OpenAPI es la fuente de verdad de la superficie HTTP, no el código Go.

**Cómo lo hacemos cumplir:**
- Los handlers se validan contra `openapi.yaml` en CI; cualquier desviación rompe el build.
- Los contract tests verifican que las formas de request y response coinciden con el schema.
- Cualquier cambio en la firma de una ruta exige actualizar `openapi.yaml` en el mismo commit que el cambio del handler.

## 3. Concurrencia atómica en la capa de datos

Las mutaciones de stock son atómicas a nivel de base de datos. No se usan locks de aplicación para serializar cambios de inventario (nada de `sync.Mutex` ni `sync.RWMutex` alrededor de contadores de stock en Go). La base de datos es la única fuente de verdad del estado del inventario.

**Cómo lo hacemos cumplir:**
- En code review se rechaza cualquier `sync.Mutex` o `sync.RWMutex` que proteja estado relacionado con stock.
- Las escrituras de reservas usan una única sentencia SQL condicional (por ejemplo, `UPDATE ... WHERE available >= ?`) o un constructo transaccional equivalente.
- Tests de concurrencia validan el invariante de que la cantidad total reservada nunca supera el stock disponible.

## 4. Idempotencia por defecto

Las operaciones mutantes (`POST`, `DELETE`) son idempotentes. La identidad se declara mediante una cabecera `Idempotency-Key` (para creación de recursos) o mediante el ID del propio recurso (para borrado). Los reintentos jamás deben producir cambios de estado duplicados.

**Cómo lo hacemos cumplir:**
- `POST /reservations` exige una cabecera `Idempotency-Key`; las claves duplicadas devuelven la respuesta original.
- `DELETE /reservations/:id` es seguro de reintentar y devuelve el mismo resultado en llamadas repetidas.
- Tests dedicados de idempotencia cubren ambos endpoints, incluyendo reintentos concurrentes con la misma clave.

## 5. Trazabilidad

Cada commit de implementación cita uno o más IDs de tarea de `tasks.md` (por ejemplo, `feat(backend): T-007 atomic reservation insert`). Cada tarea de `tasks.md` cita uno o más IDs de AC de `spec.md`. La cadena `spec → tasks → commits` es auditable de extremo a extremo.

**Cómo lo hacemos cumplir:**
- Convención de mensajes de commit: `<type>(<scope>): <T-id> <summary>`.
- Cada entrada en `tasks.md` incluye un paso de "Verificación" que indica el test o comprobación que demuestra que la tarea está hecha.
- En code review se verifica que el ID de tarea existe en `tasks.md` y que los ACs enlazados existen en `spec.md`.

## 6. Probar la condición de carrera

Toda operación que pueda invocarse de forma concurrente DEBE tener un test que ejercite al menos 50 goroutines simultáneamente y valide el invariante a nivel de sistema (no hay sobreventa, no hay stock negativo, la idempotencia se mantiene).

**Cómo lo hacemos cumplir:**
- La suite de tests incluye los cuatro tests obligatorios de concurrencia e idempotencia del enunciado del reto.
- Todo nuevo endpoint mutante exige un race test correspondiente antes de fusionar.
- Los tests verifican post-condiciones sobre el estado de la base de datos, no sólo sobre los códigos de respuesta individuales.

## 7. Sin desincronización silenciosa en el cliente

El frontend nunca asume que el estado del inventario está al día. Se refresca por polling (por defecto) o por server-push al renderizar el dashboard, y expone explícitamente al usuario los estados de "datos obsoletos" y "conflicto".

**Cómo lo hacemos cumplir:**
- El dashboard muestra un indicador visible de "última actualización".
- Las respuestas HTTP `409 Conflict` disparan un toast visible y un re-fetch automático del recurso afectado.
- Los componentes de UI no cachean cantidades de inventario más allá del ciclo de render actual sin una política de refresco explícita.

## 8. Contratos de error explícitos

Todo error de negocio devuelve un `code` estable y legible por máquina (`OUT_OF_STOCK`, `IDEMPOTENCY_CONFLICT`, `RESERVATION_EXPIRED`, `RESERVATION_NOT_FOUND`, `INVALID_QUANTITY`) además de un mensaje legible por humanos. El frontend ramifica sobre `code`, nunca sobre el texto del mensaje.

**Cómo lo hacemos cumplir:**
- El schema de respuesta de error está definido en `openapi.yaml` y se reutiliza en todas las respuestas de error.
- Los tests verifican el campo `code` en cada ruta de error, no la cadena del mensaje.
- Añadir un nuevo error de negocio exige añadir su `code` al schema de OpenAPI en el mismo commit.

## Enmiendas

Los cambios a esta constitución deben hacerse en un commit separado con el mensaje `docs(constitution): amend [seccion]`. Toda enmienda debe justificarse en `spec-kit-notes.md`.
