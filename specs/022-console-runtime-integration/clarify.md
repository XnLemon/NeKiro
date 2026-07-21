# Clarifications

| Topic | Decision |
| --- | --- |
| CORS origins | Explicit `NEKIRO_CORS_ALLOWED_ORIGINS`, comma-separated exact HTTP(S) origins. Empty and wildcard values are invalid. |
| Browser auth | Development-static bearer token remains configured through `VITE_NEKIRO_TOKEN`; it is not stored by the browser. |
| Workspace policy | The Console exposes one active Workspace at a time and sends its ID on every runtime request. |
| Agent auth | Cards declare `authentication.type`; no secret input is accepted in the Console. |
| SSE | The client validates event sequence/correlation and requires a terminal event before success. |
