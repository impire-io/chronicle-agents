# Plan 006 — the install manifest

## Layout

```
types/workspace.go        Workspace, StoreDecl, TypeDecl, AnnounceDecl; Declaration; DefineTypes; Manifest, ManifestStore, ManifestType, WireType, Wire
types/workspace_test.go   the manifest's shape and order
types/solution.go         AnnounceInstance (create only); Announce keeps defining the type first
cli/cli.go                store init walks the declaration; web passes Version and PublicURL
cli/store_test.go         store init == the manifest, by value
web/server.go             Config.Version, Config.PublicURL (json:"-"); /install.json with CORS and preflight
web/server_test.go        /install.json
README.md                 enabling from the console
```

## Mechanics

1. `Declaration(rooms, ticks)` is the one value; `store init` iterates its
   stores — create unless present (`ensureStore`, `WithSchedules` where
   declared), define the types in order, and in the announcing store
   write the announcement and `locate` the interface.
2. `Manifest(version, url)` maps the declaration to the wire's shape; the
   `solution` object is `SolutionData`, so the manifest and the
   announcement cannot name different fields.
3. `web.Handler` marshals the manifest once at start, as it does the
   configuration, and serves it with `Access-Control-Allow-Origin: *`.
