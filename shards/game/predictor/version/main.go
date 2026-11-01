// Command version prints this tree's echoimage.SimVersion with the tags it's built with, which
// client/scripts/predictor.mjs stamps into predictor.wasm, built with the same tags. The predictor can't
// work it out itself: that would ship the source to players.
package main

import (
	"fmt"

	echoimage "echo_image"
)

func main() { fmt.Println(echoimage.SimVersion()) }
