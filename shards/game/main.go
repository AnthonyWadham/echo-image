package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"

	"echo_image/shards/game/internal/physics"
	"echo_image/shards/game/internal/physics/timestep"
	"echo_image/shards/game/shard"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:     physics.ServerTickRateHz,
		SnapshotRate: uint32(timestep.Ticks(0.5)), // ticks between snapshots: half a second at any rate
	})
	if err != nil {
		panic(err.Error())
	}
	shard.Register(world)
	world.StartGame()
}
