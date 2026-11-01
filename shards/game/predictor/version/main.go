// Command version prints this tree's echoimage.SimVersion at the tick rate it's built for, which
// client/scripts/predictor.mjs stamps into predictor.wasm, built with the same tags. The predictor can't
// work it out itself: that would ship the source to players.
package main

import (
	"fmt"

	echoimage "echo_image"
	"echo_image/shards/game/internal/physics/timestep"
)

func main() { fmt.Println(echoimage.SimVersion(timestep.Rate)) }
