# sshpfm

A desktop app that stores SSH port-forward profiles and opens them as local tunnels.

## Language

**Connection**:
A saved profile describing one local port forward: SSH host, credentials, remote and local ports.
_Avoid_: Profile, server, forward

**Tunnel**:
The live, running instance of a Connection, with a state (connecting, up, error, …). It is never persisted.
_Avoid_: Connection (when meaning the running thing), session

**Tag**:
A named, coloured label that the user attaches to Connections to organise them. A Connection can carry many Tags, and a Tag can exist with no Connection.
_Avoid_: Category, group, label (for the entity itself)

**Tag filter**:
The set of Tags selected in the sidebar. Only Connections carrying every selected Tag are visible.
_Avoid_: Search, group
