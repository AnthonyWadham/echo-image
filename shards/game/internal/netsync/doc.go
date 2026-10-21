// Package netsync decides who may receive which game state. It finds the viewers (players with a
// Body) and filters each feature's per-entity state by audience and seenBy. Each feature builds and
// sends its own event, so netsync never names another feature's state and doesn't grow with them.
package netsync
