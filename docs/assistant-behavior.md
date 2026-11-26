# Assistant behaviour

Status: **draft for approval**. This document is the contract for the voice
assistant. Code follows it; if the code and this document disagree, the code is
wrong unless this document is updated first.

## Purpose

Jarvis is a voice assistant:

1. It waits for a wake word.
2. It receives natural language by voice.
3. It executes an action when the sentence asks for one.
4. It answers by voice when the sentence asks for information.

There are two fundamental intents and one grey zone:

| Intent | Examples | Result |
|---|---|---|
| Action | "enciende la luz del salón", "tengo frío", "no veo nada" | A Home Assistant action or a spoken reason why it cannot act |
| Chat | "cuéntame un chiste", "explícame la desigualdad de Cauchy-Schwarz", "¿de qué color es el caballo blanco de Santiago?" | A spoken answer from the model or from a fixed learned reply |
| Data question (grey zone) | "¿qué temperatura hace?", "¿va a llover?", "¿debo llevar paraguas?", "¿está encendida la luz?" | A spoken answer filled from exposed Home Assistant entities or the news service |

The response may be a short confirmation ("Hecho.") or a more elaborate
answer. The source of the answer depends on whether the sentence is already in
memory.

## Vocabulary

- **L1**: every deterministic, fast layer that resolves a sentence without the
  language model. Memory, the action catalog, built-in intents and data
  templates are L1.
- **Fallback**: Home Assistant conversation and then the local model, used only
  when L1 does not resolve the sentence.
- **Taught entry**: a sentence stored in L1 by the user, either as an action, a
  fixed reply or a data template.
- **Pending entry**: a taught entry whose target the client could not infer. It
  is stored incomplete and must be completed in Manhattan.
- **Template**: a sentence with placeholders filled from an entity at answer
  time, for example `Hay {temperature} grados y está {condition}.`

## Resolution chain

A sentence is resolved by the first layer that can handle it:

1. **Global cancel.** The normalized text is exactly `cancelar`. Any active
   flow stops immediately and the assistant says `okay`.
2. **Built-in intents.** Time, repeat, cancel, help and feedback, matched
   locally and deterministically.
3. **Taught memory.** A previous sentence-to-action, sentence-to-fixed-reply or
   sentence-to-template association with a positive score. A pending entry says
   it is pending instead of acting.
4. **Action catalog.** A known verb plus the name, alias or entity ID of
   exactly one exposed entity.
5. **Data L1.** Question patterns that a template can answer: weather,
   temperature, rain, forecast, time, date, entity state, news.
6. **Home Assistant conversation**, if configured.
7. **Local model.** It receives the action catalog and a read-only snapshot of
   the exposed states and forecasts. It may choose actions or answer.
8. **Failure phrase** for the matching failure type.

The deterministic layers always run before the model. The model is a fallback,
not the first responder: it is too slow on the current hardware to be used for
every input. The model is nevertheless expected to decide whenever L1 cannot,
because memory is what makes that affordable.

## Question or order

- An **action verb with a clear target** means action, even when the sentence is
  written as a question: "¿puedes apagar la tele?" acts.
- A **question without an action verb** is answered: "¿está encendida la tele?",
  "¿qué temperatura hace?".
- A **physical state** ("tengo frío", "no veo nada", "hay poca luz") means
  action when an exposed device can fix it. When no device can fix it, the
  assistant gives the useful datum and says that no device is configured.
  Example: "Hay 9 grados. No tengo ningún aparato de calefacción configurado."
- **Physical words are not intents by themselves.** `temperatura`, `grados`,
  `frío`, `calor` and `humedad` only trigger the physical-state path when the
  sentence expresses a state or a change (first person or an action verb), not
  when it asks for a datum. `¿qué temperatura hace?` is a question.
- **A question may name a device.** "¿está encendida la luz del salón?" is
  answered from state. Naming a device does not turn a question into an action.
- **Follow-up questions** reuse the last data topic for 60 seconds: after a
  weather answer, "¿y mañana?" and "¿y el sábado?" mean the same topic with a
  new day. This is the only conversational state kept. Each turn still starts
  with the wake word.

