# Spec Kit Notes — Sistema de Reservas de Inventario

Este documento registra el proceso de aplicación del **Spec Kit Workflow** durante el desarrollo del reto "Inventory Reservation System" para un cliente externo. Su propósito es triple: dejar trazabilidad de decisiones de diseño, documentar las asunciones tomadas ante ambigüedades del enunciado, y servir como bitácora de pivots y refinamientos.

Es un documento vivo: cada fase del workflow añade entradas a las secciones correspondientes mediante commits dedicados.

## 1. Workflow seguido

El proyecto se desarrolla en cinco fases estrictas, cada una con su commit (o conjunto de commits) específico, sin escribir código fuente hasta la fase 5:

| Fase | Entregable | Estado |
|------|-----------|--------|
| 0 — Setup | `.gitignore`, `git init`, remote configurado | Completada |
| 1 — Constitution + Spec | `constitution.md`, `specs/reservation/spec.md`, `specs/reservation/openapi.yaml` | Completada |
| 2 — Clarify | Edits sobre `spec.md`, este archivo `spec-kit-notes.md` | En curso |
| 3 — Plan | `plan.md` (arquitectura, stack, concurrencia, testing) | Pendiente |
| 4 — Tasks | `tasks.md` (lista ordenada con IDs y verificación) | Pendiente |
| 5 — Implementation | Backend Go, Frontend React, tests, seed, README | Pendiente |

## 2. Herramientas y entorno

- **Equipo**: iMac (Intel) con macOS Ventura 13.7.8.
- **Stack instalado**: Go 1.26.2, Node 20.20.2 (vía `nvm`), PostgreSQL 18.3 (vía Postgres.app), Git 2.39.2.
- **Editor**: Visual Studio Code 1.118.1 (Universal).
- **Agente**: Claude Code v2.1.126 con modelo Claude Opus 4.7 (1M tokens de contexto), accedido desde el panel integrado de VS Code, autenticado con cuenta Claude Max.
- **Repositorio**: https://github.com/cibersabueso/challengebeeyong (público).

## 3. LLM utilizado y justificación

**Modelo**: Claude Opus 4.7 (Anthropic), accedido vía Claude Code.

**Razones técnicas**:

1. **Coherencia con el Spec Kit Workflow**: Claude Code lee archivos `.md` del repo como contexto persistente. Eso permite que la spec, la constitución y este mismo `spec-kit-notes.md` actúen como fuente de verdad para el agente, no como prompts improvisados sesión a sesión.
2. **Razonamiento sobre concurrencia**: la pieza crítica del reto (atomic decrement con `UPDATE ... WHERE available >= $qty`, manejo de Idempotency-Key, goroutine de TTL) requiere un modelo fuerte para diseñar invariantes correctamente. Opus 4.7 produjo decisiones consistentes con la documentación oficial de PostgreSQL y patrones de la industria (Stripe, Shopify) sin alucinar.
3. **Human-in-the-loop riguroso**: el agente generó borradores, pero cada edit fue revisado y validado antes de commitear. El criterio arquitectónico es del autor; el agente acelera la redacción y la implementación, no la decisión.
4. **Soporte nativo en VS Code**: la integración Claude Code → archivos del workspace evita copy-paste manual entre chat y editor, reduciendo errores de transcripción en archivos largos como `openapi.yaml` (432 líneas) y `spec.md` (364 líneas).

## 4. Asunciones tomadas ante ambigüedades del enunciado

El PDF del reto declara explícitamente: *"There might be intentional ambiguity in this challenge. Please document your assumptions or ask for clarification when applicable."* Esta sección lista cada asunción con su rationale.

### A-01: No existe estado `confirmed` para las reservas

El enunciado menciona "if not confirmed" en pasivo, pero no define un endpoint de confirmación, ni tests asociados, ni lo lista como requisito funcional. Implementamos solo tres estados: `active`, `released`, `expired`. Cualquier "confirmación" quedaría fuera de scope.

**Documentado en**: `specs/reservation/spec.md` sección 2 (Glosario) y sección 8 (Modelo de datos).

### A-02: Identificación de usuario por header `X-User-Id` sin autenticación

