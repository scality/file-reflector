[![Post Merge](https://github.com/scality/file-reflector/actions/workflows/post-merge.yaml/badge.svg)](https://github.com/scality/file-reflector/actions/workflows/post-merge.yaml)
[![GitHub release](https://img.shields.io/github/v/release/scality/file-reflector)](https://github.com/scality/file-reflector/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/scality/file-reflector)](go.mod)
[![License](https://img.shields.io/github/license/scality/file-reflector)](LICENSE)

# file-reflector

A generic source-to-target file synchronisation agent. It watches a source
directory and continuously makes a target directory mirror it: files and
directories are created, updated, and deleted in the target so that it
reflects the source.

file-reflector is filesystem-only. It has no knowledge of Kubernetes or of
the application whose files it syncs, which makes it a small, reusable
building block wherever one directory must track another.

## How it works

On startup the agent performs a full reconciliation: it walks the source
tree and brings every entry into the target, then walks the target tree and
removes anything that does not exist in the source. After that it watches
**both** the source and the target with `inotify` (via `fsnotify`) and
reconciles each change as it happens — so edits to the source propagate, and
manual drift in the target is corrected back to the source.

Content is compared cheaply: entries of the same size are hashed (SHA-256)
and only rewritten when the hashes differ; an entry whose content already
matches but whose mode or owner differs is fixed with `chmod`/`chown` alone.
Writes are atomic (write to a temporary file, then rename), so a consumer
never observes a half-written file.

Symbolic links in the source are followed when they point to regular files:
the link is mirrored as a regular file carrying its target's content. Any
other link — broken, looping, pointing outside the source tree, or pointing
to a directory — is treated as absent (with a warning), so any stale entry
at the same path in the target is removed. Every source symlink is also
re-checked on a fixed interval (`--symlink-poll-interval`, default 10s) so
that changes happening behind a link are picked up.

For the architecture and the reasoning behind these choices, see
[DESIGN.md](DESIGN.md).

## Usage

```
file-reflector --source /path/to/source --target /path/to/target
```

| Flag | Description |
| --- | --- |
| `--source` | Path of the directory to watch. **Required.** |
| `--target` | Path of the directory to sync to. **Required.** |
| `--ignore` | Pattern of paths to never create, modify, or delete in the target. Repeatable. |
| `--file-mode` | Octal mode forced on synced files (e.g. `0644`). Default: preserve the source's mode. |
| `--dir-mode` | Octal mode forced on synced directories (e.g. `0755`). Default: preserve the source's mode. |
| `--owner` | `uid:gid` forced on synced entries. Default: preserve the source's owner. |
| `--symlink-poll-interval` | How often source symlinks are re-checked for changes behind them. `0` disables. Default: `10s`. |
| `--log-format` | `text` or `json`. Default: `text`. |
| `--log-level` | `debug`, `info`, `warn`, or `error`. Default: `info`. |
| `--version` | Print the version and exit. |
| `--help` | Print usage and exit. |

`--file-mode` and `--dir-mode` accept permission bits only (within `0777`);
the setuid/setgid/sticky bits are not supported.

### Ignore patterns

`--ignore` decides which target paths the agent must never touch — useful to
let externally-managed files coexist in the target directory.

- A pattern **without** a `/` matches a path component at any depth, so
  `--ignore '*.tmp'` ignores every `.tmp` entry anywhere in the tree, and
  `--ignore cache` ignores any entry named `cache` together with its
  contents.
- A pattern **with** a `/` is matched against the path relative to the
  target root, so `--ignore 'logs/*.log'` ignores `.log` files directly
  under `logs/` only.

Patterns use Go's [`path/filepath.Match`](https://pkg.go.dev/path/filepath#Match)
syntax (`*`, `?`, `[…]`); there is no recursive `**`. An ignored path is
never created, modified, nor deleted.

One pattern is built in and always active: `..*`. Entries whose name starts
with `..` are the plumbing of atomic publishers such as the kubelet's
ConfigMap and Secret volumes (which reserve that prefix), and mirroring them
would duplicate the published content.

### Kubernetes ConfigMap and Secret sources

ConfigMap and Secret volumes work out of the box. The kubelet publishes
their keys atomically through symlinks:

```
myfile.txt -> ..data/myfile.txt
..data     -> ..2026_06_10_07_00_00.123456789/
```

Because source symlinks are followed and the `..*` plumbing is ignored by
default, mirroring such a volume needs no options:

```
file-reflector --source /etc/config --target /var/lib/app/config
```

The target receives only the published keys (here `myfile.txt`), as regular
files. When the ConfigMap is updated the kubelet swaps `..data`, which fires
no event on the visible paths; the symlink re-check picks the change up, so
it propagates within `--symlink-poll-interval`.

## Permissions and ownership

To mirror a tree faithfully the agent may need to read source files it has no
permission to read, write into target directories it does not own, and set an
arbitrary owner on the files it creates. Running as a non-root user, it relies
on three Linux capabilities:

| Capability | Why |
| --- | --- |
| `CAP_DAC_OVERRIDE` | Read any source entry; create, write, and delete in any target directory, bypassing permission checks. |
| `CAP_FOWNER` | `chmod` target entries the agent does not own. |
| `CAP_CHOWN` | `chown` synced entries to an arbitrary `uid:gid`. |

The container image already carries these as file capabilities on the binary
(see below). If you do not use `--owner` and the agent has ordinary access to
both trees, you can drop the capabilities your deployment does not need.

## Container image

The image is distroless, statically linked, and runs as a non-root user
(uid `65532`):

```
docker run --rm \
  -v /path/to/source:/source:ro \
  -v /path/to/target:/target \
  ghcr.io/scality/file-reflector:latest \
  --source /source --target /target
```

### Kubernetes

The capabilities are baked onto the binary, but a `restricted`
PodSecurityStandard drops them; grant them back explicitly:

```yaml
securityContext:
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
    add: ["DAC_OVERRIDE", "FOWNER", "CHOWN"]
```

Granting a capability through `capabilities.add` works even with
`allowPrivilegeEscalation: false`: once `no_new_privs` is set the container
runtime applies the added capabilities through the process's ambient set
(while the binary's baked file capabilities are ignored). Add only the
capabilities your configuration actually needs.

## Development

Requires Go 1.26+. Tooling: [golangci-lint](https://golangci-lint.run/) and
[mockery](https://vektra.github.io/mockery/) (both pinned in
`.devcontainer/Dockerfile`).

```
go test ./...          # run the test suite
golangci-lint run      # lint
golangci-lint fmt      # format
go generate ./...      # regenerate the service mocks (mockery)
go build ./cmd         # build the binary
docker build -t file-reflector .
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and the
coding conventions, and [DESIGN.md](DESIGN.md) for how the agent is built.

## License

See [LICENSE](LICENSE).
