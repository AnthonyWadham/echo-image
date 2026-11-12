// Package history owns lag compensation: each thing's record (component.History), which RecordSystem
// writes once a tick and SyncSystem sends as target_sync, and how far back a client's view may be
// judged, as history content. Its pure core, history/past, holds the row, the view rule and the window, so
// judges and the client predictor import only that.
package history