El reto requiere mostrar las reservas del usuario actual ("Your Reservations" en el mockup), pero no menciona autenticación, login, JWT ni sesiones. Implementamos identificación opaca por header `X-User-Id` (UUID v4 generado en el cliente y persistido en `localStorage`). Es el patrón estándar para challenges que piden user-scoped data sin pedir auth.

**Documentado en**: `spec.md` secciones 2, 3 (RF-07), 8.1 (validación UUID v4).

### A-03: Concurrencia atómica vía `UPDATE` condicional, no `SELECT FOR UPDATE`

El enunciado pide "Correct use of PostgreSQL and/or Go to prevent race conditions" sin especificar mecanismo. Optamos por `UPDATE items SET available = available - $qty WHERE id = $id AND available >= $qty RETURNING ...`. Razones:

- Atomicidad garantizada por MVCC de PostgreSQL en single-row UPDATE.
- Mayor throughput que `SELECT FOR UPDATE` (no serializa transacciones).
- Sin locks de aplicación en Go (`sync.Mutex` está prohibido por la constitución).
- Patrón documentado en blogs de ingeniería de Stripe y Shopify para inventory.

Detección de overselling: si `RowsAffected = 0`, el handler retorna `409 OUT_OF_STOCK`.

**Documentado en**: `constitution.md` principio 3, `spec.md` RNF-02.

### A-04: TTL gestionado por la base de datos como source of truth + goroutine de barrido

El backend determina expiración por la columna `expires_at TIMESTAMPTZ`. Una goroutine corre cada 5 segundos ejecutando un `UPDATE` que marca como `expired` las reservas vencidas y devuelve stock atómicamente. Razones:

- Postgres es la única fuente de verdad (alineado con el principio 3 de la constitución).
- 5 segundos es indistinguible para el usuario (el frontend hace polling cada 2s).
- Resiliente a reinicios: en bootstrap se ejecuta una pasada inicial de cleanup antes de aceptar requests (ver `EC-06` en `spec.md`).

Descartamos: `pg_cron` (extensión no estándar, peor portabilidad) y *lazy expiry on read* (riesgo de stock atrapado si nadie consulta).

**Documentado en**: `spec.md` RF-06, AC-013, EC-06.

### A-05: Frontend con polling, no WebSockets

El reto permite "WebSockets or polling" para la sincronización del frontend. Elegimos polling cada 2 segundos. Razones:

- Cumple el requisito funcional sin agregar complejidad de manejo de reconexión, fallback y broadcast.
- Encaja en el time budget de 8 a 10 horas.
- La rúbrica evalúa "UI stays perfectly in sync"; polling 2s es suficiente para flash sale UX.
- Ante errores 5xx, el cliente aplica backoff exponencial (2s → 4s → 8s, max 8s).

**Documentado en**: `spec.md` RNF-04, QA-04.

### A-06: Idempotency store con TTL operacional de 24 horas

La tabla `idempotency_keys` retiene la response cacheada del 201 inicial por 24 horas. La misma goroutine de mantenimiento purga entradas vencidas. El reintento con la misma key después del TTL de la reserva (60s) pero antes del TTL del store (24h) devuelve la response original con el estado actual de la reserva (`expired`).

**Documentado en**: `spec.md` EC-01, EC-08.

### A-07: DELETE cross-user retorna 404 (no 403)

Si el usuario B intenta liberar una reserva creada por el usuario A, el endpoint retorna `404 RESERVATION_NOT_FOUND` en lugar de `403 Forbidden`. Razón: no filtrar la existencia de reservas de otros usuarios. Es el patrón usado por GitHub API y otros servicios maduros.

**Documentado en**: `spec.md` AC-021, sección 6.1 orden de validación de DELETE.

### A-08: Validación estricta de UUID v4

Todos los identificadores se validan como UUID v4 estricto (versión 4 en el dígito correspondiente, variante `8|9|a|b`). UUIDs de otras versiones se rechazan con código específico (`INVALID_USER_ID`, `INVALID_ITEM_ID`, `INVALID_RESERVATION_ID`).

**Documentado en**: `spec.md` sección 8.1.

### A-09: Item con `total` inmutable post-seed

Cualquier modificación de `items.total` fuera del API es responsabilidad del operador y queda fuera del scope. La constitución y el spec asumen que el seed inicial es la fuente de verdad para la cantidad total.

