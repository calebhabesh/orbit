# File synchronization

One owner synchronizes selected folders among trusted devices. Independent edits create histories that must remain understandable after devices reconnect.

## Language

**Device**: An enrolled machine identity. Reinstallation after identity or history loss creates a new device identity.

**Replica**: A device's participation in one shared folder, including its known history and available content.
_Avoid_: Primary, leader, cloud authority

**Shared folder**: A named replication group with a stable identity, explicit membership, and a local root on each member.

**Root**: The local directory registered as the working location for a shared folder.

**Version**: An immutable recorded state of one relative path, including its causal ancestry and either file contents, directory existence, or deletion.
_Avoid_: Timestamp winner, content hash as identity

**Captured version**: A locally observed version whose recorded state and required content have been durably retained.

**Head**: A version not causally superseded by another known version of the same path.

**Conflict**: Multiple incomparable heads at one path, or incompatible path structures that cannot simultaneously appear in a working folder.

**Working copy**: The visible file or directory currently present at a root-relative path. It is distinct from the replica's complete recorded history.

**Working basis**: The recorded version or reviewed version set from which a working copy was produced.

**Resolution**: A new version that explicitly supersedes the competing versions the owner reviewed.

**Tombstone**: A version recording intentional absence of a previously tracked path.

**Restore**: Creation of a new version using retained historical content.

**Durable receipt**: A peer's acknowledgement that a particular version and its required content were durably stored at that time.

**Applied status**: A report that a version was published into a peer's working folder. It may become stale after subsequent local activity.

**Retained history**: Older version content kept under the retention policy, separate from current heads and unresolved conflicts.

**Retirement**: Explicit removal of a device from a shared folder's active membership. Rejoining requires enrollment under a new identity.

**Structural conflict**: Incompatible requirements on the filesystem namespace, such as a file at a path that must also contain a child.
