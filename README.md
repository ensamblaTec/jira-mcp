# Jira MCP Server

Un servidor [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) de alto rendimiento escrito en Go (`CGO_ENABLED=0`) para interactuar con **Jira Cloud** y **Jira Data Center / Server**.

Diseñado para conectarse sin fricción desde **Claude Console**, **Claude Desktop**, **Claude Code** y desde **Antigravity CLI (`agy`)** / **Antigravity IDE**.

---

## 🚀 Capacidades

- **Lectura & Búsqueda Avanzada**: Búsqueda JQL, inspección profunda de tarjetas, comentarios, historial de cambios (changelog), registro de horas (worklogs) y dependencias/enlaces entre tickets.
- **Creación de Tarjetas**: Creación de historias, bugs, tareas, épicas y subtareas con resumen, descripción (markdown o ADF), prioridad, componentes, etiquetas y fechas límite.
- **Gestión Ágil, Sprints & Backlog**: Listado de tableros Kanban y Scrum, listado e inspección de sprints, inspección del backlog en tiempo real, creación y activación/cierre de sprints, y reasignación de tarjetas entre backlog y sprints.
- **Modificación & Flujos de Trabajo**: Actualización de campos, asignación/desasignación y transiciones de estado de workflow (`In Progress`, `Done`, etc.).
- **Operaciones Masivas (Bulk)**: Creación masiva, actualización masiva de campos y transiciones de estado en lote con concurrencia controlada y tolerancia a fallos.
- **Análisis Inteligente de Backlog & Sprint**: Diagnóstico de salud de backlog (tickets estancados > 30 días, porcentaje de no asignados, conteo por tipo y prioridad) y análisis de progreso de sprint con distribución de carga de trabajo por asignado.
- **Adjuntos y Exportación**: Listado, subida y descarga directa de archivos adjuntos, además de exportación de consultas JQL a formatos locales **JSON**, **CSV** o **Markdown**.

---

## 🔑 Autenticación & Variables de Entorno

El servidor detecta automáticamente el método de autenticación:
- **Jira Cloud**: Requiere `JIRA_EMAIL` + `JIRA_API_TOKEN` (Basic Auth).
- **Jira Server / Data Center**: Requiere `JIRA_API_TOKEN` o PAT (Bearer Auth).

### Variables Disponibles

| Variable | Descripción | Obligatorio | Ejemplo / Default |
|---|---|---|---|
| `JIRA_BASE_URL` | URL base de la instancia de Jira | **Sí** | `https://your-domain.atlassian.net` |
| `JIRA_EMAIL` | Correo electrónico de la cuenta (Jira Cloud) | Para Cloud | `user@example.com` |
| `JIRA_API_TOKEN` | API Token de Atlassian o PAT | **Sí** | `YOUR_API_TOKEN` |
| `JIRA_AUTH_TYPE` | Método de autenticación (`basic` o `bearer`) | No | Auto-detectado |
| `JIRA_API_VERSION` | Versión de API REST Jira (`2` o `3`) | No | `2` |
| `MCP_TRANSPORT` | Transporte MCP (`stdio` o `sse`) | No | `stdio` |
| `PORT` | Puerto HTTP para transporte SSE | No | `8080` |