**Documentado en**: `spec.md` EC-05.

## 5. Pivots

Esta sección documenta los cambios de rumbo significativos durante el desarrollo, con foco en las decisiones que **se tomaron diferente a lo planificado** en fases anteriores.

### P-01: Stack del frontend bajado de bleeding-edge a LTS estable

**Contexto**: en el Bloque 7 (Frontend Setup), `npm create vite@latest` con flag `react-ts` pinneó por defecto un stack agresivo: React 19.2, TypeScript 6.0 alpha, ESLint 10, con flags de TS 5.7+ habilitados.

**Decisión original (plan.md)**: "Vite 5.x, React 18.x, TypeScript 5.x con strict mode".

**Pivot**: bajar manualmente a React 18.3.1, TypeScript 5.6.3, ESLint 9.13, Vite 5.4. Eliminar el flag `erasableSyntaxOnly` del `tsconfig.node.json` que requiere TS 5.7+.

**Razón**: el ecosistema de TanStack Query 5 y Testing Library 16 está al 100% probado contra React 18.3 LTS. Apuntar a React 19 introduce riesgos de incompatibilidad cuyos costos de debug exceden cualquier beneficio para un proyecto con time budget de 8-10 horas. El plan original lo anticipaba con la frase "React 18.x"; el pivot fue rechazar la sugerencia del scaffold y mantener la versión estable.

### P-02: Augmentation de Vitest 4 cambia respecto a Vitest 3

**Contexto**: el plan original especificaba `vite.config.ts` con un comentario `/// <reference types="vitest" />` para que TypeScript reconociera el bloque `test` del config. Esta sintaxis era válida en Vitest 3 pero rompe en Vitest 4.

**Pivot**: importar `defineConfig` desde `vitest/config` en lugar de `vite`. Esto aporta los tipos del bloque `test` automáticamente sin necesidad del triple-slash directive.

**Razón**: cambio de superficie pública de la librería entre majors. No afecta la lógica del proyecto; es un detalle de tipado.

### P-03: --passWithNoTests para suite vacía durante el setup

**Contexto**: Vitest 4 cambió el comportamiento default cuando no hay archivos de test: ahora sale con código 1 en lugar de 0. Esto rompía el smoke test del Bloque 7 que requería que `npm test` terminara con código 0 antes de tener tests.

**Pivot**: añadir `--passWithNoTests` al script `test` del `package.json`.

**Razón**: el bloque de setup necesita validar que el runner está cableado correctamente sin tests aún. Sin esta flag, el setup falla y bloquea el avance al Bloque 8.

### P-04: bytes.NewReader en lugar de helper custom para body parsing

**Contexto**: en el Bloque 4 el prompt original incluía un mini-helper `bytesReader` para evitar añadir el import de `bytes` solo para `bytes.NewReader`. El prompt explícitamente permitía optar por `bytes.NewReader` "si se prefiere por simplicidad".

**Pivot**: usar `bytes.NewReader` de stdlib.

**Razón**: el helper custom era over-engineering para evitar un import trivial. Stdlib gana en legibilidad y mantenibilidad.

### P-05: Borrado de App.css demo y assets de Vite

**Contexto**: el scaffold de Vite incluye un `App.css` con estilos demo y assets en `src/assets/` (react.svg, vite.svg) y `public/` (icons.svg, hero.png) que el nuevo `App.tsx` no usa. Inicialmente quedaron commiteados por inercia tras el Bloque 7.

**Pivot**: borrarlos en el Bloque 11 final como parte del cleanup de cierre.

**Razón**: aunque no afectan al build (tree-shaking elimina el código no referenciado), dejan archivos innecesarios en el repo público. Un repo profesional no incluye assets demo sin referencia.

### Decisiones del plan que se mantuvieron sin modificación

Para contexto, las siguientes decisiones del `plan.md` se ejecutaron exactamente como estaban planificadas, sin pivot:

