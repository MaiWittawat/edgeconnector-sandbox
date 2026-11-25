# CLAUDE.md

This file guides Claude Code (claude.ai/code) when working in this repository.

## What this is

A sandbox for trying Edge Connector the way an outside team would. The center uses only the public API of edgeconnector. The customer machine is a Docker container that downloads the signed manager from the registry, the same path a real machine uses. Module `github.com/MaiWittawat/edgeconnector-sandbox`. The repo is **public**, but edgeconnector (the library) and edgeconnector-manager are private.

README is in Thai and code comments are Thai and explain *why*. The user writes Thai, so answer in Thai. Call the central system "center", never "CRM": it may be any system.

Sibling repos in `..` (one `go.work` at `../go.work` uses this repo, `edgeconnector-pkg` and `edgeconnector-manager`):
- `edgeconnector-pkg` = the library `github.com/MaiWittawat/edgeconnector`
- `edgeconnector-manager` = the manager
- `edgeconnector-registry` = the public registry of signed releases
- `../docs` = the docs for the whole system, built into one page with `python3 docs/site/build.py`

## Layout

| Path | What |
|---|---|
| `center/main.go` | example center: `connectorserver` + `store/open` (memstore file in `data/center`) + `ui` with one Basic-auth password, plus `AdminHandler` at `/admin/` for curl. No TLS (`Insecure`). Has one Call `dnc.check` that needs scope `dnc:read`, an Event handler that logs `job.done`, and `POST /admin/connectors/{id}/notify` (AdminHandler has no Notify). These match the `demo` plugin in edgeconnector-plugin. |
| `machine/Dockerfile` | the customer machine. It resolves the manager version from `manager/channels/<CHANNEL>.json` when `MANAGER_VERSION` is empty, downloads manifest and archive, checks them with `machine/verify`, then runs as user `edge` with `CC_DATA=/data`, `CC_RUN_DIR=/run/ec`. |
| `machine/entrypoint.sh` | creates `/data/machine-id` on first start, then `exec manager`. POSIX `sh` (busybox). |
| `machine/verify/main.go` | stdlib-only check of sha256 and Ed25519 signature over `cc-artifact:<name>:<version>:<os>:<arch>:<sha256>`. It must stay stdlib-only: the image cannot fetch the private edgeconnector module. |
| `compose.yml` | `machine` (modes 1 and 2), profile `local` = `registry` + `machine-local` (mode 3). Values come from `.env` (`.env.example`). |
| `scripts/package-plugin.sh` | builds a plugin for `linux/<docker arch>` and signs it into `data/registry` with the throwaway key `data/keys/sandbox.key`. A folder argument builds inside that folder's own module; an import path builds from this module (workspace). |

## The three modes

1. **Example center:** `go run ./center -ui-password secret` plus `docker compose up --build machine` with `INSECURE=true`.
2. **The user's own center:** only `machine`, with `CENTER_ADDR`, `INSECURE=false`, and optionally `CA=/certs/ca.pem` (file in `./certs`).
3. **Unreleased plugin:** `scripts/package-plugin.sh`, then `LOCAL_TRUSTED_KEYS` in `.env`, then `docker compose --profile local up --build machine-local`.

## Rules and gotchas

- **A plugin cannot run alone.** `worker.Run` takes flags and identity from the manager, and no service may self-enroll. Every mode therefore runs a manager container, and plugins are installed through the center.
- **Registry over `http://` (mode 3):** the manager accepts `http://` only for `127.0.0.1`, `localhost` and `::1` (`release/registry.go` in edgeconnector). That is why `machine-local` uses `network_mode: "service:registry"` and reads `http://127.0.0.1:8000`. `extra_hosts` must sit on `registry`, the owner of that network. The host sees the same registry on `127.0.0.1:8000`, for the example center's `-release-url`.
- **`CC_INSECURE` must never be empty:** a bool flag cannot parse `""`, so compose defaults it to `false`.
- **No self-update in the container:** `manager.update` needs `-self-dir`, which only `install-manager.sh` creates. To change the manager version, rebuild the image (`docker compose build --no-cache machine`).
- **Identity lives in a volume:** `manager-data` / `manager-data-local`. Reset it with `docker compose --profile local down -v`.
- **The machine-id lives in the same volume:** the center suspends a connector whose machine fingerprint changes ("machine fingerprint mismatch"). The fingerprint comes from `/etc/machine-id`, else the hostname; alpine has no machine-id and a container's hostname changes on every recreate. So `/etc/machine-id` in the image is a symlink to `/data/machine-id`, which `machine/entrypoint.sh` creates on first start. Keep the two together: a fresh volume means a new identity and a new machine.
- **Ignored and never committed:** `data/`, `.env` and `certs/*` (except `.gitkeep`). The sandbox key is throwaway and must never sign real releases.
- **Outside users and the private library:** they cannot `go run ./center` or `scripts/package-plugin.sh` until edgeconnector is public. The `machine` image works for everyone.
- **Go needs `GOPRIVATE` and `GONOSUMDB` for `github.com/MaiWittawat/*`.** An already set `GONOSUMDB` wins over `GOPRIVATE`.

## Checking a change

- `go vet ./...`, `sh -n scripts/package-plugin.sh`, `sh -n machine/entrypoint.sh` and `docker compose --profile local config --quiet` give a quick check.
- The full check:
  1. Run the example center.
  2. Create a token with `POST /admin/enrollment-tokens {"tenant_id":"t1","type":"manager"}`.
  3. Bring up `machine`, send `plugin.install hello 1.0.0` via `POST /admin/connectors/<id>/commands`, and check that `POST /admin/connectors/<plg-id>/requests {"type":"version"}` answers `1.0.0`.
  4. Repeat for mode 3 with `hello 0.1.0` packaged locally.
  5. Clean up with `docker compose --profile local down -v` and `rm -rf data .env`.
- Commit or push only when asked. Use Conventional Commits.
