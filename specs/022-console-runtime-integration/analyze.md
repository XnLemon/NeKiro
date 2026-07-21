# Consistency analysis

The change stays within the existing Control Plane/Data Plane boundary. The
Gateway exposes only public northbound routes, while the Router remains the
owner of invocation execution and Ledger facts. The Console does not create a
second source of truth and does not persist runtime data. CORS is a new public
policy and is therefore explicit in configuration and covered by HTTP tests.

Fallback audit before implementation:

| Item | Decision | Evidence |
| --- | --- | --- |
| Installation list default limit | Remove | Active API requires an explicit bounded limit. |
| Agent Card generated capability/name/schema/permission defaults | Remove | Agent Card schema requires these facts. |
| Token trimming | Remove | Development token must be exact and whitespace is invalid. |
| CORS implicit localhost or `*` | Remove | No policy evidence; explicit allowlist selected. |

Post-implementation fallback delta: removed 12, retained 0, added 0, net -12.
The removed paths were implicit installation limits, generated Agent Card
facts, token trimming/empty-token auth, implicit CORS grants, permission
description synthesis, and empty-success response acceptance. No new fallback
was added.
