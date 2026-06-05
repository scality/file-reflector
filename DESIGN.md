# Design

This document records how file-reflector is built and the decisions behind
it. For usage and flags see the [README](README.md).

## Goals

- Keep a target directory a faithful mirror of a source directory: create,
  update, and delete entries so the target reflects the source.
- React to changes in near real time, and self-heal drift in the target.
- Stay filesystem-only and application-agnostic — no knowledge of Kubernetes
  or of the data being synced.

### Non-goals

- Bidirectional sync or conflict resolution.
- A point-in-time snapshot of the whole tree (consistency is per-file and
  eventual; see [Concurrency](#concurrency-and-locking)).
- Propagating symbolic links (they are skipped, see [Sync model](#sync-model)).

## Architecture

The code follows a clean-architecture layering; dependencies point inward
only.

```
cmd/                  composition root: flag parsing, signal handling, wiring
pkg/
  domain/             entities (FileNode, NodeKind) — no external deps
  service/            port interfaces consumed by the use cases
  usecase/            application logic (SyncPath, InitialSync)
  infrastructure/     adapters implementing the ports + the DI container
  presentation/       the long-running watcher that drives the use cases
```

- **domain** holds the entities and nothing else; it imports no other layer.
- **service** declares small, role-based ports (`MetadataReader`,
  `ContentReader`, `ContentHasher`, `ContentWriter`, `MetadataWriter`,
  `EntryRemover`, `Matcher`, `EventSource`). The aggregate `SourceReader` and
  `TargetWriter` interfaces are compositions of these roles, so an adapter
  implements only what it needs and no method is duplicated between the source
  and target sides.
- **usecase** depends only on the ports. `SyncPath` reconciles one path;
  `InitialSync` walks the trees and delegates to `SyncPath`.
- **infrastructure** implements the ports against the local filesystem
  (`os.Root`-scoped), `fsnotify`, and `path/filepath.Match`, and wires
  everything in a hand-rolled DI container.
- **presentation/watcher** is the only long-running component: it starts the
  event sources, runs the initial reconciliation, and drains events.

## Sync model

### Two-phase initial reconciliation

On startup the agent runs `InitialSync`:

1. **Source walk (top-down).** Every source entry is reconciled into the
   target and recorded. Ignored subtrees are skipped entirely.
2. **Target cleanup (bottom-up).** Every target entry not seen in the source
   walk is removed; children are visited before their parent so a directory
   is emptied before it is removed.

### Event-driven reconciliation

The watcher then watches **both** trees with `fsnotify`:

- watching the **source** propagates edits to the target;
- watching the **target** detects out-of-band drift and reconciles it back to
  the source — this is the self-healing property.

The watch on each tree is installed *before* the initial reconciliation, so a
change that happens while the trees are being walked is captured and replayed
afterwards rather than lost. New directories are added to the watch set
recursively as they appear, because `inotify` is not recursive.

Events are processed by a single consumer goroutine, so reconciliation is
serialized within one agent. A path whose reconciliation fails transiently
(for example a file caught mid-write) is requeued after a short delay rather
than dropped; a permanently failing path keeps logging at each retry.

### Per-path decision

`SyncPath.Execute` stats both sides and applies the minimal operation:

| Source | Target | Action |
| --- | --- | --- |
| absent | present | remove |
| file | absent | write |
| file | file, different content | rewrite (atomic) |
| file | file, same content, different mode/owner | `chmod`/`chown` only |
| file | directory or symlink | remove, then write |
| directory | absent | create |
| directory | file or symlink | remove, then create |
| symlink | any | skip (treated as absent → stale target removed) |
| any | any, path ignored | no-op |

### Content comparison: full hashing, not mtime

When a source and target file have the same size, the agent hashes both
(SHA-256) and rewrites only if the hashes differ. It deliberately does **not**
use an rsync-style size+mtime quick-check.

The agent watches the target for self-healing, and both size and mtime are
forgeable by anyone with write access to the target: change the content
keeping the same length, then restore the old mtime with `touch -r` /
`utimensat`. A size+mtime quick-check would declare the tampered file
identical and never repair it, silently defeating self-healing. Tying the
repair decision to the actual content is the integrity guarantee the agent
exists to provide — the same reason rsync hides checksum comparison behind
`--checksum`. The cost is bounded: a hash only runs when sizes already match,
per event.

### Metadata overrides

`--file-mode`, `--dir-mode`, and `--owner` are applied in `SyncPath`, on the
source `FileNode` just after it is read. From that point the forced values
flow through the whole reconciliation — creation, rewrite, and the
metadata-only path — and a target that drifts from them is corrected like any
other divergence. This keeps the override an explicit application policy
rather than a hidden property of a filesystem adapter.

Only the nine permission bits are accepted; the setuid/setgid/sticky bits are
rejected at parse time because Go does not carry them in the low bits of
`fs.FileMode`, so a `chmod` would silently drop them and the agent would
re-`chmod` (and re-fire a target event) forever. Mode `0` is a valid value —
a fully locked-down file — but see the capability note below: a non-root
agent can only re-read and re-hash a mode-`0` file it created if it holds the
appropriate `DAC` capability.

## Concurrency and locking

file-reflector takes no file locks, by design.

- **Per-file atomicity** comes from `WriteAtomic`: write to a temp file, set
  its mode/owner, then `rename(2)` into place. `rename` is atomic on POSIX, so
  a reader of the target sees the old or the new complete file with final
  permissions, never a torn write. This is stronger than advisory `flock`,
  which would require cooperating readers.
- **Internal execution is serialized**: the watcher drains events in a single
  goroutine, so `SyncPath.Execute` never runs concurrently within one agent.
- **Source reads are TOCTOU-tolerant**: reading a source file mid-write may
  copy intermediate content, but the subsequent source change re-fires an
  event and re-reconciles. Consistency is eventual, not point-in-time.
- **Multiple writers on the same target are not supported.** Two agents, or
  one agent racing a third-party writer on the target, are unguarded — and an
  advisory lock would not help against a non-cooperating writer. Deploy a
  single writer per target (single replica / leader-elected). The guarantee is
  per-file atomicity, not a whole-tree snapshot.

## Capabilities and deployment profiles

The agent runs as a non-root user. To mirror a tree regardless of the files'
permissions and ownership it needs Linux capabilities; the exact set depends
on the deployment.

| Profile | Capabilities | When |
| --- | --- | --- |
| Read-only owner | `CAP_DAC_READ_SEARCH` | The agent already owns the target tree and `--owner` is not used. It still needs to read, hash, and walk every source entry regardless of mode. |
| Forced ownership | `CAP_DAC_OVERRIDE` + `CAP_FOWNER` + `CAP_CHOWN` | The realistic case: the agent writes into a target it does not own and/or forces a `uid:gid`. `DAC_OVERRIDE` to read source and write target past permission checks, `FOWNER` to `chmod` entries it does not own, `CHOWN` to set the owner. |

The container image bakes the forced-ownership set onto the binary. Grant the
capabilities through whichever mechanism fits the runtime:

- **setcap on the binary** (baked into the image): works for plain
  `docker run` on a default capability set.
- **systemd**: `AmbientCapabilities=` with `CapabilityBoundingSet=` and
  `NoNewPrivileges=` as appropriate.
- **Kubernetes**: `securityContext.capabilities` — `drop: ["ALL"]` then `add`
  the needed set. This works even with `allowPrivilegeEscalation: false`:
  once `no_new_privs` is set the container runtime (runc) applies the added
  capabilities through the process's ambient set, while the binary's baked
  file capabilities are ignored. This relies on a runtime that honours it —
  containerd and CRI-O do; the removed dockershim did not.

Restrictive forced modes such as `--file-mode=0` are only viable with these
capabilities: without them a non-root agent cannot re-read and re-hash its own
mode-`0` target files (EACCES), and the next `InitialSync` would fail. Mode
`0` is accepted at parse on purpose; the capability requirement is its
documented flip side.