> **Nota para Jira Cloud:** Puedes generar un API Token en [Atlassian API Tokens](https://id.atlassian.com/manage-profile/security/api-tokens).

---

## 🛠️ Conexión con Clientes MCP

### 1. Antigravity CLI (`agy`) & Antigravity IDE

Copia el archivo `.mcp.json.template` a `.mcp.json` en la raíz de tu proyecto o configura el servidor:

```json
{
  "mcpServers": {
    "jira": {
      "command": "/path/to/jira-mcp/bin/jira-mcp",
      "args": [],
      "env": {
        "JIRA_BASE_URL": "https://your-domain.atlassian.net",
        "JIRA_EMAIL": "user@example.com",
        "JIRA_API_TOKEN": "YOUR_API_TOKEN"
      }
    }
  }
}
```

O exportando las variables de entorno en tu terminal antes de iniciar tu cliente MCP:
```bash
export JIRA_BASE_URL="https://your-domain.atlassian.net"
export JIRA_EMAIL="user@example.com"
export JIRA_API_TOKEN="YOUR_API_TOKEN"
```

### 2. Claude Desktop

Edita el archivo de configuración `claude_desktop_config.json`:
- **Linux**: `~/.config/Claude/claude_desktop_config.json`
- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "jira": {
      "command": "/path/to/jira-mcp/bin/jira-mcp",
      "env": {
        "JIRA_BASE_URL": "https://your-domain.atlassian.net",
        "JIRA_EMAIL": "user@example.com",
        "JIRA_API_TOKEN": "YOUR_API_TOKEN"
      }
    }
  }
}
```

### 3. Claude Code / Claude Console

Para Claude Code CLI:
```bash
claude mcp add jira -- /path/to/jira-mcp/bin/jira-mcp
```
(Asegúrate de exportar `JIRA_BASE_URL`, `JIRA_EMAIL` y `JIRA_API_TOKEN` en tu `.bashrc` o `.zshrc`).

---

## 📋 Catálogo Completo de Herramientas (50 Tools)

### 🧠 Base de Datos de Contexto para LLMs & Búsqueda FTS5 (¡NUEVO!)
- `jira_get_context`: Genera un **Context Pack** operativo exhaustivo para un ticket. Consolida en un solo paquete estructurado:
  - **Jerarquía:** Épica o tarjeta padre y **todas las tarjetas hermanas** bajo la misma épica (para entender qué otros módulos se están construyendo).
  - **Subtareas:** Lista completa de subtareas con su estado de avance (`[x]` completadas / `[ ]` pendientes).
  - **Grafo de Dependencias:** Bloqueos bidireccionales (**⛔ BLOCKED BY** / **⚠️ BLOCKS**) y dependencias cruzadas.
  - **Actividad:** Comentarios recientes con autor y fecha, más historial de auditoría (changelog de cambios de estado y campos).
  - **Formato Dual:** Puede emitirse como Markdown denso (ideal para inyección directa en el prompt del LLM) o JSON estructurado.
- `jira_context_search`: Búsqueda contextual de texto completo potenciada por SQLite **FTS5 y BM25 ranking** (`< 5ms`). Devuelve resultados ranqueados por relevancia con fragmentos (`snippets`) de texto resaltados, con filtros opcionales por proyecto, estado, prioridad y asignado.

### ⚡ Caché Local SQLite & Sincronización Incremental
- `jira_sync_cache`: Sincroniza tarjetas hacia la base de datos local SQLite de forma incremental (descarga **ÚNICAMENTE** los tickets creados o modificados desde la última sincronización usando filtro temporal `updated >= ...`).
- `jira_search_local_cache`: Búsqueda instantánea en SQLite local (**< 5ms, sin tocar la red** ni consumir cuota de API). Búsqueda por texto en título/descripción/clave, proyecto, estado, asignado o prioridad.
- `jira_get_cached_issue`: Obtiene el detalle completo de un ticket directamente desde la BD local SQLite por clave (ej. `PROJ-38`).
- `jira_cache_stats`: Estadísticas de la base de datos local (total de tickets cacheados, relaciones indexadas, estado del motor FTS5, desglose por proyecto y fechas de corte).

### 🔌 Conexión y Metadatos
- `jira_test_connection`: Prueba la conexión con Jira y verifica el usuario autenticado.
- `jira_get_myself`: Obtiene los datos del perfil actual (ID, email, display name, zona horaria).
- `jira_list_projects`: Lista todos los proyectos disponibles con sus claves y nombres.
- `jira_get_project`: Obtiene detalles de un proyecto específico, tipos de issue y componentes.
- `jira_get_fields`: Lista todos los campos del sistema y personalizados (IDs de custom fields como Story Points, Epic Link, etc.).
- `jira_get_issue_types`: Lista los tipos de tarjetas disponibles en Jira (Bug, Story, Task, Epic, Sub-task).
- `jira_get_priorities`: Lista las prioridades de Jira (`Highest`, `High`, `Medium`, `Low`, `Lowest`).
- `jira_find_users`: Busca usuarios por nombre o correo para obtener su `accountId` exacto y asignar tarjetas sin errores.

### 🔍 Lectura e Inspección
- `jira_get_issue`: Obtiene el detalle completo de un ticket (campos, estado, asignado, fechas, etc.).
- `jira_search_issues`: Búsqueda avanzada con JQL y soporte automático para la nueva API `/rest/api/3/search/jql` de Jira Cloud y `/rest/api/2/search` en Data Center.
- `jira_get_epic_issues`: Lista todos los tickets que pertenecen a una Épica específica.
- `jira_get_issue_comments`: Obtiene los comentarios de una tarjeta.
- `jira_get_issue_transitions`: Lista las transiciones de estado disponibles para una tarjeta.
- `jira_get_issue_worklogs`: Lista los registros de tiempo invertido en una tarjeta.
- `jira_get_issue_links`: Lista los enlaces y dependencias del ticket (bloquea, depende, etc.).
- `jira_get_issue_changelog`: Historial de auditoría y cambios realizados en la tarjeta.

### ✍️ Creación, Versiones y Relaciones
- `jira_create_issue`: Crea una nueva tarjeta (Story, Task, Bug, Epic) con auto-guardado en el caché local.
- `jira_create_subtask`: Crea una subtarea vinculada a un ticket padre.
- `jira_link_issues`: Vincula dos tickets (ej. Blocks, Relates to, Duplicates).
- `jira_list_versions`: Lista las versiones y releases de software de un proyecto.
- `jira_create_version`: Crea una nueva versión/release en un proyecto.

### 🔄 Modificación y Gestión
- `jira_update_issue`: Actualiza campos existentes (título, descripción, prioridad, etiquetas, etc.).
- `jira_transition_issue`: Cambia el estado de una tarjeta en el flujo de trabajo (`Done`, `In Progress`, etc.).
- `jira_assign_issue`: Asigna o desasigna una tarjeta.
- `jira_add_comment`: Agrega un comentario en formato Markdown.
- `jira_add_worklog`: Registra horas/tiempo invertido (ej. `2h 30m`).
- `jira_delete_issue`: Elimina un ticket de Jira.

### 📊 Agile, Sprints & Backlog
- `jira_list_boards`: Lista los tableros Kanban y Scrum.
- `jira_list_sprints`: Lista los sprints de un tablero (filtro por activos, futuros o cerrados).
- `jira_get_sprint_issues`: Obtiene los tickets de un sprint determinado.
- `jira_get_backlog_issues`: Obtiene todos los tickets que están en el backlog del tablero.
- `jira_create_sprint`: Crea un nuevo sprint en un tablero.
- `jira_update_sprint_state`: Inicia (`active`) o cierra (`closed`) un sprint.
- `jira_move_issues_to_sprint`: Mueve tickets al sprint seleccionado.
- `jira_move_issues_to_backlog`: Mueve tickets de vuelta al backlog.

### 📈 Análisis Inteligente
- `jira_analyze_backlog`: Análisis cuantitativo de salud del backlog (tickets estancados sin tocar en >30 días, tasa de no asignados, desglose por tipo y prioridad).
- `jira_analyze_sprint`: Métricas del sprint (porcentaje de completitud, cuellos de botella y balance de carga de trabajo por desarrollador).

### ⚡ Operaciones Masivas (Bulk)
- `jira_bulk_create_issues`: Crea una lista de tickets en lote concurrentemente.
- `jira_bulk_update_issues`: Actualiza etiquetas, prioridad o asignado sobre múltiples tickets en bloque.
- `jira_bulk_transition_issues`: Mueve una lista de tickets al mismo estado en lote (ej. cerrar 10 tickets).

### 📎 Adjuntos y Exportación
- `jira_list_attachments`: Lista archivos adjuntos de un ticket con sus IDs y pesos.
- `jira_download_attachment`: Descarga un archivo adjunto al disco local.
- `jira_upload_attachment`: Adjunta un archivo local a una tarjeta de Jira.
- `jira_export_issues`: Exporta resultados JQL a un archivo local en formato `json`, `csv` o `markdown`.

---

## 💻 Compilación & Pruebas

```bash
# Compilar binario estático optimizado
make build

# Ejecutar suite de pruebas con detección de carreras (-race)
make test

# Generar reporte de cobertura HTML
make test-coverage

# Instalar binario en ~/.local/bin
make install
```

---

## 💡 Ejemplos de Prompts para el Asistente IA

- *"Conéctate a Jira con `jira_test_connection` y dime qué usuario está autenticado."*
- *"Analiza la salud del backlog del tablero 12 con `jira_analyze_backlog`."*
- *"Busca todos los bugs abiertos del proyecto PROJ asignados a mí usando `jira_search_issues`."*
- *"Crea 3 tareas masivas para refactorizar la autenticación usando `jira_bulk_create_issues`."*
- *"Pasa la tarjeta PROJ-42 a Done y agrega un comentario de cierre con `jira_transition_issue`."*
- *"Descarga el reporte adjunto de la tarjeta PROJ-88 a `./downloads/`."*

---

## Contributors

- **ensamblaTec** — Author & Maintainer
- **Google Antigravity / Gemini** — AI-Assisted Development

See [CONTRIBUTORS.md](CONTRIBUTORS.md) for details.
