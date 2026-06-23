# Wiki de Doki

<sub>[INDICE DE DOCUMENTACION / v0.11.0]</sub>

> Documentacion complementaria para el runtime de contenedores sin
> root Doki. Empieza en el [README](../README.md) para la definicion
> del proyecto, luego usa esta wiki para detalle de subsistemas,
> procedimientos de instalacion y referencia de CLI.

Esta wiki esta disponible en ingles (`Page.md`) y espanol (`Page.es.md`).

---

## Primeros Pasos

- [Instalacion](Installation.es) -- Setup por plataforma: Termux, Linux, macOS, Raspberry Pi, WSL2, Chromebook
- [Inicio Rapido](Quick-Start.es) -- Recorrido de 5 minutos: instalar, daemon, pull, run, compose, cleanup

## Conceptos

- [Arquitectura](Architecture.es) -- Internales del daemon, pipeline, conformidad OCI, cache de capas
- [Niveles de Aislamiento](Isolation-Levels.es) -- Los 12 modos de runner: proot, native, gVisor, microVM, wasm, pKVM, otros
- [Seguridad](Security.es) -- Seccomp, AppArmor, capabilities, user namespaces, TLS, modelo de amenazas

## Referencia

- [Referencia CLI](CLI-Reference.es) -- Catalogo completo de comandos: Docker, Podman, Compose, Kubernetes, Mesh, Deps
- [Configuracion](Configuration.es) -- schema config.json, variables de entorno, sockets por OS
- [Networking](Networking.es) -- Bridge, CNI, port mapping, DNS, iptables, fallback rootless, DokiLink mesh
- [Storage](Storage.es) -- 5 drivers, VFS, overlay2, btrfs/zfs, FUSE rootless, almacenamiento content-addressable

---

## Estructura del Repositorio

La wiki espeja el arbol de codigo en `pkg/`, `internal/`, y `cmd/`.
Lee [Arquitectura](Architecture.es) primero, luego profundiza en
paquetes especificos.

## Contribuir a la Wiki

El fuente de la wiki vive en `.wiki/` en la raiz del repo. Para anadir
una pagina:

1. Crea `Tu-Pagina.md` en `.wiki/`
2. Opcionalmente anade `Tu-Pagina.es.md` para la version en espanol
3. Anade un link desde esta pagina
4. Commit y push a `main`
5. El workflow de CI sincroniza a GitHub Wiki, GitLab Wiki y Codeberg Wiki

Las paginas usan GitHub Flavored Markdown con bloques de codigo
etiquetados.

## Mirrors

Todas las ediciones van a GitHub (`OpceanAI/Doki`), la fuente de
verdad:

- GitHub: [OpceanAI/Doki/wiki](https://github.com/OpceanAI/Doki/wiki)
- GitLab: [aguitauwu/doki/-/wikis](https://gitlab.com/aguitauwu/doki/-/wikis/home)
- Codeberg: [aguitauwu/Doki/wiki](https://codeberg.org/aguitauwu/Doki/wiki)