## Data answers and templates

Data questions are answered by templates filled from exposed entities. The
first answer to a new phrasing may need the model; after that it is a template
and costs no model call.

- **Built-in templates** exist for the common types: current temperature,
  weather condition, rain probability, daily forecast, umbrella question, time,
  date, on/off state, numeric attribute and news. They are correct by
  construction and are the preferred source.
- **A template is auto-created** when an answer uses exposed data and no
  built-in template fits. It is stored as a taught entry, filled on every use.
- **Grammar is part of the contract.** A template must be syntactically and
  semantically correct in Spanish: no missing spaces, no broken agreement, no
  condition code left untranslated. `Hay 15 grados y está lloviendo` is valid;
  `Hay 15 gradosy esta lluvia` is not.
- **Validation.** Auto-created templates pass a validator (known placeholders,
  balanced braces, no double or missing spaces, translated conditions, unit
  rules). A template that does not pass is stored as a pending draft and is not
  used for speech until it is fixed in Manhattan.
- **Placeholders** reference an entity and an attribute, for example
  `{weather.aemet.attributes.temperature}` or `{weather.aemet.state}` with the
  Spanish condition translation. Units are appended by the renderer.
- **Unavailable data** is never invented. If the entity is unavailable or the
  value is missing, the answer is the data-missing failure phrase.
- **The model sees** the exposed states and forecasts, read-only. It can answer
  from them but cannot use them to name an action or a device.

## Teaching flow

The flow starts with the keyword `aprende`, not automatically. It never opens
the microphone on its own.

### Trigger

- The client remembers the last order or question it could not resolve for
  60 seconds.
- `aprende` within that window teaches that sentence.
- `aprende` with no recent failure asks `Dime la frase.` and then follows the
  same flow.

### Sequence

1. The ordered or question sentence is reused, and the assistant asks
   `¿Es correcta la orden <X>?` or `¿Es correcta la pregunta <X>?`.
2. The answer is one of the exact words `sí`, `no` or `cancelar`.
   - `no`: the assistant says `Dime la orden.` (or `Dime la pregunta.`), records
     the new sentence and asks the confirmation question again.
   - `cancelar`: the assistant says `okay` and the flow ends.
   - `sí`: continue.
3. The assistant asks `¿Qué debo hacer?` for an order, or `¿Qué debo responder?`
   for a question.
4. The assistant infers the target:
   - One catalog action, or one data template, or one fixed reply.
   - Several candidates: it says `Explícamelo de otra forma.` and returns to
     step 3.
   - No candidate: it creates a pending entry with the sentence and says
     `No he podido deducir la acción. La dejo pendiente para ajustar en Manhattan.`
5. Success: the assistant says only `Entendido.`

### Dialogue rules

- Only the first turn needs the wake word (`Alexa, aprende`). After that, each
  prompt opens a **10 second response window** without the wake word.
- If nothing is heard, or the answer is not one of the exact words, the
  assistant repeats the prompt. It repeats indefinitely; only `cancelar` (or a
  successful ending) closes the flow.
- `cancelar`, when it is the whole sentence, is a global emergency word: it
  closes any conversational flow immediately with `okay`. Outside a flow it also
  answers `okay`.
- The flow never asks anything outside itself. A false wake word must not start
  a question.

### What is stored

- **Order**: the normalized sentence and the catalog action, with score 1.
- **Question with a data source**: the normalized sentence and a template, with
  score 1. Never a frozen text for a value that changes.
- **Question without a data source**: the normalized sentence and the fixed
  reply text, with score 1.
- **Pending entry**: the normalized sentence and a pending marker. The client
  says the entry is pending and never acts on it. Manhattan lets the user
  complete it by choosing a catalog action or writing a template.

## Memory and feedback

- A served action, reply or template is assumed correct. `bien` is implicit;
  the user only penalizes.
- `mal` within 60 seconds of the last action, reply or template sets its score
  to -1. A penalized action is not repeated; a penalized reply or template is
  not used again, and the next attempt escalates to the fallback layers.
- A repeated success raises the score. A different target replaces the
  association.
