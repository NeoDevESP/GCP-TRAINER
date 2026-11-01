# CloudMastery 2.0 — Informe de entrega

Punto de partida: 1.0.68. Rama: `claude/bold-turing-acsitj`. Auditoría previa:
[cloudmastery-2.0-auditoria.md](cloudmastery-2.0-auditoria.md).

El trabajo se hizo por bloques; cada uno compila, pasa todas las pruebas y se
subió en su propio commit:

| Bloque | Commit | Contenido |
|---|---|---|
| 1 | `2.0 block 1: honest assessment` | Estados reales de actividad, exámenes y tickets suspendidos que no cuentan, nota reciente, modos de aprendizaje aplicados en la API |
| 2 | `2.0 block 2: dynamic study plan and reliable saving` | Plan por duración real y tiempo diario, cola de guardado con reintentos y conflictos, estado de guardado visible |
| 3 | `2.0 block 3: learning engine …` | Pistas en 5 niveles, tutor de diagnóstico, competencias, informe post-incidente |
| 4 | `2.0 block 4: navigation, onboarding …` | 5 destinos, búsqueda global, atajos, bienvenida con diagnóstico, siguiente acción, tema y densidad |
| 5 | `2.0 block 5: session in an HttpOnly cookie …` | Sesión en cookie HttpOnly, CSRF, fuera `?token=`, CSP y cabeceras |
| 6 | `2.0 block 6: weekly goal, docs …` | Objetivo semanal, documentación, versión 2.0.0 |

## 1. Problemas corregidos

Los 13 de la auditoría (tabla «Estado tras la 2.0» en la auditoría) y cinco
más encontrados por el camino:

- **Evaluación honesta.** Un simulacro suspendido ya no marca el plan; un
  ticket entregado sin resolver no cuenta como resuelto; aprobar con la
  solución o el tutor es «hecho con ayuda», no «superado»; el dominio usa la
  nota reciente (50/30/20) y no la mejor histórica; «Dominado» exige dos tests
  ≥ 85 % en días distintos; leer conceptos ya no da la preparación de un tema.
- **Modos aplicados en el servidor.** Independiente: sin pistas ni tutor
  (403). Producción: pistas sí, solución y tutor no. Los jefes finales siempre
  en independiente.
- **Plan realista.** Antes, días de 3,5 a 12 h frente a las «4 h» anunciadas.
  Ahora lo pendiente se reparte en sesiones con la duración real de cada
  actividad y el tiempo que elige la persona (30 min – 4 h), con actividades
  esenciales, recomendadas y opcionales.
- **Guardado fiable.** Antes los errores se silenciaban. Ahora hay cola con
  reintentos (2 s → 1 min), aviso visible (guardando, guardado, sin conexión,
  error con botón de reintento) y fusión cuando otro dispositivo guardó antes
  (409 + `X-Run-Base`).
- **Sesión.** El token salía en la URL (`?token=`, retorno OIDC) y vivía en
  `localStorage`. Ahora va en cookie HttpOnly, SameSite=Strict (Secure con
  HTTPS) y los cambios hechos con la cookie piden la cabecera `X-CM-Client`.
- **CSP y cabeceras** en todas las páginas.
- **Escalas ×100.** Los factores de «Cómo has trabajado» y las dimensiones del
  panel se mostraban multiplicados por 100.
- **Quitar un permiso público** penalizaba la seguridad de la solución
  correcta como si fuera un permiso arriesgado.
- **`gam info/print`** contaban como cambios en el tutor.
- **Contraste** de los diagramas en modo oscuro; **fecha** del plan con el
  formato del navegador; el campo de nombre sin `autocomplete`.

## 2. Funcionalidades nuevas

**Motor de aprendizaje (F3/F4)**
- Pistas en cinco niveles para cada laboratorio: pregunta guía → concepto →
  dónde mirar → comando de ejemplo (con los valores ocultos) → solución. Se
  construyen a partir de las pistas del autor y de la solución. Una prueba
  recorre los 187 laboratorios en los dos idiomas (358 escaleras) y comprueba
  que los niveles 1–3 no contienen ningún cambio de la solución. Cada nivel
  cuesta más y baja la autonomía; los niveles usados quedan en el intento.
