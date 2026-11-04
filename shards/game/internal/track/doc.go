// Package track owns lag compensation: how far back a client's view may be judged, as track content.
// Its pure core, track/past, holds the row, the view rule and the window, so judges and the client
// predictor import only that.
package track
