# No-Nonsense Containers (NNC)

NNC implements very simple linux containers, which are secure by default.
When they are misconfigured, chances are there won't be enough access to do what they need, rather than accidentally having too much access.

- The container gets a new namespace for every kernel concept that allows it.  This incudes mounts, processes, users, etc.  There is no way to opt out of this or share a namespace of any kind with the parent.
- An empty mount configuration results in an empty root.
Everything mounted in root must be asked for explicitly.
- There are no network devices by default, all network device must be asked for explicitly.
