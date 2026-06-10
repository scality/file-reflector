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
- Replicating symbolic links as links (they are dereferenced instead, see
  [Symbolic links](#symbolic-links-dereferenced-on-read-polled-for-changes)).

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

A third event source, the symlink poller, feeds the same merged stream: it
covers the changes that happen *behind* source symlinks, which `fsnotify`
structurally cannot report (see
[Symbolic links](#symbolic-links-dereferenced-on-read-polled-for-changes)).

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
| symlink (to a regular file) | any | follow: the file rules apply to its target |
| symlink (anything else) | any | treated as absent → stale target removed |
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

### Symbolic links: dereferenced on read, polled for changes

Source symlinks to regular files are dereferenced at the metadata layer:
the source-side `MetadataReader` resolves the link and reports its target's
mode, owner, and size, so the rest of the reconciliation treats the path as
a regular file and the target receives real content, never a link.
Resolution happens inside `os.Root`, which confines it to the source tree:
a link that is broken, loops, or escapes the root is reported as absent
(with a warning) and any stale target entry is removed. The target-side
reader keeps `lstat` semantics, so a symlink found in the target is seen as
a symlink and replaced by the source content.

Symlinks to directories are also treated as absent, on purpose. Reporting
them as directories would make every tree walk recurse through the link,
with two consequences: a link to an ancestor (`sub/up -> ..`) mirrors the
tree into itself unboundedly at startup, and the files mirrored under the
link's path go permanently stale afterwards — events name the real paths
only, and reconciling the link path alone does not recurse. Restricting the
follow to regular files keeps the model sound: one event (or one poll) on a
link path is always a complete reconciliation of that path.

Links are dereferenced rather than replicated because the consumer of the
target expects files: a replicated link would point at a path that does not
exist on the target side (or escapes it), and the primary case — the
kubelet's atomically-published ConfigMap and Secret volumes — uses links
purely as publication plumbing.

#### Why a poller

`inotify` is inode-based; there is no such thing as watching a symlink. A
change *behind* a link therefore produces no event naming the link's path:

- **kubelet volumes** publish keys as `myfile.txt -> ..data/myfile.txt` with
  `..data -> ..<timestamp>/`. An update swaps `..data` atomically: the only
  events fire on the `..*` plumbing — excluded by the built-in ignore — and
  never on `myfile.txt` itself.
- **in-place edits**: with `link -> cache/file.txt` and `--ignore cache`, a
  write to the file fires an event on `cache/file.txt` only, which
  reconciliation ignores; nothing ever names `link`.

So files are *evented*, symlinks are *polled*: a dedicated `EventSource`,
the symlink poller, sweeps the source tree every `--symlink-poll-interval`
(default 10s, `0` disables) and emits the path of every non-ignored symlink
into the same merged stream the watcher drains. `SyncPath` re-stats and
re-hashes on every event, so an unchanged link is a cheap no-op (a stat,
plus a hash only when sizes already match) and a changed one is repaired
within one interval — comfortably ahead of the kubelet's own update cadence
(about a minute).

Each sweep is stateless on purpose. Alternatives considered:

- **A registry of known links** (filled from walks and create events) would
  save the periodic walk, but adds state that can go stale silently: a lost
  create event means a link that is never polled and never repaired. The
  sweep self-heals by construction; the registry is a possible optimisation
  if walking ever shows up in profiles.
- **A dependency index** (link → watched real paths) re-introduces the same
  staleness risk and breaks on the kubelet case anyway: the real paths live
  under the ignored `..*` plumbing.
- **A full tree re-sync on any symlink-related event** handles the `..data`
  swap but cannot see in-place edits behind links into ignored directories,
  and rescans everything when one path would do.

### Built-in `..*` ignore

The `..*` ignore pattern is always appended to the user's `--ignore` list.
Atomic publishers reserve the `..` name prefix — the kubelet rejects
ConfigMap and Secret keys starting with `..` — so such entries can only be
publication plumbing, and mirroring them would duplicate every published
file in the target. The pattern is appended after flag parsing rather than
set as the flag's default, because `pflag` replaces a default with the first
user-supplied value, which would silently re-expose the plumbing as soon as
the user passes any `--ignore` of their own.

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
