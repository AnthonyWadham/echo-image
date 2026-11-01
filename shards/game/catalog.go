package main

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"

	"echo_image/shards/game/content"
	"echo_image/shards/game/internal/ability"
	"echo_image/shards/game/internal/area"
	"echo_image/shards/game/internal/echo"
	"echo_image/shards/game/internal/effect"
	"echo_image/shards/game/internal/player"
	"echo_image/shards/game/internal/projectile"
)

// catalog is every content file the shard reads, loaded once at boot. Each system gets the file it
// reads.
type catalog struct {
	movement  player.MovementTuning
	echoes    echo.EchoTuning
	abilities ability.AbilityTuning
	shots     projectile.ProjectileTuning
	effects   effect.EffectTuning
	areas     area.AreaTuning
}

// loadCatalog loads each file from fsys with content.Load, then checks the IDs one file names in
// another. Any error fails the boot.
func loadCatalog(fsys fs.FS) (catalog, error) {
	var c catalog
	var errs [6]error
	c.movement, errs[0] = content.Load[player.MovementTuning](fsys)
	c.echoes, errs[1] = content.Load[echo.EchoTuning](fsys)
	c.abilities, errs[2] = content.Load[ability.AbilityTuning](fsys)
	c.shots, errs[3] = content.Load[projectile.ProjectileTuning](fsys)
	c.effects, errs[4] = content.Load[effect.EffectTuning](fsys)
	c.areas, errs[5] = content.Load[area.AreaTuning](fsys)
	if err := errors.Join(errs[:]...); err != nil {
		return c, err
	}
	return c, c.lineUp()
}

// lineUp checks the IDs one file names in another: each ability's shots and effect kinds, and each
// champion's kit, Echo and Echo area. A champion without an Echo could never be eliminated.
func (c catalog) lineUp() error {
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(c.abilities.Abilities)) {
		refs := c.abilities.Refs(id)
		for _, shot := range refs.Shots {
			if _, ok := c.shots.Shots[shot]; !ok {
				errs = append(errs, fmt.Errorf("ability %q launches unknown shot %q", id, shot))
			}
		}
		for _, kind := range refs.Effects {
			if _, ok := c.effects.Kinds[kind]; !ok {
				errs = append(errs, fmt.Errorf("ability %q applies unknown effect kind %q", id, kind))
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(c.movement.Champions)) {
		if _, ok := c.abilities.Kits[id]; !ok {
			errs = append(errs, fmt.Errorf("champion %q has no kit", id))
		}
		if def, ok := c.echoes.Champions[id]; !ok {
			errs = append(errs, fmt.Errorf("champion %q has no Echo", id))
		} else if _, ok := c.areas.Kinds[def.Area]; def.Area != "" && !ok {
			errs = append(errs, fmt.Errorf("champion %q's Echo has unknown area kind %q", id, def.Area))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(c.echoes.Champions)) {
		if _, ok := c.movement.Champions[id]; !ok {
			errs = append(errs, fmt.Errorf("Echo %q belongs to no champion", id))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(c.abilities.Kits)) {
		if _, ok := c.movement.Champions[id]; !ok {
			errs = append(errs, fmt.Errorf("kit %q belongs to no champion", id))
		}
	}
	if _, ok := c.movement.Champions[c.movement.DevChampion]; !ok {
		errs = append(errs, fmt.Errorf("dev_champion %q is no champion", c.movement.DevChampion))
	}
	return errors.Join(errs...)
}