- `UPDATE ... WHERE available >= $qty` como mecanismo de concurrencia (D-01).
- `pgx/v5` como driver, sin `database/sql` ni ORM (D-02).
- Migrations con `golang-migrate/migrate` embebido en el binario (D-03).
- `slog` stdlib como logger estructurado (D-04).
- TanStack Query con `refetchInterval: 2000` (D-05).
- Tailwind CSS sin libs de UI (D-06).
- Tests con Postgres local + DB dedicada `challengebeeyong_test` y TRUNCATE entre tests (D-07).
- Vitest + Testing Library (D-08).
- 21 ACs y 8 edge cases del spec se implementaron al 100%.
- Las 8 asunciones documentadas (A-01 a A-09) se mantuvieron coherentes en código.

## 6. Comandos y prompts relevantes pasados al agente

Lista de prompts significativos pasados a Claude Code, en orden cronológico. Se omiten interacciones triviales.

### Fase 1A — Constitution

- **Objetivo**: redactar `constitution.md` con 8 principios enforzables.
- **Iteraciones**: 2 (la primera versión salió en inglés, se regeneró en español tras confirmación del cliente vía email de que la documentación puede estar en español).

### Fase 1B — Spec + OpenAPI

- **Objetivo**: generar `specs/reservation/spec.md` (con 20 ACs y 8 edge cases) y `specs/reservation/openapi.yaml` (OpenAPI 3.1 completo) en una sola pasada.
- **Iteraciones**: 1. Validación posterior con `ruby -ryaml -e "YAML.load_file(...)"` confirmó que el YAML es sintácticamente correcto.
- **Output**: 301 líneas en `spec.md`, 432 líneas en `openapi.yaml`.

### Fase 2 — Clarify

- **Objetivo**: aplicar 4 edits incrementales al `spec.md` resolviendo ambigüedades detectadas durante la revisión:
  1. Insertar sección 6.1 con orden estricto de validación para POST y DELETE.
  2. Agregar AC-021 para el caso DELETE cross-user.
  3. Agregar sección 8.1 con validación estricta de UUID v4.
  4. Agregar sección 9.1 con catálogo completo de códigos de error.
- **Iteraciones**: 1. Resultado: 364 líneas (de 301), 21 ACs (de 20), sin renumeraciones ni rupturas de formato.

### Fase 5 — Implementación

La Fase 5 se ejecutó en 11 bloques temáticos, cada uno con su commit dedicado. Cada prompt al agente seguía la misma estructura: contexto del bloque, reglas de código, archivos a crear con su contenido EXACTO, smoke test obligatorio antes de finalizar. El agente NO podía ejecutar `git add` ni `git commit`; el commit se hacía manualmente solo tras revisión del diff.

**Bloque 1 — Backend Infra (T-001 a T-006)**: `go.mod`, config, pool de pgx, migraciones SQL crudas, seed con 6 items alineados al mockup, scaffold de `main.go`. Smoke test: arranque del binario muestra los 5 logs estructurados esperados (migrations applied, database pool ready, seed applied, server scaffold ready, shutting down) y `SELECT COUNT(*) FROM items` retorna 6.

**Bloque 2 — Backend Domain + Repos (T-007 a T-014)**: tipos puros, 13 errores tipados, 3 repositorios con queries SQL atómicas como constantes con nombre. Decisión clave: introducir una interfaz `Executor` que satisfacen tanto `*pgxpool.Pool` como `pgx.Tx`, para que los repos participen transparentemente en transacciones del service layer. Smoke test: `go build ./...` silencioso.

**Bloque 3 — Backend Service + Idempotency (T-015 a T-020)**: `CanonicalHash` con sort recursivo de keys + SHA-256, `ReservationService.Create` con flujo idempotency-first → tx con AtomicDecrement → Insert → Persist (con manejo de race contra PK 23505), `Release` y `ListByUser`, helper `LogStockMutation` para observabilidad.

**Bloque 4 — Backend HTTP (T-021 a T-026)**: chi router con middleware chain (RequestID, RealIP, Logger, Recoverer, Timeout 15s), middleware `RequireUserID` que valida UUID v4 estricto e inyecta el ID parseado en el context, handlers para los 4 endpoints aplicando el orden estricto de validación de spec.md sección 6.1, mapping de errores de dominio a HTTP status según spec.md sección 9.1. Smoke test: 4 curl directos contra el server validando los códigos de error con cada combinación de headers.