- Tutor de diagnóstico al entregar: nombra el tipo de error (identidad
  equivocada, recurso equivocado, cambio arriesgado, cambiar antes de mirar,
  no comprobar después del último cambio) con los comandos de la persona y el
  siguiente paso. La solución oficial de cada laboratorio y misión se
  comprueba para que no dispare ninguno de los que culpan. En los 166 atajos
  tramposos de los laboratorios nombra al menos un error en 123.
- Competencias por habilidad: comprensión y transferencia se suman a
  ejecución, autonomía y retención; lo que aún no se ha medido se muestra
  como «—», no como 0.
- Informe post-incidente en incidencias (síntoma → evidencia → hipótesis →
  causa raíz → acción → validación → prevención), puntuado por apartado y
  contrastado con la sesión: la evidencia debe citar lo que se consultó y la
  validación, una comprobación posterior al cambio. Se puntúa aparte.
- Modos explorador, asistido, independiente y producción elegibles al empezar.

**Experiencia (F2/F5)**
- Cinco destinos (Inicio, Aprender, Practicar, Trabajo, Perfil) en escritorio
  y móvil; las páginas de cada destino como pestañas. No se quitó ninguna
  página: las 11 secciones anteriores viven dentro de su destino.
- Búsqueda global (Ctrl+K o `/`) sobre páginas, laboratorios y lecciones;
  atajos `g h/a/p/t/u` y `?`.
- Menú de cuenta con idioma, tema (sistema, claro, oscuro) y densidad
  (cómoda, compacta), también en móvil; se aplican antes de pintar.
- Bienvenida para cuentas nuevas: objetivo, tiempo al día y diagnóstico de
  seis preguntas («No lo sé» cuenta como fallo). Coloca a la persona (desde
  cero, base, ventaja), ajusta el plan y abre la primera actividad de su nivel.
- El panel empieza por **una** acción: continuar lo empezado, ajustar la ruta,
  repaso espaciado, laboratorio por repasar o siguiente paso.
- Objetivo semanal: tiempo real de los laboratorios entregados (con tope) y
  5 min por cuestionario, frente a tiempo diario × 5.

## 3. Archivos principales

Go: `internal/learning/engine.go` (modos, nota reciente, crédito,
`HintAutonomy`, competencias), `internal/tutor/ladder.go` (escalera de
pistas), `internal/grader/diagnose.go` (tutor de diagnóstico),
`internal/grader/report.go` (informe post-incidente),
`internal/orchestrator/orchestrator.go` (`HintLadder`, `learnMode`,
`report`), `internal/api/server.go` (modos, cookie, CSRF, cabeceras),
`internal/api/study.go` (historial de tests, revisiones de runs, conflictos).

Web: `web/lib/model.ts` (estados, reglas de examen y tema, planificador,
fusión, colocación, objetivo semanal — sin dependencias y con pruebas),
`web/lib/sync.ts` (cola de guardado), `web/lib/prefs.ts` (tema y densidad),
`web/lib/api.ts` (sesión), `web/components/Nav.tsx`,
`web/components/CommandSearch.tsx`, `web/components/SaveStatus.tsx`,
`web/app/start/page.tsx`, `web/app/plan/page.tsx`, `web/app/lab/page.tsx`,
`web/app/dashboard/page.tsx`, `web/e2e/flows.mjs`.

## 4. Migraciones y compatibilidad

No hay migración que ejecutar: todos los campos nuevos son opcionales y los
datos de la 1.0 se leen igual.

- Intentos: `mode` (vacío = asistido). `hintsUsed` sigue siendo la lista de
  índices; con la escalera, el índice + 1 es el nivel.
- Documento de estudio: `scores` por tema (si falta, se usa `best`),
  `runRevs` (si falta, revisión 0). Nueva clave de run permitida:
  `cm-onboarding`.
- Sesión: quien tenga un token de la 1.0 en `localStorage` sigue entrando
  (Bearer); al volver a iniciar sesión pasa a la cookie. Los clientes de API
  siguen con `Authorization: Bearer`. **Ya no** se acepta `?token=` en la URL.
- La autonomía de intentos antiguos se recalcula con la escalera de 5 niveles
  (una pista de 3 antes daba 75; ahora 83).

