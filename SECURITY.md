# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in AI Switch, please report it responsibly:

- **Do not** open a public GitHub issue
- Email the maintainer directly or use [GitHub's private vulnerability reporting](https://github.com/keepmind9/ai-switch/security/advisories/new)
- Include a clear description of the vulnerability and steps to reproduce

We will acknowledge your report within 48 hours and aim to provide a fix as soon as possible.

## Security Considerations

- **API keys** are stored in `config.yaml` — ensure this file is not publicly accessible
- The admin API is restricted to `localhost` only
- Config hot-reload via `POST /api/reload` is also localhost-only