**Bloque 5 — Backend TTL (T-027 a T-029)**: `expiry.Service` con `RunOnce`, `Loop` (ticker + select sobre ctx.Done) y `Bootstrap` sincrónico. Wired en `main.go` antes del HTTP listener (cubre EC-06). Smoke test: insertar reserva con `expires_at` en el pasado → arrancar el servidor → verificar que la reserva queda `expired` y el stock se devuelve antes de aceptar tráfico, confirmado por orden de logs.

**Bloque 6 — Backend Tests (T-030 a T-034)**: helper `testutil` con `NewTestDB`, `Reset`, `SeedItem`, `NewServer` (httptest con la misma cadena de handlers que producción) y `findMigrationsDir` para resolver el path en cualquier package. 6 tests ejecutándose con `-race -count=1`: los 4 obligatorios del enunciado más bootstrap cleanup y cross-user DELETE. Todos verifican invariantes vía SELECT directo a DB. Race detector cero warnings.

**Bloque 7 — Frontend Setup (T-035 a T-038)**: Vite scaffold + Tailwind + TanStack Query + Vitest. Aquí surgió el primer **pivot real** (ver sección Pivots).

**Bloque 8 — Frontend API + Hooks (T-039 a T-043)**: `lib/uuid.ts`, `lib/errors.ts` con `ApiError` tipado, cliente HTTP que parsea automáticamente `{code, message}` del backend, hooks de TanStack Query con `refetchInterval: 2000` y mutations que generan `Idempotency-Key` con `crypto.randomUUID()` por invocación. Compilación bajo TS strict + noUncheckedIndexedAccess al primer intento.

**Bloque 9 — Frontend Components (T-044 a T-049)**: 8 componentes (Toast, StatusBadge, Skeleton, ItemCard con stepper, InventoryGrid, ReservationItem con countdown color-coded, ReservationsPanel, App.tsx reescrito). Layout 2 columnas alineado al mockup del PDF (inventory grid + sidebar de 320px). Bundle final 197 kB JS / 12 kB CSS, gzip 62 kB / 3.2 kB.

**Bloque 10 — Frontend Tests (T-050 a T-052)**: 11 tests de Vitest cubriendo los 3 obligatorios del enunciado (timer logic, reserve flow happy path, error state). Mocks con `vi.fn`, fake timers para countdown, `userEvent` para clicks reales.

**Bloque 11 — Cierre (T-053 a T-055)**: `README.md` raíz, actualización de este archivo, cleanup de assets demo de Vite, smoke test end-to-end final.

## 7. Trazabilidad

La cadena de trazabilidad del proyecto sigue este flujo:

```
constitution.md
       │
       │ define principios no negociables
       ▼
specs/reservation/spec.md
       │
       │ formaliza ACs (AC-001 a AC-021) y edge cases (EC-01 a EC-08)
       ▼
specs/reservation/openapi.yaml
       │
       │ contrato HTTP exacto referenciado por la spec
       ▼
plan.md  (Fase 3)
       │
       │ traduce ACs a decisiones de arquitectura
       ▼
tasks.md  (Fase 4)
       │
       │ descompone el plan en tareas con IDs (T-001, T-002, ...)
       │ cada tarea cita los ACs que implementa
       ▼
commits  (Fase 5)
       │
       │ cada commit cita los T-ids de tasks.md
       ▼
código fuente (backend + frontend + tests)
```

Esta cadena es auditable end-to-end: dado cualquier commit de implementación, se puede rastrear la línea de código → task ID → AC → edge case → principio de la constitución que la motiva.

## 8. Convenciones de commits

Formato seguido durante todo el proyecto (Conventional Commits adaptado):

- `chore: <summary>` — setup, configuración, no afecta funcionalidad.
- `docs(<scope>): <summary>` — documentación pura (constitution, spec, plan, tasks, notes, README).
- `docs(spec): <summary>` — modificaciones específicas a la spec.
- `feat(<scope>): T-XXX <summary>` — implementación que cumple una task de `tasks.md`.
- `test(<scope>): T-XXX <summary>` — tests aislados.
- `fix(<scope>): T-XXX <summary>` — correcciones sobre código previo.
- `refactor(<scope>): T-XXX <summary>` — refactor sin cambio de comportamiento observable.

Los commit messages se mantienen en inglés por convención global de la industria, mientras que la documentación en `.md` está en español por confirmación explícita del cliente.
