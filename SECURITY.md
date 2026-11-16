# Security policy

## Supported versions

Security fixes are released as PATCH versions of the latest MINOR version
(see [docs/releasing.md](docs/releasing.md)).

## Reporting a vulnerability

Please do not open a public issue. Report it privately through GitHub:
**Security → Report a vulnerability** on this repository
([private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)).

Include what you found, how to reproduce it, and the version affected. We aim
to answer within a week, and will credit you in the advisory unless you
prefer otherwise.

In scope, for example: secrets leaking in clear (database, history, exports,
logs), a crafted import file (OpenAPI, Postman, Bruno, cURL) that crashes the
app, corrupts data or runs code, a post-request script escaping its sandbox.
