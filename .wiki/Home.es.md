# Wiki de Doki

Bienvenido a la wiki de documentación de Doki (Motor de Contenedores Universal). Esta wiki es el complemento en profundidad del [README](../README.md) — empieza ahí para la propuesta de valor, luego ven aquí para los detalles.

> **Idioma**: Esta wiki está disponible en inglés (`Pagina.md`) y español (`Pagina.es.md`). Si una versión en español no existe para una página, por favor abre un issue.

## Última versión

**v0.9.2** (junio 2026) — Reescritura de DNS, 12 niveles de aislamiento, fixes críticos de bugs

- [Release notes](../RELEASE_NOTES.md)
- [Descargar binarios](https://github.com/OpceanAI/Doki/releases/tag/v0.9.2)
- [Qué hay de nuevo en v0.9.2](../README.md#qu%C3%A9-hay-de-nuevo)

## Tabla de contenidos

### Empezando

| Página | Qué cubre |
|:-------|:----------|
| [Instalación](Installation.es) | Instalación por plataforma: Termux, Linux (apt/dnf/pacman/portage), macOS, Android NDK, WSL2, Chromebook, Raspberry Pi, postmarketOS |
| [Inicio Rápido](Quick-Start.es) | Tutorial de 5 minutos: instalar → daemon → pull → run → compose → logs → cleanup |

### Conceptos

| Página | Qué cubre |
|:-------|:----------|
| [Arquitectura](Architecture.es) | Internos del daemon, pipeline, compliance OCI, cliente de registry, caché de capas |
| [Niveles de aislamiento](Isolation-Levels.es) | Cobertura detallada de los 12 modos: WASM, pKVM, microVM, sysbox, namespaces, gVisor, FEX, QEMU, proot, legacy32, chroot, native |
| [Seguridad](Security.es) | Perfil seccomp, AppArmor, capabilities, user namespaces, TLS, modelo de amenaza |

### Referencia

| Página | Qué cubre |
|:-------|:----------|
| [Referencia de CLI](CLI-Reference.es) | Los 108 comandos con tablas de flags, ejemplos, muestras de salida |
| [Configuración](Configuration.es) | Esquema de `config.json`, variables de entorno, paths de socket por SO, DNS, registries, niveles de log |
| [Networking](Networking.es) | Bridge, plugins CNI, port mapping, DNS, iptables (DNAT/SNAT), rootless (pasta), IPv6 |
| [Storage](Storage.es) | 5 drivers, VFS, requisitos de kernel para overlay2, btrfs/zfs, FUSE rootless, store content-addressable |

## Layout del repositorio

La wiki refleja el árbol de fuentes en `pkg/`, `internal/` y `cmd/`. Si quieres entender el código, lee [Arquitectura](Architecture.es) primero, luego sumérgete en paquetes específicos.

## Contribuir a la wiki

La fuente de la wiki se almacena en `.wiki/` en la raíz del repo. Para añadir una página:

1. Crea `Tu-Pagina.md` en `.wiki/`
2. Opcionalmente añade `Tu-Pagina.es.md` para la versión en español
3. Añade un link desde el ToC de [Home.md](Home)
4. Commit + push a `main`
5. El workflow de CI `.github/workflows/wiki-sync.yml` empuja la página a la Wiki de GitHub, la rama Wiki de GitLab y la Wiki de Codeberg

Las páginas de la wiki usan [GitHub Flavored Markdown](https://github.github.com/gfm/) con anchors en kebab-case (`#niveles-de-aislamiento`). Los bloques de código deben tener tag (` ```bash `, ` ```yaml `, ` ```dockerfile `). Tablas para contenido comparativo; ASCII art para diagramas.

## Mirrors

Esta wiki está sincronizada en tres forges. Todas las ediciones deben ir a GitHub (`OpceanAI/Doki`), que es la fuente de verdad:

- **GitHub**: [OpceanAI/Doki/wiki](https://github.com/OpceanAI/Doki/wiki) (primario)
- **GitLab**: [aguitauwu/doki/-/wikis](https://gitlab.com/aguitauwu/doki/-/wikis/home) (mirror, rama `wiki`)
- **Codeberg**: [aguitauwu/Doki/wiki](https://codeberg.org/aguitauwu/Doki/wiki) (mirror, repo separado)

Si encuentras una divergencia entre mirrors, abre un issue en GitHub.
