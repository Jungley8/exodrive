# Security Policy

## Supported versions

Exodrive is pre-release. Only the latest `main` branch receives fixes.

## Report a vulnerability

Do not open a public issue for a security problem. Send a private report to the
maintainer through the repository's private vulnerability reporting page
(Security → Report a vulnerability).

Include these items:

- The affected version or commit.
- The steps that reproduce the problem.
- The data that the problem exposes, deletes, or corrupts.

You receive an answer within seven days. A fix and a disclosure follow. Credit
is given unless you ask otherwise.

## Threat model

Exodrive runs with your user permissions and moves your own data. It does not
run a service and does not accept network input.

These are in scope:

- Data loss through a bad copy, a bad switch, or a bad rollback.
- Deletion outside the reported cache path.
- A failure that reports success.
- A path that follows a symbolic link into data outside the cache.

These are out of scope:

- An attacker who already has your user account.
- A malicious `npm`, `pnpm`, `orb`, or other executable on your `PATH`.
- A filesystem or kernel defect.

## Design rules that limit damage

- The target filesystem is an allowlist. An unknown type is refused.
- Every copy is verified before the local data moves.
- A running tool stops the migration.
- Every move is recorded before the local copy is removed.
- A rollback restores the data before it removes the record.
