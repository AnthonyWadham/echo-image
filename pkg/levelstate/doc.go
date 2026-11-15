// Package levelstate keeps one live map in a Cardinal world.
//
// State names the live map. Memory holds every map the build ships, so a change of map is a swap.
//
// # What it keeps
//
//   - In state: [Current], on one entity. It holds the live map's ID, the hash of its file, and an
//     epoch that counts changes.
//   - In memory: each map of [Config], decoded, built into a Box2D world of its own, and checked by
//     the game's rules (Config.Check). [NewPlugin] does this at boot, so a bad map fails the boot.
//
// # How it is used
//
//  1. Make the plugin with [NewPlugin], and call [Plugin.Register] before the game's own systems.
//  2. Read the live map with [Plugin.Engine] (its Box2D world, for queries) and [Plugin.Map].
//  3. Ask for another map with [Plugin.Change]. It is live from the next tick.
//
// # The one system
//
// It runs at Init and first in every tick. It makes memory match state:
//
//   - It takes a requested change.
//   - It takes this build's file for the live map. A snapshot from a build whose file differed gets
//     the new file, and the epoch rises.
//   - It emits [Changed] when a tick changes the live map.
//
// State that names a map this build lacks panics: the snapshot came from another build, and a world
// purge starts over.
//
// # Files
//
//   - levelstate.go: Current, Changed, Config and the Plugin.
//   - reconcile.go: the system.
package levelstate
