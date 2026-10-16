# Web Application

This directory contains the FRP Web Panel frontend, built with Next.js and exported into the Go application's static asset directory.

## Local Development

```bash
pnpm install --frozen-lockfile
pnpm dev
```

The frontend expects a running Master API. Configure the API endpoint in the application's platform settings or local development environment.

## Production Build

```bash
pnpm install --frozen-lockfile
pnpm build
```

The package build exports the static site to `../cmd/frpp/out`, which is embedded into the Go application during the container build.