- Learned sentences are stored in `VOICE_LEARNING_FILE` with mode `0600`.
- The dashboard mirrors taught entries and pending entries. Edits are queued
  for the client, which applies them on its next poll.
- Only single-action sentences are learned. A sentence with several actions is
  not stored as one association.

## Model configuration

Jarvis is model-agnostic. Any OpenAI-compatible endpoint is valid. Today it is
the 0.5B model in the local container; tomorrow it can be a larger model on a
desktop machine with a GPU or a cloud API.

Manhattan has a **Model endpoint** section with:

- URL. This is the only required field; the local default is
  `http://127.0.0.1:8080`.
- Model name. It is optional: when empty, the dashboard uses the first model
  reported by the endpoint's `/v1/models`.
- Timeout.
- Optional API key. The key is write-only: the API stores it with mode `0600`
  and never returns it, only `configured` and a clear button.

A **Conectar Modelo** button probes the endpoint, detects the model when the
name is empty, and saves the configuration only when the probe succeeds. On
failure it shows the error and keeps the previous configuration. The client
polls the settings every 30 seconds and applies the new configuration to the
next request, without a restart. In-flight requests finish with the previous
client. If the dashboard is unreachable, the client keeps the last known
configuration.

## News service

A new service, separate from the collector and the dashboard.

- **Process**: Go service `news/`, container `m4nhattan-news`, in-memory items,
  no volume for news. Configuration is persisted so it survives a restart.
- **Inputs**: RSS and Atom feeds. Defaults: elDiario.es, infoLibre, La Marea,
  The Conversation ES and Civio.
- **Refresh**: every 5 minutes by default, configurable in Manhattan.
- **Retention**: 20 items per feed and 7 days by default. Manhattan offers the
  presets 5 items, 20 items / 7 days, 50 items / 30 days, and "only the last
  fetch". Items are never written to disk.
- **Voice**: reads one headline per feed by default, configurable, in feed
  order, as `Fuente: titular.` It does not summarize.
- **Manhattan**: a separate News section lists the recent items with the link
  to the real article, the refresh status and the per-feed errors. Feed URLs and
  the refresh interval are edited there.
- **Failure**: if the service is unreachable, the answer is
  `No tengo noticias ahora.`
- **Network**: the dashboard reaches the service over the compose network; the
  voice client reaches it on `127.0.0.1:8091`, published by the compose file.
- **Etiquette**: a descriptive user agent, conditional requests where the feed
  supports them, and titles plus links only.

## Failure phrases

| Situation | Phrase |
|---|---|
| Order that cannot be processed | `Aún no sé procesar la orden <X>.` |
| Data that is not available | `No tengo ese dato.` |
| Model unreachable or timeout | `No puedo responder ahora.` |
| News service unreachable | `No tengo noticias ahora.` |
| Physical state with no device | `<datum>. No tengo ningún aparato de <type> configurado.` |
| Pending entry | `No he podido deducir la acción. La dejo pendiente para ajustar en Manhattan.` |

The order failure phrase carries no invitation. The keyword `aprende` is
documented, not advertised on every failure.

## Response style

- Language: Spanish. Units: Celsius and km/h. Both are configurable.
- Data answers: one short sentence, 200 characters by default.
- Chat answers: at most two sentences or 400 characters by default.
- Both limits and the model timeout are configurable in Manhattan.
- Replies that echo the question, exceed the limit or contain no letters or
  digits are rejected. Rejection of a reply must never happen merely because
  the question mentions a device or a physical word.

## Acceptance contract

Every line is a test, by voice and by `POST /text`.

### Actions

- `enciende la luz del salón` → turns the switch on, says `Hecho.`
- `apaga la luz del salón` → turns the switch off, says `Hecho.`
- `silencia la tele` → mute.
- `sube el volumen de la tele` → volume up.
- `activa calentar 1h` → scene.
- `¿puedes apagar la tele?` → acts, because the action verb wins.

### Data

