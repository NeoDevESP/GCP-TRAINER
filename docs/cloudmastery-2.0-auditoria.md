# CloudMastery 2.0 — Auditoría del código (punto de partida: 1.0.68)

Hallazgos **verificados en el código** antes de tocar nada. Cada punto indica dónde está y cómo se ha comprobado.

## 1. Mapa de la arquitectura

```
web/ (Next.js 16, export estático servido por Go)
  app/<página>/page.tsx  ──fetch──►  /api/*   (lib/api.ts, JWT Bearer desde localStorage)
  lib/study.ts   progreso de Aprende, exámenes, repaso espaciado, "runs" (turno, plan, final)
  lib/plan.ts    plan de 10 días (lista fija de ítems)
  components/learn.tsx  useProgress(): estado de cada laboratorio desde /api/me/attempts
  components/Nav.tsx    raíl con 11 secciones (barra inferior en móvil)
  app/lab        espacio de trabajo: consola replicada + Cloud Shell (xterm) + tutor

cmd/gcplab  ──►  internal/api (HTTP, auth, rate limit)
                 ├─ internal/orchestrator  sesiones de laboratorio (estado del simulador por sesión)
                 │    └─ internal/cli  (gcloud, kubectl, bq, terraform, ssh…)  ─┐ mismo estado
                 │    └─ vistas de consola (/api/sessions/{id}/views/{kind}) ──┘ (sim.State)
                 ├─ internal/grader     rúbricas (comprobaciones de estado, http, iam, desk…)
                 ├─ internal/learning   catálogo, motor de XP/dominio, estudio (Aprende), KPIs
                 ├─ internal/tutor      recorrido paso a paso a partir de la solución
                 ├─ internal/mentor     preguntas socráticas tras entregar (+ LLM opcional)
                 └─ internal/store      JSON en fichero / SQLite / PostgreSQL
content/ (YAML): labs, rutas y etapas, temas de Aprende, exámenes, guía, mundo Zentra
```

La consola y la terminal comparten **un único `sim.State` por sesión** (orchestrator), así que la sincronización consola↔terminal ya es estructural. Se conserva tal cual.

## 2. Errores funcionales verificados (prioridad)

| # | Severidad | Dónde | Problema | Cómo se verificó |
|---|---|---|---|---|
| 1 | Crítica | `web/lib/plan.ts` `itemDone` | Un simulacro de examen **suspendido** marca el ítem del plan como hecho (`h.full` sin mirar la nota). | Lectura del código. |
| 2 | Crítica | `web/app/shift/page.tsx` `solved` | Un ticket cuenta como **resuelto** si existe cualquier intento entregado (`score >= 0`), aunque haya suspendido; además puntúa el SLA. | Lectura del código. |
| 3 | Alta | `components/learn.tsx` `useProgress` | Un laboratorio aprobado con la **solución a la vista o el tutor** cuenta igual que uno resuelto solo (`passed` ⇒ `done`). | Lectura del código. |
| 4 | Alta | `internal/api` `hint`, `tutorOn` | El **modo examen** solo oculta pistas en el cliente: el servidor sigue entregando pistas y la solución a quien llame a la API. | Lectura del código. |
| 5 | Alta | `internal/learning/engine.go` `Mastery` | La corrección usa la **mejor nota histórica** por laboratorio: un deterioro reciente no se ve. | Lectura del código. |
| 6 | Alta | `web/lib/study.ts` `readiness` | El 40 % de la preparación de un tema sale de **marcar conceptos como entendidos**; la nota es la **mejor** histórica. | Lectura del código. |
| 7 | Alta | `web/lib/plan.ts` | Días de 3,5 a **12 h** frente a «unas 4 horas al día» declaradas (medido con los minutos reales de cada laboratorio y tema). | Script sobre el contenido YAML. |
| 8 | Media | `web/lib/study.ts` `saveProgress`, `addHistory`, `saveRun`, `pushReview` | Errores de guardado **silenciados** (`.catch(() => {})`): el alumno cree que guardó. Sin reintentos ni aviso. | Lectura del código. |
| 9 | Media | `internal/api/server.go` `authenticate` | El token se acepta también en la **URL** (`?token=`), que acaba en registros e historial. | Lectura del código. |
| 10 | Media | `web/lib/api.ts` | JWT en `localStorage` (expuesto a cualquier XSS). | Lectura del código. |
| 11 | Media | `internal/api/server.go` | Sin `Content-Security-Policy`; solo `X-Frame-Options`. | `grep` de cabeceras. |
| 12 | Media | `components/Nav.tsx` | 11 secciones planas; en el móvil la barra inferior se **corta** («Inciden…») y el selector de idioma no aparece. | Captura en 390 px. |
| 13 | Baja | `app/layout.tsx` | `lang="es"` fijo hasta que se monta el cliente (se corrige en `useI18n`, pero solo en páginas que lo usan). | Lectura del código. |

Ya correcto (no se toca): Monaco y Mermaid ya se cargan de forma diferida; el detalle de laboratorio (`GET /api/labs/{id}`) no incluye la solución; el examen final de Zentra ya anula los casos con tutor o solución; el servidor ya tiene limitación de peticiones.

## 3. Estrategia de migración segura

1. **Sin romper datos**: los intentos (`attempts`) y el documento de estudio conservan sus campos. Los nuevos (`mode`, `last`, `history` de notas, estados) son opcionales y se derivan de lo que ya existe; los perfiles antiguos se leen igual.
2. **Lógica crítica en módulos puros con tests**: estados de actividad y modelo de dominio en Go (`internal/learning`) y en TypeScript sin dependencias (`web/lib/model.ts`, probado con `node --test`).
3. **El servidor manda**: los modos de evaluación se aplican en la API (no solo ocultando botones).
4. **Por bloques con commit**: cada bloque compila, pasa `go test ./...`, `tsc`, `check:i18n`, `next build` y las pruebas de navegador antes de seguir.
