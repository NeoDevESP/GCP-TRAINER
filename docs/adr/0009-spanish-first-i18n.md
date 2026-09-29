# ADR 0009 — Español como idioma principal; inglés como traducción

- Estado: aceptada
- Contexto: el alumnado y el equipo docente trabajan en español, pero la
  plataforma debe poder usarse también en inglés. Traducir por encima de un
  contenido escrito en inglés dejaría el español como ciudadano de segunda.
- Decisión:
  - Todo se escribe primero en español: interfaz web, mensajes del servidor,
    comandos propios de la terminal (`help`, `ticket`, `why`, `whatif`,
    `chaos`, `interview`, `arch`), laboratorios, misiones, biblioteca de
    fallos, habilidades, rutas, carrera e insignias.
  - El inglés es una traducción: cada archivo de contenido lleva un bloque
    `en:`; el código usa `i18n.P(lang, "es", "en")`; la web usa `t("texto en
    español")` con el diccionario `web/lib/en.ts`.
  - La salida de las herramientas reales (`gcloud`, `gsutil`, `bq`,
    `kubectl`, `terraform`) sigue en inglés, como en el trabajo real.
  - El idioma se resuelve por petición (`X-Lang`, `?lang=`, preferencia
    guardada del usuario, `Accept-Language`, español por defecto) y una sesión
    de laboratorio conserva el idioma con el que empezó.
  - Se puede responder en cualquiera de los dos idiomas: las palabras clave de
    las evidencias, justificaciones, preguntas a personas y notas de ticket se
    comparan con un glosario bilingüe y los grupos de palabras clave incluyen
    variantes en español.
  - Los incidentes generados se construyen dos veces con la misma semilla
    (biblioteca en español y su vista en inglés) y se emparejan campo a campo
    para producir el bloque `en:` del laboratorio.
  - CI falla si falta una traducción: `labctl validate` (contenido) y
    `npm run check:i18n` (interfaz, antes de cada build).
- Consecuencias: añadir contenido exige escribir también su `en:`; a cambio,
  ninguna pantalla queda a medio traducir y la evaluación es idéntica en los
  dos idiomas. Las respuestas de referencia de CI están en español, lo que
  demuestra que se puede aprobar respondiendo en español.
