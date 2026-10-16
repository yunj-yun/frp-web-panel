# FRP Web Panel

FRP Web Panel is a self-hosted management and monitoring panel for FRP, customized from FRP-Panel for this deployment's workflows: node health, Docker Compose generation, port visibility, traffic history, Caddy certificate management, and a streamlined interface.

## Features

- Manage Master, FRP Server, and FRP Client nodes from one web interface.
- Create, edit, and remove TCP, UDP, HTTP, and HTTPS tunnels, with advanced FRP JSON configuration.
- Generate Docker Compose files for Client and Server nodes, including Server-side Caddy.
- View node status, version, heartbeat, latency, and observed port usage.
- Inspect current and historical tunnel traffic, including custom date ranges.
- Review real-time node logs and open remote terminals when enabled by the administrator.
- Request and manage certificates, renew them automatically, and deploy them to selected Servers.
- Keep Worker management available on Linux nodes.

## Docker Deployment

Docker is the primary deployment method. The Master deployment script creates a random application secret and starts the SQLite-backed service. After creating nodes in the web interface, copy the generated Compose file to each target Linux host.

See [Docker deployment instructions](./deploy/README_zh.md) for image building, publishing, Master setup, and node deployment.

## Development

The repository contains the Go services and the Next.js web application. See [DEVELOPMENT_PLAN.md](./DEVELOPMENT_PLAN.md) and [PROJECT_PLAN.md](./PROJECT_PLAN.md) for the product scope and development notes.

## Attribution and License

This project is a customization of [VaalaCat/frp-panel](https://github.com/VaalaCat/frp-panel). The upstream AGPL-3.0 license and copyright notices are retained in [LICENSE](./LICENSE).
