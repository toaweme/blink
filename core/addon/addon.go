// Package addon defines the extension surfaces that let blink grow without
// special-casing each integration in the CLI:
//
//   - Runtime (runtime.go): per-ecosystem service backends (shell, go, node, docker), picked by `runtime:` in blink.yaml.
//   - ServiceHook (service.go): cross-cutting lifecycle hooks (e.g. portkill).
//
// Registry (registry.go) is the single place the binary wires these in
// explicitly: no init() side-effects, no blank imports.
package addon
