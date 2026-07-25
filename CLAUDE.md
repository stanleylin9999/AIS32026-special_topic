# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

Tracks project-wide docs (`README.md`, `SETUP.md`, `ATTACK_WALKTHROUGH.md`,
`PROJECT_SCOPE.md`) plus the one piece of code this project actually authors, the
FrostyGoop rewrite under `frostygoop-rewrite/`. `GRFICSv3/` and `Mythic/` are
independently cloned forks with their own git remotes; both are `.gitignore`d here so
they don't get pulled into this repo's history.

The rewrite lives here rather than in either fork because it belongs to neither: it
targets GRFICS but isn't part of it, and Mythic is operated as a black box whose source
we don't modify. It is delivered onto an implant as a static binary, so it has no build
coupling to either stack.

Purpose: bridge a simulated OT/ICS environment (GRFICSv3) with a real C2 framework
(Mythic) so a Mythic-driven attack ends up moving the process on the GRFICS 3D
dashboard. Deadline 2026-07-30 (14:00 slides, 20:30-20:45 live demo).

Authorization: instructor-authorized for classroom/presentation use only. Never point
the attack chain at anything outside the two local Compose stacks.

## Where to look

- `README.md` — port map (both stacks), attack path summary, ops gotchas, deadline
- `SETUP.md` — build both stacks from a clean Linux host (in 正體中文)
- `ATTACK_WALKTHROUGH.md` — the seven-phase attack tutorial for the live demo
- `PROJECT_SCOPE.md` — deliverables, day-by-day schedule, and the verified environment
  facts the presentation cites (PLC interlocks, register persistence tiers, IDS blind
  spot). Anything claimed in slides should trace back to a fact recorded here.
- `GRFICSv3/CLAUDE.md` — GRFICS architecture (three networks, container roles,
  fork-specific deviations from upstream Fortiphyd)
- `Mythic/README.md` — Mythic is a third-party framework we operate but don't modify
  the source of; treat it as a black box driven via `mythic-cli` and the operator UI

## Commands (spanning both stacks)

Bring up just what the demo needs:

```bash
docker compose -f GRFICSv3/docker-compose.yml up -d simulation plc hmi router
(cd Mythic && sudo ./mythic-cli start)
```

Tear down between runs (see recovery in `ATTACK_WALKTHROUGH.md` for lighter options):

```bash
docker compose -f GRFICSv3/docker-compose.yml down
(cd Mythic && sudo ./mythic-cli stop)
```

## Ops gotchas that span both stacks

Native postgresql on host port 5432 belongs to an unrelated project on this host and
must not be touched. Mythic's `POSTGRES_PORT` is remapped to `5433` in `.env` to avoid
the collision. Anything that suggests reclaiming 5432 for Mythic is wrong.

UFW on this host drops bridge -> host `INPUT`/`FORWARD` by default, silently breaking
Poseidon's `192.168.95.1:80` callback path (times out rather than connection-refused).
`README.md`'s ops-notes section covers the allow rule.

Mythic's postgres and rabbitmq persist their initial credentials into bind-mounted
data dirs on first init; editing `.env` afterward causes auth failures. Only wipe
those dirs on a fresh install with no real operation data.

## docker-compose overrides append lists

`docker-compose.override.yml` merges `ports:` by appending, not replacing — you can
silently end up with the same host port bound twice. Always verify with
`docker compose config` before assuming an override took effect. Port choices belong
in the tracked compose file, not in a machine-specific override (which shouldn't
exist here).

## Git workflow

Three independent repos, three independent remotes:

- this meta-repo (top-level docs)
- `GRFICSv3/` (fork at `abb00717/GRFICSv3`)
- `Mythic/` (fork at `abb00717/Mythic`)

Commits/PRs stay in the repo that owns the changed file — e.g. edits to
`docker-compose.yml` under `GRFICSv3/` never come to this meta-repo. Only edits to
files directly under `ICS_C2_Demo/` (the markdown docs, `frostygoop-rewrite/`,
`.gitignore`, `.claude/`) belong here.
