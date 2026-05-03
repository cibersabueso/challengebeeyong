# Spec Kit Notes — Sistema de Reservas de Inventario

Este documento registra el proceso de aplicación del **Spec Kit Workflow** durante el desarrollo del reto "Inventory Reservation System" para Beeyond Media. Su propósito es triple: dejar trazabilidad de decisiones de diseño, documentar las asunciones tomadas ante ambigüedades del enunciado, y servir como bitácora de pivots y refinamientos.

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

Esta sección documenta cualquier cambio de rumbo significativo durante el desarrollo. Cada entrada describe el contexto, qué se intentó originalmente, por qué falló, y qué se hizo en su lugar.

*A la fecha de Fase 2 no se han producido pivots. Todas las decisiones tomadas en Fase 0 a Fase 2 se mantienen sin modificación. Esta sección se actualizará en commits posteriores si surgen pivots durante Plan, Tasks o Implementation.*

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