## 5. Resultados de las pruebas (ejecución real de este entorno)

| Comprobación | Resultado |
|---|---|
| `gofmt -l`, `go vet ./...` | sin avisos |
| `go test -count=1 ./...` | 16 paquetes, 89 pruebas de primer nivel, 0 fallos |
| `go test -race ./...` | sin fallos |
| `go run ./cmd/labctl validate` | 187 laboratorios, 17 rutas, 27 insignias, 8 certificaciones |
| `npm run lint` (tsc + traducciones + `node --test`) | 2392 textos traducidos; 13/13 pruebas del modelo |
| `npm run build` | compilado sin errores |
| `node web/e2e/flows.mjs` (Chromium) | 77/77 comprobaciones |

Las pruebas de navegador cubren: preferencias del plan; guardado sin conexión
y envío al volver; simulacro suspendido y aprobado; las cinco pistas en orden
y registradas; modo independiente sin pistas (API y página); diagnóstico e
informe tras un cambio a ciegas; competencias y objetivo semanal; bienvenida
con diagnóstico y plan; navegación en escritorio y móvil sin desbordes;
búsqueda con teclado y atajos; tema y densidad tras recargar; idioma desde el
móvil; sesión por cookie (sin token en `localStorage`, banderas de la cookie,
CSRF rechazado, sin bloqueos de CSP, cierre de sesión); axe (WCAG 2.2 A/AA)
sin infracciones en 13 páginas y en el laboratorio, en claro y oscuro.

## 6. Comandos

```sh
(cd web && npm ci && npm run build)       # web estática en web/out
go run ./cmd/gcplab                       # http://localhost:8080
go test ./... && (cd web && npm run lint) # pruebas
node web/e2e/flows.mjs http://localhost:8080   # pruebas de navegador (servidor en marcha)
VERSION=2.0.0 ./deploy/windows/build-msi.sh    # instalador de Windows
VERSION=2.0.0 ./deploy/portable/build-zip.sh   # ZIP portable
```

## 7. Pendiente (lo que no se completó y por qué)

- **CSP con `'unsafe-inline'` en scripts.** La exportación estática de Next.js
  incrusta sus datos en `<script>` en línea y no admite nonces; sin esa
  directiva la web no arranca. La CSP sí bloquea scripts de otros orígenes
  (salvo el CDN de Monaco), `object`, `base` y el enmarcado.
- **Monaco se carga del CDN jsdelivr** (comportamiento de 1.0, necesita
  internet para el editor). Empaquetarlo en local quitaría esa dependencia y
  ese origen de la CSP.
- **Revocación del token.** Cerrar sesión borra la cookie, pero el token
  firmado sigue siendo válido hasta que caduca (no hay lista de revocación).
- **Tutor con LLM opcional**: no se implementó; el tutor es determinista.
- **Pruebas e2e que faltan** del enunciado: resolver y suspender un ticket
  desde la interfaz del turno, y completar un laboratorio entero solo con el
  teclado. Hoy los comandos de los laboratorios se envían por la API de la
  sesión (que es el mismo estado que usan consola y terminal).
- **Vídeos.** No se pudo verificar la reproducción aquí: YouTube responde
  «vídeo no disponible» en este entorno incluso en una página sin las
  cabeceras de la aplicación. No hay bloqueos de CSP en las páginas con vídeo.
- **Laboratorio en el móvil**: el panel lateral sigue siendo estrecho; no se
  rediseñó.
- **Rediseño visual página a página** con el nuevo sistema: se añadieron tema,
  densidad y la nueva estructura, pero cada página conserva su maquetación.
- La búsqueda global no incluye todavía la guía de diagnóstico ni los runbooks.

## 8. Recomendaciones

1. Empaquetar Monaco en local y quitar el CDN de la CSP.
2. Añadir revocación de sesiones (lista corta en el almacén) para el cierre
   de sesión y el cambio de contraseña.
3. Ampliar el diagnóstico a errores de red concretos (ruta, etiqueta, rango)
   comparando el estado final con el de la solución, no solo los comandos.
4. Contar el informe post-incidente en la competencia de comprensión cuando
   haya suficientes datos para calibrarlo.
5. Llevar `web/e2e/flows.mjs` al CI con un servidor efímero.
