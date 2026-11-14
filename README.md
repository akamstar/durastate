# durastate

`dstate` keeps your long-running processes alive, from a single declarative YAML file: queue workers, APIs, scripts. One binary, no runtime dependency, runs as a native OS service. Windows first, Linux and macOS next.

> **Status:** work in progress, not usable yet.

```yaml
services:
  api:
    command: python app.py
```

## Documentation

- [Architecture decision records](docs/adr/README.md)

## License

[Apache 2.0](LICENSE)
