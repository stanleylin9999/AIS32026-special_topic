# ICS/OT + Mythic C2 Demo

Simulated industrial environment (GRFICSv3) attacked through a real C2 framework
(Mythic), running locally as two independent Docker Compose stacks.

## Layout

- `GRFICSv3/` — fork of Fortiphyd's OT/ICS lab, own git repo/remote. Provides the
  simulation/visualization target only; see its `CLAUDE.md` for architecture.
- `Mythic/` — fork of the Mythic C2 framework, own git repo/remote. Provides the
  attacker side.

Both are independently cloned repos, not submodules — this top-level repo only tracks
project-wide docs and holds no code of its own.

New to this project? `SETUP.md` walks through building both stacks from a clean Linux host.

## Port map

| Port                | Service                          | Notes                                                                                 |
| ------------------- | -------------------------------- | ------------------------------------------------------------------------------------- |
| 8090                | GRFICS simulation dashboard      | localhost only                                                                        |
| 6081                | GRFICS HMI (ScadaLTS)            | localhost only                                                                        |
| 51820/udp           | GRFICS router WireGuard          | localhost only                                                                        |
| 8080                | Mythic operator UI (nginx)       | localhost only                                                                        |
| 5433                | Mythic postgres                  | moved off 5432, which is taken by an unrelated native postgresql service on this host |
| 8082/8091/3000/8888 | Mythic hasura/docs/react/jupyter | moved off defaults to avoid clashing with the 8080/8090 convention above              |

GRFICS's `plc` container has no host-published port — it's only reachable from inside
the `b-ics-net` Docker network, which is the point (it's the actual attack target).

## Attack path

Mythic's HTTP C2 profile (`http` container) runs with `network_mode: host`, listening
on `0.0.0.0:80` on the host directly rather than on the `mythic_default` bridge network.
An implant placed on GRFICS's `b-ics-net` (192.168.95.0/24, where `plc` lives) reaches
it by calling back to the bridge's gateway address, `192.168.95.1:80` — no
`docker network connect` between the two Compose projects needed.

Status: Poseidon agent + HTTP C2 profile installed and running in Mythic, callback path
from `b-ics-net` to the C2 listener verified reachable. Payload build/deploy onto a
`b-ics-net` host and tasking it against `plc:502` (Modbus) is the next step, to make the
`simulation` dashboard visibly react.

## Operational notes

**Postgres/RabbitMQ password drift after editing `.env` by hand**: both containers
persist their credentials into a bind-mounted data directory on first init
(`postgres-docker/database`, `rabbitmq-docker/storage`). If `POSTGRES_PASSWORD` or
`RABBITMQ_PASSWORD` in `.env` changes afterward — e.g. from regenerating secrets in a
password manager — the already-initialized service keeps using the old password, so
`mythic_server` fails to authenticate to both and reports unhealthy. Fix: wipe the
affected data directory and restart, so it re-inits with the current `.env` value.
Safe to do on a fresh install with no operations/checkins yet; not safe once real
Mythic operation data exists.

**UFW blocks bridge-to-host traffic by default**: this host's UFW firewall drops
`INPUT`/`FORWARD` traffic from Docker bridge networks by default, which silently
breaks the `b-ics-net` → `192.168.95.1:80` callback path above (times out, not
connection-refused). Needs an explicit allow rule scoped to the relevant subnet/port;
see git history of this file or ask before assuming it's already open on a fresh host.

## Deadline

Presentation 2026-07-30 14:00 (slides), live demo slot 20:30-20:45.
