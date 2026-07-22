# Trusted publication quickstart (Slice A)

1. Authenticate as the provider owner at the Gateway.
2. Create an endpoint binding for the exact A2A endpoint.
3. Create a single-use challenge and place the returned proof at the
   challenge URL's documented well-known location on the provider endpoint.
4. Complete the challenge. Inspect the binding until it is `verified`.
5. Continue with the release lifecycle in Sub-Issue #49. An endpoint binding
   alone does not publish or install an Agent.

For the repository Sample Agents, configure the absolute
`NEKIRO_AGENT_CHALLENGE_DIRECTORY`, write the exact returned proof without
adding a newline to `{directory}/{challengeId}`, complete the challenge through
the Gateway, then remove the proof file. The Sample Agent fails startup when
the directory configuration is absent or invalid; it never chooses a fallback
directory.

The service does not retry a failed challenge, follow redirects, or select a
different endpoint. Correct the provider endpoint or explicitly change the
approved network policy, then create a new challenge.