- `¿qué temperatura hace?` → `Hay 9 grados y está lloviendo.`
- `¿qué tiempo hace?` → condition and temperature.
- `¿va a llover?` and `¿va a llover mañana?` → yes/no and probability.
- `¿debo llevar paraguas?` → rain probability.
- `¿qué hora es?` and `¿qué día es hoy?` → time and date.
- `¿está encendida la luz del salón?` → `Sí, está encendida.`
- `¿cuánta humedad hay?` → the AEMET sensor.
- `cuéntame las últimas noticias` → five headlines, `Fuente: titular.`
- `¿y mañana?` up to 60 seconds after a weather answer → the same topic.

### Chat

- `cuéntame un chiste` → the model.
- `explícame la desigualdad de Cauchy-Schwarz` → the model, at most two
  sentences.
- `¿de qué color es el caballo blanco de Santiago?` → the model or a taught
  fixed reply.
- `¿quién eres?` → the model.

### Teaching

- `Alexa, aprende` after a failed order → `¿Es correcta la orden <X>?`
- `no` → `Dime la orden.` → the new sentence → `¿Es correcta la orden <Y>?`
- `cancelar` → `okay`, and any active flow ends.
- `sí` → `¿Qué debo hacer?` → the explanation → inference → `Entendido.`
- Ambiguous explanation → `Explícamelo de otra forma.`
- No inference → a pending entry and the pending phrase.
- `Alexa, aprende` after a failed question → `¿Qué debo responder?` → a
  template or a fixed reply.
- A data question template is created automatically and is editable in
  Manhattan.
- `mal` after an action → the action is not repeated.
- `mal` after a reply or template → it is not used again and the next attempt
  escalates.

### Failures

- Unprocessable order → `Aún no sé procesar la orden <X>.`
- Missing datum → `No tengo ese dato.`
- Model down → `No puedo responder ahora.`
- News down → `No tengo noticias ahora.`
- `tengo frío` with no climate entity → a datum and the no-device phrase.

### Infrastructure

- Changing the model in Manhattan and pressing Conectar Modelo → the next
  request uses the new endpoint within 30 seconds, without a restart.
- The model API key is never returned by the API.
- News refresh every 5 minutes; Manhattan shows the items with links.
- Container logs stay bounded with the `local` driver.

## Out of scope

- Web search beyond RSS feeds.
- Conversation state shared with a remote model.
- Interrupting speech (barge-in).
- Wake-word tuning and false-positive reduction.
- Authentication for the dashboard; it stays on localhost.
- Storing news items on disk.
- Multi-room awareness beyond `VOICE_ROOM`.
- Changes to the Home Assistant conversation agent.

## Risks

- A 0.5B model may propose poor templates. Built-in templates and the validator
  are the mitigation; a pending draft is better than a wrong sentence.
- The dashboard has no authentication. The model key is stored at rest and the
  dashboard must stay on localhost or a trusted network.
- Feeds change URLs and may rate-limit. The service keeps the last good items
  and reports per-feed errors in Manhattan.
- The teaching flow can wait indefinitely. `cancelar` is the only exit; a
  client restart closes it too.
- A cloud model sends the sentence and the context snapshot out of the house.
  The Model section makes that explicit.

## Test plan

- Go table tests for every acceptance line: intent classification, question
  versus order, physical intent, device-named questions, follow-ups, templates,
  teaching state machine and failure phrases.
- Template renderer tests with fixed entity states, including grammar cases and
  unavailable values.
- Teaching tests with a fake Home Assistant and a fake clock for the 60-second
  and 10-second windows.
- News service tests with RSS and Atom fixtures, no network.
- Dashboard lint and build with the existing Docker command.
- Integration by `POST /text` against the running client.
- A short manual voice pass, done by the user, for the ten most important
  phrases.

## Implementation order

1. Behaviour core: resolution chain, question versus order, reply policy,
   failure phrases, data L1 and templates.
2. Teaching v2: scores for replies and templates, pending entries, the state
   machine and the 10-second window.
3. News service, compose and the Manhattan News section.
4. Model settings, the Conectar Modelo button and the client poll.
5. Tests, the acceptance run and documentation updates.

The work happens on `feat/assistant-behavior` with a pull request; the user
approves the pull request.
