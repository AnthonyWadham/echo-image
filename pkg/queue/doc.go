// Package queue lets one feature ask another to change state the other owns, and fire an event on a
// later tick.
//
// # Owner queues
//
// Each kind of request has one owner: the feature whose state it changes.
//
//  1. The owner calls [Declare] in its RegisterTypes.
//  2. Any feature asks with [Send], or [SendAt] for a later tick.
//  3. The owner takes what is due with [Drain], from one system, once a tick.
//
// A request sent before the owner drains lands that tick. One sent after waits for the next drain. It
// is state, so it is never lost, and a snapshot keeps it. Drain sorts by the tick each was due, then
// by the owner's compare, never by the order they were sent in, which can change after a restore. A
// second drain of one queue in a tick panics.
//
// # Delayed events
//
// [DeclareDelayed] registers a system event E, its queue, and the PreUpdate system that fires it at
// the start of a tick. [Schedule] fires an E on a later tick and returns a [Ticket]. [Cancel] and
// [CancelFunc] stop events that are still waiting. A paused world runs no ticks, so nothing fires.
//
// # Stand-ins for what the engine lacks
//
//   - IDs with versions. Cardinal gives a freed entity ID out again at once. A request or event that
//     waits past its tick names its entity with a version the game keeps, and whoever acts on it
//     checks that version.
//   - A startup check that each sender runs before its owner drains, and each emitter before the
//     readers of its event. A game can check this from its source.
//   - A phase before PreUpdate. Delayed events fire from the PreUpdate system that DeclareDelayed
//     registers.
//
// # Files
//
//   - queue.go: Request, Declare, Send, SendAt and Drain.
//   - delayed_event.go: Event, DeclareDelayed, Schedule, Cancel and CancelFunc.
package queue
